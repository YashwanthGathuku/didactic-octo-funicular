package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"sentinel-gateway/internal/auth"
	"sentinel-gateway/internal/connectors"
	"sentinel-gateway/internal/secrets"
)

// Storing and using a customer source connection.
//
// This is the layer stage 16.2 left unbuilt: the catalog, the descriptors, the
// driver and the conformance suite all existed and there was no way to save a
// connection, so none of it was reachable.
//
// Every route here is tenant-scoped through the same `resolveScope` the rest of
// the gateway uses, and no route returns a credential -- not because each
// handler remembers to strip one, but because `connectors.Connection` has no
// field that could carry it.

// maxConnectionBody bounds a submitted connection.
//
// A connection is a handful of short strings. The limit exists because the body
// is attacker-controlled and one of its fields is a credential the server has
// to hold in memory long enough to seal.
const maxConnectionBody = 64 << 10

var (
	connectionStoreOnce sync.Once
	connectionStore     *connectors.Store
	connectionStoreErr  error
)

// connectionStoreFor builds the persistence layer once.
func connectionStoreFor(db *sql.DB, cfg *Config) (*connectors.Store, error) {
	connectionStoreOnce.Do(func() {
		if cfg == nil || cfg.Sealer == nil {
			// Refused rather than substituted with an in-memory store. A
			// deployment with no durable secret store would accept connections
			// and lose their credentials on restart, which presents as every
			// connection failing authentication the morning after a deploy.
			connectionStoreErr = errors.New(
				"source connections require a configured secret sealer")
			return
		}
		// The secret store is built on the same database and the deployment's
		// sealer, whose key lives outside the database. A connection's
		// credentials therefore get the same protection as every other secret
		// this system holds, including the audit trail on every read.
		secretStore, err := secrets.NewSQLStore(db, cfg.Sealer)
		if err != nil {
			connectionStoreErr = err
			return
		}
		connectionStore, connectionStoreErr = connectors.NewStore(
			db, "sqlite", connectorRegistry(), secretStore)
		if connectionStoreErr != nil {
			return
		}

		// Lifecycle events go to the append-only evidence chain, so "who
		// pointed this at a production replica, and when" has an answer that
		// cannot be edited afterwards.
		connectionStore.SetAuditor(&ledgerConnectorAuditor{db: db})
		connectionStore.SetAuditFailureHandler(func(ev connectors.AuditEvent, err error) {
			// Loud, and not fatal to the operation it describes. Refusing to
			// delete a connection because the ledger was unavailable would
			// leave a live credential in place for the duration of an
			// unrelated outage.
			log.Printf("connections: AUDIT FAILURE recording %s for connection %d in tenant %s: %v",
				ev.Action, ev.ConnectionID, ev.TenantID, err)
		})
	})
	return connectionStore, connectionStoreErr
}

// registerConnectionRoutes mounts the connection lifecycle.
func registerConnectionRoutes(r chi.Router, db *sql.DB, cfg *Config) {
	r.Get("/connections", listConnections(db, cfg))
	r.Post("/connections", createConnection(db, cfg))
	r.Get("/connections/{id}", getConnection(db, cfg))
	r.Delete("/connections/{id}", deleteConnection(db, cfg))
	r.Post("/connections/{id}/test", testConnection(db, cfg))
	r.Post("/connections/{id}/secrets/{field}", replaceConnectionSecret(db, cfg))
}

// connectionScope builds the secret-store scope from the request principal.
//
// The tenant and the actor both come from verified claims, never from a request
// field. A connection's credentials are stored under the tenant the caller
// actually belongs to, so a caller cannot write into another tenant's secret
// namespace by asking.
func connectionScope(r *http.Request, perm auth.Permission) (secrets.Scope, *scopeError) {
	// resolveScope runs first, so membership and permission are checked before
	// any secret-store scope exists. The principal is then read from the
	// request context rather than taken off the scope, because
	// repository.Scope deliberately does not expose it -- a scope is an
	// authorization result, not a container of claims to be re-read.
	scope, serr := resolveScope(r, perm)
	if serr != nil {
		return secrets.Scope{}, serr
	}
	sec, err := secrets.NewScope(auth.FromContext(r.Context()), scope.TenantID())
	if err != nil {
		return secrets.Scope{}, &scopeError{status: http.StatusForbidden, code: "forbidden"}
	}
	return sec, nil
}

