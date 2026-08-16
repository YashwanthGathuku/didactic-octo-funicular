package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "modernc.org/sqlite"

	"sentinel-gateway/internal/auth"
	"sentinel-gateway/internal/egress"
	"sentinel-gateway/internal/objectstore"
	"sentinel-gateway/internal/secrets"
)

// SlaExpectationResponse carries deterministic deadline state only.
//
// BreachRiskPct and CountdownMinutes were removed: they were assigned by an
// if-statement on Status (98.4/-15 when OVERDUE, 12.5/23 otherwise), which
// presented a constant as a live predictive figure. Predictive breach risk is
// an explicit non-goal for v1 (docs/engineering/SCOPE.md §3).
type SlaExpectationResponse struct {
	ID                 int64  `json:"id"`
	ContractID         int64  `json:"contractId"`
	PartnerName        string `json:"partnerName"`
	PartnerRouting     string `json:"partnerRouting"`
	ContractName       string `json:"contractName"`
	FilenamePattern    string `json:"filenamePattern"`
	ExpectedTime       string `json:"expectedTime"`
	GracePeriodMinutes int    `json:"gracePeriodMinutes"`
	DeliveryStart      string `json:"deliveryStart"`
	DeliveryEnd        string `json:"deliveryEnd"`
	Status             string `json:"status"` // PENDING, DUE, OVERDUE, BREACHED, ARRIVED, WAIVED

	// The business date this occurrence is for, and the local reading of its
	// deadline. The board previously showed only UTC instants, which a partner
	// disputing a breach cannot check against their own agreement.
	BusinessDate string `json:"businessDate,omitempty"`
	FeedID       string `json:"feedId,omitempty"`
	DueLocal     string `json:"dueLocal,omitempty"`
	Timezone     string `json:"timezone,omitempty"`
	BreachesAt   string `json:"breachesAt,omitempty"`

	// ScheduleNote explains a deadline that is not simply the contracted time:
	// a calendar adjustment, a schedule collision, or a daylight-saving
	// transition. Empty when none applied.
	ScheduleNote string `json:"scheduleNote,omitempty"`

	// ReviewRequired means an arrival could not be attributed to exactly one
	// occurrence and a human must decide. The occurrence is still ageing, so a
	// row carrying this flag is not a satisfied one.
	ReviewRequired bool `json:"reviewRequired,omitempty"`
}

type IncidentDetailResponse struct {
	ID             int64                     `json:"id"`
	ExpectationID  int64                     `json:"expectationId"`
	FileInstanceID *int64                    `json:"fileInstanceId,omitempty"`
	Type           string                    `json:"type"`
	Severity       string                    `json:"severity"`
	Status         string                    `json:"status"`
	CreatedAt      string                    `json:"createdAt"`
	PartnerName    string                    `json:"partnerName"`
	Filename       string                    `json:"filename,omitempty"`
	Sha256         string                    `json:"sha256,omitempty"`
	Findings       []ValidationFindingRecord `json:"findings"`
}

type RawIngestRequest struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

// TriageRequest is what the AI tier receives.
//
// It carries no raw content. AI is read-only and evidence-grounded, and the
// evidence it is grounded in must already be redacted -- a model provider is an
// external party, and a raw record sent to one has left this system.
type TriageRequest struct {
	FileID   int64    `json:"file_id"`
	Findings []string `json:"findings"`
}

type ActionProposal struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

type AnalystResponse struct {
	Summary         string                 `json:"summary"`
	Citations       []string               `json:"citations"`
	ProposedActions []ActionProposal       `json:"proposed_actions"`
	Confidence      float64                `json:"confidence"`
	AgentVersion    string                 `json:"agent_version"`
	Metrics         map[string]interface{} `json:"metrics"`
}