func listConnections(db *sql.DB, cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scope, serr := resolveScope(r, auth.PermReadTenant)
		if serr != nil {
			serr.write(w)
			return
		}
		store, err := connectionStoreFor(db, cfg)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"error": "connections_unavailable", "detail": err.Error(),
			})
			return
		}

		list, err := store.List(r.Context(), scope.TenantID())
		if err != nil {
			log.Printf("connections: list failed for tenant %s: %v", scope.TenantID(), err)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
			return
		}
		if list == nil {
			list = []*connectors.Connection{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"connections": list})
	}
}

func createConnection(db *sql.DB, cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sec, serr := connectionScope(r, auth.PermManageContract)
		if serr != nil {
			serr.write(w)
			return
		}
		store, err := connectionStoreFor(db, cfg)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"error": "connections_unavailable", "detail": err.Error(),
			})
			return
		}

		var body struct {
			DisplayName   string            `json:"displayName"`
			ConnectorType string            `json:"connectorType"`
			AuthMode      string            `json:"authMode"`
			Fields        map[string]string `json:"fields"`
			Secrets       map[string]string `json:"secrets"`
			Allowlist     []string          `json:"resourceAllowlist"`
			MaxRows       int64             `json:"maxRows"`
			MaxBytes      int64             `json:"maxBytes"`
			TimeoutSecond int               `json:"timeoutSeconds"`
			MaxPerMinute  int               `json:"maxPerMinute"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxConnectionBody)).Decode(&body); err != nil {
			// The decode error is not returned. A malformed body containing a
			// credential would otherwise be echoed in the message.
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": "invalid_request", "detail": "the request body could not be read",
			})
			return
		}

		conn, err := store.Create(r.Context(), sec, connectors.CreateRequest{
			DisplayName:   body.DisplayName,
			ConnectorType: body.ConnectorType,
			AuthMode:      body.AuthMode,
			Fields:        body.Fields,
			Secrets:       body.Secrets,
			Allowlist:     body.Allowlist,
			MaxPerMinute:  body.MaxPerMinute,
			Limits: connectors.Limits{
				MaxRows: body.MaxRows, MaxBytes: body.MaxBytes,
				Timeout: time.Duration(body.TimeoutSecond) * time.Second,
			},
		})
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, connectors.ErrNotSelectable) {
				// 409, not 400: the request is well formed and the platform is
				// not in a state to accept it. A 400 would send the operator
				// looking for a mistake in their own input.
				status = http.StatusConflict
			}
			writeJSON(w, status, map[string]any{
				"error":  "connection_rejected",
				"detail": err.Error(),
			})
			return
		}

		// 201 with the masked summary. The credential the caller just sent is
		// not echoed back, which is the one moment a naive implementation would
		// have returned it.
		writeJSON(w, http.StatusCreated, conn)
	}
}

func getConnection(db *sql.DB, cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scope, serr := resolveScope(r, auth.PermReadTenant)
		if serr != nil {
			serr.write(w)
			return
		}
		store, id, ok := connectionTarget(w, r, db, cfg)
		if !ok {
			return
		}

		conn, err := store.Get(r.Context(), scope.TenantID(), id)
		if err != nil {
			writeConnectionError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, conn)
	}
}

func deleteConnection(db *sql.DB, cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sec, serr := connectionScope(r, auth.PermManageContract)
		if serr != nil {
			serr.write(w)
			return
		}
		store, id, ok := connectionTarget(w, r, db, cfg)
		if !ok {
			return
		}

		if err := store.Delete(r.Context(), sec, id); err != nil {
			writeConnectionError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// testConnection makes a real check and records what it found.
//
// The response reports whatever the driver returned, including a failure. There
// is no path here that reports a healthy connection without one having been
// made -- the health comes from `store.TestConnection`, which writes what the
// driver said.
func testConnection(db *sql.DB, cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sec, serr := connectionScope(r, auth.PermManageContract)
		if serr != nil {
			serr.write(w)
			return
		}
		store, id, ok := connectionTarget(w, r, db, cfg)
		if !ok {
			return
		}

		health, checkErr := store.TestConnection(r.Context(), sec, id)
		if errors.Is(checkErr, connectors.ErrConnectionNotFound) {
			writeConnectionError(w, checkErr)
			return
		}

		// 200 whether the check passed or failed: the request succeeded and its
		// answer is the health. A non-2xx for a failed check would make an
		// unreachable customer database indistinguishable from a broken
		// gateway.
		body := map[string]any{
			"state":     health.State,
			"checkedAt": health.CheckedAt,
			"latencyMs": health.Latency.Milliseconds(),
		}
		if health.ErrorCategory != connectors.ErrorNone {
			// The category and its fixed sentence, never the driver's message.
			body["errorClass"] = health.ErrorCategory
			body["detail"] = health.ErrorCategory.Detail()
		}
		if checkErr != nil {
			var ce *connectors.ConnectorError
			if errors.As(checkErr, &ce) {
				// The driver's own text goes to the server log only, redacted.
				log.Printf("connections: connection %d check failed: %s", id, ce.LogDetail())
			}
		}
		writeJSON(w, http.StatusOK, body)
	}
}

func replaceConnectionSecret(db *sql.DB, cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sec, serr := connectionScope(r, auth.PermManageSecret)
		if serr != nil {
			serr.write(w)
			return
		}
		store, id, ok := connectionTarget(w, r, db, cfg)
		if !ok {
			return
		}

		var body struct {
			Value string `json:"value"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxConnectionBody)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": "invalid_request", "detail": "the request body could not be read",
			})
			return
		}

		field := chi.URLParam(r, "field")
		if err := store.ReplaceSecret(r.Context(), sec, id, field, body.Value); err != nil {
			writeConnectionError(w, err)
			return
		}
		// No content. There is nothing to return: the new credential is
		// write-only from the moment it is stored.
		w.WriteHeader(http.StatusNoContent)
	}
}