func main() {
	// Subcommands run and exit without starting the server.
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		runMigrateCommand(os.Args[2:])
		return
	}

	// 1. Configuration. Validated before anything is opened, so a misconfigured
	// process refuses to start instead of failing at first use.
	cfg, err := Load()
	if err != nil {
		log.Fatalf("%v", err)
	}

	// Every log record from this point on passes through the scrubber. It is a
	// backstop, not the primary control -- credentials held as secrets.Value
	// already redact themselves -- but it covers the log calls written by
	// someone who has not read internal/secrets, which over time is most of
	// them.
	log.SetOutput(secrets.NewLogWriter(os.Stderr, cfg.Scrubber))

	if cfg.IsDemo() {
		log.Printf("PROFILE=local-demo. Binding %s (loopback only).", cfg.Addr())
		if cfg.APIToken.IsZero() {
			log.Println("PROFILE=local-demo: API authentication is DISABLED. This profile is for a developer machine only.")
		}
	} else {
		log.Printf("PROFILE=%s. Binding %s.", cfg.Profile, cfg.Addr())
	}

	// 2. Database
	db, err := sql.Open("sqlite", sqliteDSN(cfg.DatabaseURL))
	if err != nil {
		log.Fatalf("Unable to open database %q: %v", cfg.DatabaseURL, err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("Unable to connect to database %q: %v", cfg.DatabaseURL, err)
	}

	// 3. Migrations. Versioned and recorded; see migrate.go.
	applied, err := Migrate(db)
	if err != nil {
		log.Fatalf("Migration failed: %v", err)
	}
	if len(applied) > 0 {
		log.Printf("Applied %d migration(s): %s", len(applied), strings.Join(applied, ", "))
	}

	// 3b. Artifact storage.
	//
	// Built before the router so a misconfigured store fails at startup rather
	// than on the first upload, which would turn an operator's error into a
	// rejected financial file.
	if err := buildObjectStore(cfg); err != nil {
		log.Fatalf("Artifact storage: %v", err)
	}

	// 4. Background inbox watcher.
	//
	// Only started when an operator has named the tenant its files belong to.
	// It has no request and therefore no principal, so leaving it to a default
	// meant every watched file landed in one tenant regardless of origin.
	if cfg.WatcherEnabled() {
		var known int
		if err := db.QueryRow("SELECT COUNT(*) FROM tenants WHERE id = ?", cfg.WatcherTenant).Scan(&known); err != nil || known == 0 {
			log.Fatalf("SENTINEL_WATCHER_TENANT=%q does not exist; create the tenant or unset it to disable the watcher", cfg.WatcherTenant)
		}
		log.Printf("Inbox watcher enabled for tenant %s at %s", cfg.WatcherTenant, cfg.InboxPath)
		StartInboxWatcher(db, cfg.WatcherTenant, cfg.InboxPath)
	} else {
		log.Println("Inbox watcher disabled: SENTINEL_WATCHER_TENANT is not set. Files dropped in the inbox will not be ingested.")
	}

	// Build the token verifier. In production a failure here is fatal: a
	// gateway that cannot verify identity must not serve traffic.
	var verifier *auth.Verifier
	if !cfg.IsDemo() {
		// The JWKS URL is operator-configured and fetched by this process, which
		// makes it an SSRF surface: a wrong or hostile value would otherwise
		// have this service issue a request to any address reachable from it,
		// including the cloud metadata endpoint. The guard resolves the host,
		// validates every address, and connects to a validated address.
		guard := egress.New(egress.Policy{
			AllowedHosts:     []string{jwksHost(cfg.OIDCJWKSURL)},
			RequireHTTPS:     true,
			Timeout:          15 * time.Second,
			MaxResponseBytes: 1 << 20,
			MaxRedirects:     2,
		})
		if _, err := guard.CheckURL(cfg.OIDCJWKSURL); err != nil {
			log.Fatalf("SENTINEL_OIDC_JWKS_URL is refused by the egress policy: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		keys, err := auth.FetchJWKSWithClient(ctx, cfg.OIDCJWKSURL, guard.Client())
		cancel()
		if err != nil {
			log.Fatalf("Cannot fetch identity provider keys from %s: %v", cfg.OIDCJWKSURL, err)
		}
		verifier, err = auth.NewVerifier(auth.VerifierConfig{
			Issuer:   cfg.OIDCIssuer,
			Audience: cfg.OIDCAudience,
			Keys:     keys,
		})
		if err != nil {
			log.Fatalf("Cannot build token verifier: %v", err)
		}
		log.Printf("Authentication enabled: issuer=%s audience=%s keys=%d",
			cfg.OIDCIssuer, cfg.OIDCAudience, len(keys))
	}

	// Background job workers. Until Prompt 08 nothing leased the jobs ingest
	// enqueued, so every uploaded artifact stayed RECEIVED forever.
	workerCtx, cancelWorkers := context.WithCancel(context.Background())
	defer cancelWorkers()
	stopWorkers, err := startWorkers(workerCtx, db, cfg)
	if err != nil {
		log.Fatalf("Cannot start job workers: %v", err)
	}

	r := NewRouter(db, cfg, verifier)
	server := &http.Server{Addr: cfg.Addr(), Handler: r}

	// Shutdown order matters. The server stops accepting first, so no request
	// is answered by a process that can no longer do the work it promises; then
	// the pool drains, so held jobs complete and release their leases rather
	// than waiting out a lease expiry on the next process.
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-shutdown
		log.Println("Shutting down: refusing new requests, then draining workers.")

		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("HTTP shutdown: %v", err)
		}
		stopWorkers()
		log.Println("Shutdown complete.")
		os.Exit(0)
	}()

	log.Printf("Sentinel Gateway listening on %s", cfg.Addr())
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed to start: %v", err)
	}
}

// NewRouter builds the HTTP surface.
//
// Extracted from main() so the route table is addressable by tests: Prompt 01
// requires proving that deleted routes return 404 rather than merely that their
// handlers no longer compile.
func NewRouter(db *sql.DB, cfg *Config, verifier *auth.Verifier) chi.Router {
	return NewRouterWithStore(db, cfg, verifier, cfg.ObjectStore)
}

// NewRouterWithStore is the constructor the tests use to supply a store
// directly. Production goes through NewRouter, which takes the one built at
// startup.
func NewRouterWithStore(db *sql.DB, cfg *Config, verifier *auth.Verifier, store objectstore.ObjectStore) chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Add CORS middleware
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Wildcard CORS on an unauthenticated API let any origin read the
			// database. Restrict to the configured origin.
			allowedOrigin := cfg.AllowedOrigin
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-CSRF-Token")
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
			next.ServeHTTP(w, r)
		})
	})

	// -------------------------------------------------------------------
	// Authentication.
	//
	// Fails closed in every direction. The previous middleware treated an unset
	// SENTINEL_API_TOKEN as "authentication disabled" and served every route
	// publicly with a log line as the only trace. That branch is gone.
	//
	// The local-demo profile is the single exception, and it is explicit: a
	// named demo principal with clearly demo roles, only reachable on loopback,
	// announced at startup and labelled on every screen.
	// -------------------------------------------------------------------
	authMW := &auth.Middleware{Verifier: verifier}
	if cfg.IsDemo() {
		authMW.DemoPrincipal = &auth.Principal{
			Subject:  "demo-operator@local",
			Issuer:   "local-demo-profile",
			Audience: "local-demo",
			Memberships: []auth.Membership{{
				TenantID: DefaultTenantID,
				Roles: []auth.Role{
					auth.RoleViewer, auth.RoleOperator, auth.RoleReviewer, auth.RoleTenantAdmin,
				},
			}},
		}
	}

	// Prometheus metrics.
	//
	// Guarded by its own credential rather than the API's identity layer: a
	// scraper is a machine with no tenant and no roles, so giving it an OIDC
	// identity would be modelling it as something it is not. In the demo profile
	// the process binds loopback only, which is the guard there.
	r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
		if !cfg.IsDemo() {
			// Value.Equal compares in constant time and the zero Value never
			// matches, so an unconfigured token cannot be satisfied by an
			// empty Authorization header.
			got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !cfg.MetricsToken.Equal(got) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
				return
			}
		}
		ServePrometheusMetrics(w, r)
	})

	// API Version 1
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(authMW.Authenticate)
		// CSRF applies only to cookie-authenticated mutations; a request bearing
		// an Authorization header is not forgeable cross-origin by a browser.
		r.Use(auth.RequireCSRFToken("sentinel_session", "X-CSRF-Token"))
		RegisterStreamRoutes(r)

		// Liveness: is this process running? It checks nothing else, and must
		// not, or a dependency outage would cause an orchestrator to kill a
		// healthy process instead of routing around it.
		r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "alive",
				"service": "sentinel-gateway",
				"profile": string(cfg.Profile),
				"demo":    cfg.IsDemo(),
			})
		})

		// Readiness: can this process serve critical operations right now?
		//
		// Every field below is derived from an actual probe. The previous
		// /health returned the literal string "healthy" and checked nothing,
		// so it reported healthy while the database was unreachable.
		r.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
			checks := map[string]any{}
			ready := true

			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()

			if err := db.PingContext(ctx); err != nil {
				checks["database"] = map[string]any{"status": "UNAVAILABLE", "detail": err.Error()}
				ready = false
			} else {
				var n int
				if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&n); err != nil {
					checks["database"] = map[string]any{"status": "DEGRADED", "detail": "schema not migrated"}
					ready = false
				} else {
					checks["database"] = map[string]any{"status": "OK", "migrationsApplied": n}
				}
			}

			// A dependency that is not configured is reported as such. It is
			// not counted as ready, and it is not counted as failed either.
			if cfg.ObjectStoreURL == "" {
				checks["objectStore"] = map[string]any{"status": "NOT_CONFIGURED"}
			} else {
				checks["objectStore"] = map[string]any{"status": "CONFIGURED", "url": cfg.ObjectStoreURL}
			}
			if cfg.AITierURL == "" {
				// Optional by design: deterministic ingestion never depends on AI.
				checks["aiTier"] = map[string]any{"status": "NOT_CONFIGURED", "required": false}
			} else {
				checks["aiTier"] = map[string]any{"status": "CONFIGURED", "required": false, "url": cfg.AITierURL}
			}

			w.Header().Set("Content-Type", "application/json")
			if !ready {
				w.WriteHeader(http.StatusServiceUnavailable)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ready":   ready,
				"profile": string(cfg.Profile),
				"checks":  checks,
			})
		})

		// GET SLA Board
		r.Get("/sla-board", func(w http.ResponseWriter, r *http.Request) {
			scope, serr := resolveScope(r, auth.PermReadTenant)
			if serr != nil {
				serr.write(w)
				return
			}
			// The scheduling terms come from the contract version the
			// occurrence was materialized under, not from file_contracts. The
			// contract row carries one mutable set of terms; the version is
			// what governed this business date, so a board row and a
			// historical report cannot disagree.
			rows, err := db.Query(`
				SELECT e.id, e.contract_id, p.name, p.routing_number, c.name,
				       COALESCE(v.filename_pattern, c.filename_pattern),
				       COALESCE(v.expected_local, c.expected_time),
				       COALESCE(v.grace_minutes, c.grace_period_minutes),
				       e.expected_delivery_start, e.expected_delivery_end, e.status,
				       e.business_date, e.due_local, e.timezone, e.breach_at,
				       e.schedule_note, e.review_required, COALESCE(v.feed_id, '')
				FROM expectations e
				JOIN file_contracts c ON e.contract_id = c.id AND c.tenant_id = e.tenant_id
				JOIN partners p ON c.partner_id = p.id AND p.tenant_id = e.tenant_id
				LEFT JOIN file_contract_versions v
				       ON v.id = e.contract_version_id AND v.tenant_id = e.tenant_id
				WHERE e.tenant_id = ?
				ORDER BY e.expected_delivery_end ASC
			`, scope.TenantID())
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer rows.Close()

			expectations := make([]SlaExpectationResponse, 0)
			for rows.Next() {
				var (
					exp      SlaExpectationResponse
					business sql.NullTime
					breach   sql.NullTime
					review   int
				)
				if err := rows.Scan(&exp.ID, &exp.ContractID, &exp.PartnerName, &exp.PartnerRouting, &exp.ContractName,
					&exp.FilenamePattern, &exp.ExpectedTime, &exp.GracePeriodMinutes,
					&exp.DeliveryStart, &exp.DeliveryEnd, &exp.Status,
					&business, &exp.DueLocal, &exp.Timezone, &breach,
					&exp.ScheduleNote, &review, &exp.FeedID); err != nil {
					// A row that cannot be read is logged rather than dropped
					// in silence: a board that quietly omits an overdue feed is
					// worse than one that errors.
					log.Printf("sla-board: skipping an unreadable expectation for tenant %s: %v",
						scope.TenantID(), err)
					continue
				}
				if business.Valid {
					exp.BusinessDate = business.Time.UTC().Format("2006-01-02")
				}
				if breach.Valid {
					exp.BreachesAt = breach.Time.UTC().Format(time.RFC3339)
				}
				exp.ReviewRequired = review != 0
				expectations = append(expectations, exp)
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(expectations)
		})

		// GET Partners
		r.Get("/partners", func(w http.ResponseWriter, r *http.Request) {
			scope, serr := resolveScope(r, auth.PermReadTenant)
			if serr != nil {
				serr.write(w)
				return
			}
			rows, err := db.Query(
				"SELECT id, name, routing_number, created_at FROM partners WHERE tenant_id = ? ORDER BY id ASC",
				scope.TenantID())
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer rows.Close()

			var partners []map[string]interface{}
			for rows.Next() {
				var id int64
				var name, routing, created string
				if err := rows.Scan(&id, &name, &routing, &created); err == nil {
					partners = append(partners, map[string]interface{}{
						"id": id, "name": name, "routingNumber": routing, "createdAt": created,
					})
				}
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(partners)
		})

		// POST Partners
		r.Post("/partners", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Name          string `json:"name"`
				RoutingNumber string `json:"routingNumber"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "Invalid JSON", http.StatusBadRequest)
				return
			}
			res, err := db.Exec("INSERT INTO partners (name, routing_number) VALUES (?, ?)", body.Name, body.RoutingNumber)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			id, _ := res.LastInsertId()
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(fmt.Sprintf(`{"status": "CREATED", "id": %d}`, id)))
		})

		// GET Contracts
		r.Get("/contracts", func(w http.ResponseWriter, r *http.Request) {
			scope, serr := resolveScope(r, auth.PermReadTenant)
			if serr != nil {
				serr.write(w)
				return
			}
			rows, err := db.Query(`
				SELECT c.id, c.partner_id, p.name, c.name, c.direction, c.filename_pattern, c.expected_time, c.grace_period_minutes, c.timezone
				FROM file_contracts c
				JOIN partners p ON c.partner_id = p.id
				WHERE c.tenant_id = ?
				ORDER BY c.id ASC
			`, scope.TenantID())
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer rows.Close()

			var contracts []map[string]interface{}
			for rows.Next() {
				var id, partnerID int64
				var partnerName, name, direction, pattern, expTime, tz string
				var grace int
				if err := rows.Scan(&id, &partnerID, &partnerName, &name, &direction, &pattern, &expTime, &grace, &tz); err == nil {
					contracts = append(contracts, map[string]interface{}{
						"id": id, "partnerId": partnerID, "partnerName": partnerName, "name": name,
						"direction": direction, "filenamePattern": pattern, "expectedTime": expTime,
						"gracePeriodMinutes": grace, "timezone": tz,
					})
				}
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(contracts)
		})

		// GET Incidents
		r.Get("/incidents", func(w http.ResponseWriter, r *http.Request) {
			scope, serr := resolveScope(r, auth.PermReadTenant)
			if serr != nil {
				serr.write(w)
				return
			}
			rows, err := db.Query(`
				SELECT i.id, i.expectation_id, i.file_instance_id, i.type, i.severity, i.status, i.created_at,
				       COALESCE(p.name, 'Central Clearing Network') as partner_name,
				       COALESCE(f.filename, '') as filename,
				       COALESCE(f.sha256_hash, '') as sha256
				FROM incidents i
				LEFT JOIN expectations e ON i.expectation_id = e.id
				LEFT JOIN file_contracts c ON e.contract_id = c.id
				LEFT JOIN partners p ON c.partner_id = p.id
				LEFT JOIN file_instances f ON i.file_instance_id = f.id
				WHERE i.tenant_id = ?
				ORDER BY i.id DESC
			`, scope.TenantID())
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer rows.Close()

			incidents := make([]IncidentDetailResponse, 0)
			for rows.Next() {
				var inc IncidentDetailResponse
				if err := rows.Scan(&inc.ID, &inc.ExpectationID, &inc.FileInstanceID, &inc.Type, &inc.Severity, &inc.Status, &inc.CreatedAt,
					&inc.PartnerName, &inc.Filename, &inc.Sha256); err != nil {
					continue
				}

				// Fetch findings if file instance exists
				if inc.FileInstanceID != nil {
					fRows, _ := db.Query(`
						SELECT id, code, rule_version, provenance, description, severity,
						       line_number, byte_offset, field_start, field_end,
						       evidence_redacted, expected_value, actual_value
						FROM validation_findings
						WHERE file_instance_id = ? AND tenant_id = ?
					`, *inc.FileInstanceID, scope.TenantID())
					inc.Findings = make([]ValidationFindingRecord, 0)
					if fRows != nil {
						for fRows.Next() {
							var f ValidationFindingRecord
							// raw_data is gone. The evidence column is redacted at
							// the point it is written, so this query cannot
							// return a payment instruction however it is used.
							//
							// The rule reference is no longer asserted here
							// either: it used to be set to the literal string
							// "Nacha Operating Rules 2025" for every finding
							// regardless of what produced it, which cited a
							// source the repository does not have. The version
							// now comes from the rule that raised the finding.
							if err := fRows.Scan(&f.ID, &f.Code, &f.RuleVersion, &f.Provenance,
								&f.Description, &f.Severity, &f.LineNumber, &f.ByteOffset,
								&f.FieldStart, &f.FieldEnd, &f.Evidence,
								&f.Expected, &f.Actual); err == nil {
								inc.Findings = append(inc.Findings, f)
							}
						}
						fRows.Close()
					}
				}

				incidents = append(incidents, inc)
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(incidents)
		})

		// GET Audit Ledger
		r.Get("/ledger", func(w http.ResponseWriter, r *http.Request) {
			scope, serr := resolveScope(r, auth.PermReadEvidence)
			if serr != nil {
				serr.write(w)
				return
			}
			summary, err := GetLedger(db, scope.TenantID())
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(summary)
		})

		// POST Raw Ingest (For Chaos & Synthetic Payload Buttons)
		r.Post("/files/ingest-raw", func(w http.ResponseWriter, r *http.Request) {
			var req RawIngestRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Invalid JSON body", http.StatusBadRequest)
				return
			}
			if req.Filename == "" {
				req.Filename = fmt.Sprintf("NACHA_INTAKE_%d.ach", time.Now().Unix())
			}

			scope, serr := resolveScope(r, auth.PermUploadArtifact)
			if serr != nil {
				serr.write(w)
				return
			}

			result, err := ProcessFileBytes(db, scope.TenantID(), req.Filename, []byte(req.Content))
			if err != nil {
				http.Error(w, "Processing failed: "+err.Error(), http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)
		})

		// POST Multipart File Upload.
		//
		// Streams to immutable storage and returns 202 with identifiers. The
		// handler this replaces called ParseMultipartForm, read the whole body
		// with io.ReadAll, and returned a synchronous validation verdict.
		//
		// A nil store means artifact storage is not configured. That is a 503,
		// not a fall back to the old in-memory path: accepting a financial file
		// this system cannot durably store is worse than refusing it.
		r.Post("/files/upload", func(w http.ResponseWriter, r *http.Request) {
			if store == nil {
				writeIngestError(w, http.StatusServiceUnavailable, "storage_unavailable",
					"artifact storage is not configured; uploads are refused rather than held in memory")
				return
			}
			ingestUpload(db, store)(w, r)
		})

		// GET raw artifact bytes through an audited streaming proxy.
		r.Get("/artifacts/{id}/content", func(w http.ResponseWriter, r *http.Request) {
			if store == nil {
				writeIngestError(w, http.StatusServiceUnavailable, "storage_unavailable",
					"artifact storage is not configured")
				return
			}
			serveArtifactContent(db, store)(w, r)
		})

		// POST Incident AI Triage (Proxies to Python AI Tier)
		r.Post("/incidents/{id}/triage", func(w http.ResponseWriter, r *http.Request) {
			triageScope, serr := resolveScope(r, auth.PermReadTenant)
			if serr != nil {
				serr.write(w)
				return
			}
			incIDStr := chi.URLParam(r, "id")
			incID, _ := strconv.ParseInt(incIDStr, 10, 64)

			// Fetch incident findings
			var fileID *int64
			_ = db.QueryRow("SELECT file_instance_id FROM incidents WHERE id = ?", incID).Scan(&fileID)

			findingsList := []string{}
			if fileID != nil {
				fRows, _ := db.Query("SELECT code || ': ' || description FROM validation_findings WHERE file_instance_id = ?", *fileID)
				if fRows != nil {
					for fRows.Next() {
						var s string
						if fRows.Scan(&s) == nil {
							findingsList = append(findingsList, s)
						}
					}
					fRows.Close()
				}
			}

			// Call Python AI Tier.
			//
			// There is no fallback. The previous offline branch invented a
			// summary, two Nacha citations, two proposed actions, a confidence
			// of 0.94 and a fixed token/cost block whenever this call failed.
			// Because AI_TIER_URL is not honoured and the address below is
			// hardcoded, that fabrication was the default behaviour in
			// containers rather than an edge case. A missing dependency now
			// produces UNAVAILABLE.
			aiReq := TriageRequest{
				FileID:   incID,
				Findings: findingsList,
			}
			aiReqBytes, _ := json.Marshal(aiReq)

			// Honour the configured address. This was hardcoded to 127.0.0.1
			// while AI_TIER_URL was set and ignored, so in containers the call
			// always failed and the fabricated fallback was the default path.
			if cfg.AITierURL == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"status":     "NOT_CONFIGURED",
					"detail":     "No AI tier is configured (AI_TIER_URL unset). Deterministic processing is unaffected.",
					"incidentId": incID,
				})
				return
			}

			resp, err := http.Post(cfg.AITierURL+"/analyze", "application/json", bytes.NewReader(aiReqBytes))
			if err != nil || resp.StatusCode != http.StatusOK {
				if resp != nil {
					resp.Body.Close()
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"status":     "UNAVAILABLE",
					"detail":     "AI analysis tier is not reachable. No analysis was produced.",
					"incidentId": incID,
				})
				return
			}

			var aiRes AnalystResponse
			decodeErr := json.NewDecoder(resp.Body).Decode(&aiRes)
			resp.Body.Close()
			if decodeErr != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadGateway)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"status":     "INVALID_RESPONSE",
					"detail":     "AI analysis tier returned a response that could not be decoded.",
					"incidentId": incID,
				})
				return
			}

			// Record AI run in audit ledger
			_, _ = AppendAuditEvent(db, triageScope.TenantID(), "AI_ANALYSIS_EXECUTED", "AI_ANALYSIS_TIER", map[string]interface{}{
				"incidentId":   incID,
				"confidence":   aiRes.Confidence,
				"citations":    aiRes.Citations,
				"actionsCount": len(aiRes.ProposedActions),
			})

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(aiRes)
		})

		// POST Incident Approval
		r.Post("/incidents/{id}/approve", func(w http.ResponseWriter, r *http.Request) {
			incIDStr := chi.URLParam(r, "id")
			incID, _ := strconv.ParseInt(incIDStr, 10, 64)

			// Actor identity comes from the verified principal ONLY.
			//
			// This handler previously read `actor` from the request body and
			// defaulted it to the literal "TREASURY_SUPERVISOR_01" when absent,
			// so any caller could record a decision under any name -- including
			// a name that looked like a real supervisor. Any `actor` field in
			// the body is now ignored entirely.
			scope, serr := resolveScope(r, auth.PermApproveRelease)
			if serr != nil {
				serr.write(w)
				return
			}
			principal := auth.FromContext(r.Context())
			if principal == nil || principal.ActorID() == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
				return
			}

			var body struct {
				Justification string `json:"justification"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if strings.TrimSpace(body.Justification) == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error":  "justification_required",
					"detail": "a decision must carry a reason; it is recorded in the audit ledger",
				})
				return
			}

			actor := principal.ActorID()

			_, _ = db.Exec(
				"UPDATE incidents SET status = 'RESOLVED', updated_at = datetime('now') WHERE id = ? AND tenant_id = ?",
				incID, scope.TenantID())

			_, _ = AppendAuditEvent(db, scope.TenantID(), "INCIDENT_RESOLVED_BY_REVIEWER", actor, map[string]interface{}{
				"incidentId":    incID,
				"justification": body.Justification,
			})

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":     "APPROVED",
				"incidentId": incID,
				"actor":      actor,
			})
		})

		// GET Generator Sample
		r.Get("/generator/sample", func(w http.ResponseWriter, r *http.Request) {
			presetStr := r.URL.Query().Get("preset")
			if presetStr == "" {
				presetStr = string(PresetBalancedPayroll)
			}
			result := GenerateNachaScenario(GeneratorPreset(presetStr))
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)
		})

		// GET AI Adversarial Evals.
		//
		// There is no fallback. The previous one returned passRatePct 100.0
		// with 5/5 passed whenever the evaluator was unreachable, which is a
		// fabricated assurance result. An evaluation that cannot run reports
		// that it did not run.
		r.Get("/evals/run", func(w http.ResponseWriter, r *http.Request) {
			if cfg.AITierURL == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"status": "NOT_CONFIGURED",
					"detail": "No AI tier is configured (AI_TIER_URL unset). No pass rate was produced.",
				})
				return
			}

			resp, err := http.Get(cfg.AITierURL + "/evals/run")
			if err != nil || resp.StatusCode != http.StatusOK {
				if resp != nil {
					resp.Body.Close()
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"status": "NOT_RUN",
					"detail": "Adversarial evaluation tier is not reachable. No pass rate was produced.",
				})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.Copy(w, resp.Body)
			resp.Body.Close()
		})

		// GET evidence export of the application hash chain (carries no regulatory claim)
		r.Get("/compliance/export", func(w http.ResponseWriter, r *http.Request) {
			scope, serr := resolveScope(r, auth.PermReadEvidence)
			if serr != nil {
				serr.write(w)
				return
			}
			pkg, err := GenerateCompliancePackage(db, scope.TenantID())
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Disposition", `attachment; filename="SEC_17a4_COMPLIANCE_PROOF_PACKAGE.json"`)
			json.NewEncoder(w).Encode(pkg)
		})

		// POST Verify SSH Public Key
		r.Post("/security/verify-key", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Key string `json:"key"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Key == "" {
				http.Error(w, "missing key in payload", http.StatusBadRequest)
				return
			}
			res, err := VerifySshPublicKey(body.Key)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(res)
		})

		// POST Verify PGP Detached Signature
		r.Post("/security/verify-signature", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Payload   string `json:"payload"`
				Signature string `json:"signature"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid payload", http.StatusBadRequest)
				return
			}
			res, err := VerifyDetachedPgpSignature([]byte(body.Payload), body.Signature)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(res)
		})
	})

	return r
}