// connectionTarget resolves the store and the id from a request.
func connectionTarget(w http.ResponseWriter, r *http.Request, db *sql.DB, cfg *Config) (*connectors.Store, int64, bool) {
	store, err := connectionStoreFor(db, cfg)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "connections_unavailable", "detail": err.Error(),
		})
		return nil, 0, false
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		// The same shape as a connection belonging to another tenant, so a
		// malformed id and a foreign id are indistinguishable.
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "connection_not_found"})
		return nil, 0, false
	}
	return store, id, true
}

func writeConnectionError(w http.ResponseWriter, err error) {
	if errors.Is(err, connectors.ErrConnectionNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"error":  "connection_not_found",
			"detail": "connection not found in this tenant",
		})
		return
	}
	if errors.Is(err, connectors.ErrNotSelectable) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "connector_unavailable", "detail": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusBadRequest, map[string]any{
		"error": "connection_rejected", "detail": err.Error(),
	})
}

// ledgerConnectorAuditor writes connection lifecycle events to the evidence
// chain.
//
// The payload arrives already built by internal/connectors, which is the one
// place that decides what a connection event may say. This adapter adds
// nothing: a second place assembling the payload is a second place that can
// put a host or a credential into it.
type ledgerConnectorAuditor struct {
	db *sql.DB
}

func (a *ledgerConnectorAuditor) RecordConnectorEvent(ctx context.Context, ev connectors.AuditEvent) error {
	_, err := AppendAuditEvent(a.db, ev.TenantID, string(ev.Action), ev.Actor, ev.Payload)
	return err
}