// jwksHost extracts the host the identity provider's key document is served
// from, so the egress allowlist contains exactly that host and nothing else.
//
// An unparseable URL yields the empty string, which matches no host: the
// allowlist then permits nothing and the CheckURL that follows reports the
// refusal rather than this function failing silently.
func jwksHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// buildObjectStore selects and constructs the artifact store.
//
// The selection is explicit and fails closed. A production profile with no
// OBJECT_STORE_URL already refuses to start in Load; here, a URL that names an
// S3 endpoint requires credentials, and a local-demo profile with no URL gets a
// filesystem store under a named directory. There is no path that leaves the
// process running with artifact storage silently absent.
func buildObjectStore(cfg *Config) error {
	if cfg.ObjectStoreURL == "" {
		if !cfg.IsDemo() {
			// Load already reports this; the guard is here so a future change to
			// Load cannot silently produce a production process with no store.
			return fmt.Errorf("OBJECT_STORE_URL is required outside the local-demo profile")
		}
		root := env("SENTINEL_ARTIFACT_ROOT", "./data/artifacts")
		store, err := objectstore.NewFilesystemStore(root)
		if err != nil {
			return err
		}
		cfg.ObjectStore = store
		cfg.ArtifactStoreRoot = store.Root()
		log.Printf("Artifact storage: filesystem at %s (local-demo)", store.Root())
		return nil
	}

	// A file:// URL selects the local adapter explicitly, which is what a
	// developer wants when exercising the production configuration path.
	if strings.HasPrefix(cfg.ObjectStoreURL, "file://") {
		root := strings.TrimPrefix(cfg.ObjectStoreURL, "file://")
		store, err := objectstore.NewFilesystemStore(root)
		if err != nil {
			return err
		}
		cfg.ObjectStore = store
		cfg.ArtifactStoreRoot = store.Root()
		log.Printf("Artifact storage: filesystem at %s", store.Root())
		return nil
	}

	endpoint, bucket, useSSL, err := objectstore.ParseS3URL(cfg.ObjectStoreURL)
	if err != nil {
		return err
	}
	accessKey := env("OBJECT_STORE_ACCESS_KEY", "")
	rawSecret := env("OBJECT_STORE_SECRET_KEY", "")
	if accessKey == "" || rawSecret == "" {
		return fmt.Errorf("OBJECT_STORE_ACCESS_KEY and OBJECT_STORE_SECRET_KEY are required for an S3 artifact store")
	}
	secretKey, err := secrets.New(rawSecret)
	if err != nil {
		return fmt.Errorf("OBJECT_STORE_SECRET_KEY: %w", err)
	}
	cfg.Scrubber.Register(secretKey)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	store, err := objectstore.NewS3Store(ctx, objectstore.S3Config{
		Endpoint:  endpoint,
		Bucket:    bucket,
		Region:    env("OBJECT_STORE_REGION", "us-east-1"),
		UseSSL:    useSSL,
		AccessKey: accessKey,
		SecretKey: secretKey,
	})
	if err != nil {
		return err
	}
	cfg.ObjectStore = store
	log.Printf("Artifact storage: s3 bucket %q at %s (tls=%v)", bucket, endpoint, useSSL)
	return nil
}
