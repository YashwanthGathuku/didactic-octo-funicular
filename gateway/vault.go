package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

// ---------------------------------------------------------------------------
// SECURITY REMEDIATION 2026-08-14
//
// Previous implementation had four defects, each individually a full break:
//
//  1. HARDCODED KEY. The HMAC secret was the literal string
//     "SENTINEL_FIPS140_HMAC_SECRET" compiled into the binary. Because the
//     tokenizer is deterministic, anyone with the source can precompute tokens
//     for the entire ABA routing space (~10^7 assigned) or the SSN space
//     (10^9) in seconds and invert every token. Keyed-hash tokenization is only
//     as strong as the key's secrecy over a low-entropy domain.
//
//  2. PLAINTEXT "VAULT". Tokens map[string]string stored the RAW value in
//     memory, unencrypted. The vault was a plaintext dictionary.
//
//  3. NO AUTHENTICATION ON DETOKENIZE. The only checks were
//     SupervisorID != "" and len(AuditReason) >= 10. AuthCertDigest was parsed
//     and never used. RequireApproval in the policy was never read. Any caller
//     could retrieve plaintext PII.
//
//  4. FALSE AUDIT CLAIM. The response asserted "auditLogged": true while never
//     writing to the audit ledger -- the worst possible failure for a product
//     whose value proposition is SEC 17a-4 evidentiary proof.
//
// Also fixed: fieldType[0:3] panicked on any fieldType shorter than 3 chars.
//
// NOTE ON LABELLING: the policy Algorithm field previously advertised
// "FPE_AES256" while the code did substring masking + HMAC. That is not
// format-preserving encryption. Real FPE requires FF1 or FF3-1 per NIST
// SP 800-38G. The label is now honest about what is actually implemented.
// FIPS 140 is a validation of a cryptographic module by an accredited lab --
// it cannot be asserted in a struct tag and that claim has been removed.
// ---------------------------------------------------------------------------

type TokenizationPolicy struct {
	TenantID        string   `json:"tenantId"`
	MaskedFields    []string `json:"maskedFields"`
	Algorithm       string   `json:"algorithm"`
	RetentionDays   int      `json:"retentionDays"`
	RequireApproval bool     `json:"requireApprovalForDetokenize"`
}

type TokenizedRecord struct {
	OriginalType string    `json:"originalType"`
	MaskedValue  string    `json:"maskedValue"`
	TokenKey     string    `json:"tokenKey"`
	TenantID     string    `json:"tenantId"`
	CreatedAt    time.Time `json:"createdAt"`
}

type vaultEntry struct {
	ciphertext []byte
	expiresAt  time.Time
}

type TokenVaultStore struct {
	mu       sync.RWMutex
	entries  map[string]vaultEntry
	Policies []TokenizationPolicy
}

var GlobalVault = &TokenVaultStore{
	entries: make(map[string]vaultEntry),
	Policies: []TokenizationPolicy{
		{
			TenantID:        "TENANT-MERIDIAN-PROD",
			MaskedFields:    []string{"RoutingNumber", "AccountNumber", "TaxIdentifier", "IndividualName"},
			Algorithm:       "HMAC_SHA256_TOKEN + AES-256-GCM_AT_REST",
			RetentionDays:   2555, // 7 years, SEC 17a-4(f)
			RequireApproval: true,
		},
		{
			TenantID:        "TENANT-APEX-GLOBAL",
			MaskedFields:    []string{"BeneficiaryAccount", "CounterpartyRouting"},
			Algorithm:       "HMAC_SHA256_TOKEN + AES-256-GCM_AT_REST",
			RetentionDays:   2555,
			RequireApproval: true,
		},
	},
}

var (
	vaultKeyOnce sync.Once
	vaultHMACKey []byte
	vaultAEADKey []byte
	vaultKeyErr  error
)

// loadVaultKeys reads key material from the environment. There is no default:
// if SENTINEL_VAULT_HMAC_KEY / SENTINEL_VAULT_AES_KEY are unset the vault
// refuses to operate rather than silently falling back to a known constant.
func loadVaultKeys() ([]byte, []byte, error) {
	vaultKeyOnce.Do(func() {
		h := os.Getenv("SENTINEL_VAULT_HMAC_KEY")
		a := os.Getenv("SENTINEL_VAULT_AES_KEY")
		if h == "" || a == "" {
			vaultKeyErr = errors.New("vault disabled: SENTINEL_VAULT_HMAC_KEY and SENTINEL_VAULT_AES_KEY must be set (32+ bytes, base64 or hex)")
			return
		}
		hk, err := decodeKey(h, 32)
		if err != nil {
			vaultKeyErr = fmt.Errorf("SENTINEL_VAULT_HMAC_KEY: %w", err)
			return
		}
		ak, err := decodeKey(a, 32)
		if err != nil {
			vaultKeyErr = fmt.Errorf("SENTINEL_VAULT_AES_KEY: %w", err)
			return
		}
		vaultHMACKey, vaultAEADKey = hk, ak
	})
	return vaultHMACKey, vaultAEADKey, vaultKeyErr
}

func decodeKey(s string, want int) ([]byte, error) {
	if b, err := hex.DecodeString(s); err == nil && len(b) >= want {
		return b[:want], nil
	}
	if b, err := base64.StdEncoding.DecodeString(s); err == nil && len(b) >= want {
		return b[:want], nil
	}
	return nil, fmt.Errorf("must decode (hex or base64) to at least %d bytes", want)
}

func sealValue(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func openValue(key, ct []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ct) < gcm.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	return gcm.Open(nil, ct[:gcm.NonceSize()], ct[gcm.NonceSize():], nil)
}

func typeAbbrev(fieldType string) string {
	t := strings.ToUpper(strings.TrimSpace(fieldType))
	if t == "" {
		return "GEN"
	}
	if len(t) < 3 {
		return t + strings.Repeat("X", 3-len(t))
	}
	return t[:3]
}

func policyFor(tenantID string) *TokenizationPolicy {
	GlobalVault.mu.RLock()
	defer GlobalVault.mu.RUnlock()
	for i := range GlobalVault.Policies {
		if GlobalVault.Policies[i].TenantID == tenantID {
			return &GlobalVault.Policies[i]
		}
	}
	return nil
}

// TokenizeField produces a masked display value and a keyed token pointer, and
// stores the raw value AES-256-GCM encrypted with a retention deadline.
func TokenizeField(tenantID string, fieldType string, rawValue string) (TokenizedRecord, error) {
	hmacKey, aesKey, err := loadVaultKeys()
	if err != nil {
		return TokenizedRecord{}, err
	}

	trimmed := strings.TrimSpace(rawValue)
	masked := maskValue(fieldType, trimmed)

	h := hmac.New(sha256.New, hmacKey)
	h.Write([]byte(tenantID + ":" + fieldType + ":" + trimmed))
	tokenKey := "TOK-" + typeAbbrev(fieldType) + "-" + hex.EncodeToString(h.Sum(nil))[:12]

	ct, err := sealValue(aesKey, []byte(trimmed))
	if err != nil {
		return TokenizedRecord{}, fmt.Errorf("vault seal failed: %w", err)
	}

	retention := 2555
	if p := policyFor(tenantID); p != nil && p.RetentionDays > 0 {
		retention = p.RetentionDays
	}

	GlobalVault.mu.Lock()
	GlobalVault.entries[tokenKey] = vaultEntry{
		ciphertext: ct,
		expiresAt:  time.Now().UTC().AddDate(0, 0, retention),
	}
	GlobalVault.mu.Unlock()

	return TokenizedRecord{
		OriginalType: fieldType,
		MaskedValue:  masked,
		TokenKey:     tokenKey,
		TenantID:     tenantID,
		CreatedAt:    time.Now().UTC(),
	}, nil
}

func maskValue(fieldType, trimmed string) string {
	switch {
	case fieldType == "ROUTING_NUMBER" && len(trimmed) == 9:
		return trimmed[0:4] + "****" + trimmed[8:9]
	case fieldType == "ACCOUNT_NUMBER" && len(trimmed) >= 4:
		return strings.Repeat("*", len(trimmed)-4) + trimmed[len(trimmed)-4:]
	case fieldType == "INDIVIDUAL_NAME":
		parts := strings.Split(trimmed, " ")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if len(p) > 1 {
				out = append(out, p[0:1]+strings.Repeat("*", len(p)-1))
			} else {
				out = append(out, p)
			}
		}
		return strings.Join(out, " ")
	default:
		return "******** (REDACTED)"
	}
}

// authorizeDetokenize enforces a shared-secret supervisor credential in constant
// time. This is a minimum bar, not a substitute for real mTLS client-cert or
// OIDC identity; it exists so the endpoint is not open to the world.
func authorizeDetokenize(r *http.Request) error {
	expected := os.Getenv("SENTINEL_SUPERVISOR_TOKEN")
	if expected == "" {
		return errors.New("detokenization disabled: SENTINEL_SUPERVISOR_TOKEN is not configured")
	}
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
		return errors.New("detokenization denied: invalid supervisor credential")
	}
	return nil
}

func RegisterVaultRoutes(r chi.Router, db *sql.DB) {
	r.Route("/vault", func(r chi.Router) {
		r.Post("/tokenize", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				TenantID  string `json:"tenantId"`
				FieldType string `json:"fieldType"`
				RawValue  string `json:"rawValue"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid request body", http.StatusBadRequest)
				return
			}
			record, err := TokenizeField(body.TenantID, body.FieldType, body.RawValue)
			if err != nil {
				http.Error(w, err.Error(), http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(record)
		})

		r.Get("/policies", func(w http.ResponseWriter, r *http.Request) {
			GlobalVault.mu.RLock()
			defer GlobalVault.mu.RUnlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(GlobalVault.Policies)
		})

		r.Post("/detokenize", func(w http.ResponseWriter, r *http.Request) {
			if err := authorizeDetokenize(r); err != nil {
				// Log the denied attempt BEFORE returning.
				_, _ = AppendAuditEvent(db, "VAULT_DETOKENIZE_DENIED", "unauthenticated",
					map[string]interface{}{"reason": err.Error(), "remoteAddr": r.RemoteAddr})
				http.Error(w, err.Error(), http.StatusUnauthorized)
				return
			}

			var body struct {
				TokenKey     string `json:"tokenKey"`
				SupervisorID string `json:"supervisorId"`
				AuditReason  string `json:"auditReason"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid detokenize request", http.StatusBadRequest)
				return
			}
			if body.SupervisorID == "" || len(body.AuditReason) < 10 {
				http.Error(w, "supervisor id and a justification of >=10 chars are required", http.StatusForbidden)
				return
			}

			GlobalVault.mu.RLock()
			entry, exists := GlobalVault.entries[body.TokenKey]
			GlobalVault.mu.RUnlock()
			if !exists {
				http.Error(w, "token not found", http.StatusNotFound)
				return
			}
			if time.Now().UTC().After(entry.expiresAt) {
				http.Error(w, "token past retention deadline", http.StatusGone)
				return
			}

			_, aesKey, err := loadVaultKeys()
			if err != nil {
				http.Error(w, err.Error(), http.StatusServiceUnavailable)
				return
			}
			plaintext, err := openValue(aesKey, entry.ciphertext)
			if err != nil {
				http.Error(w, "vault decryption failed", http.StatusInternalServerError)
				return
			}

			// Write the audit record and only claim auditLogged if it succeeded.
			ev, auditErr := AppendAuditEvent(db, "VAULT_DETOKENIZE", body.SupervisorID,
				map[string]interface{}{
					"tokenKey":    body.TokenKey,
					"auditReason": body.AuditReason,
					"remoteAddr":  r.RemoteAddr,
				})
			if auditErr != nil {
				// Fail closed: no audit record means no disclosure.
				http.Error(w, "refusing to disclose: audit ledger write failed", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"tokenKey":     body.TokenKey,
				"detokenized":  string(plaintext),
				"supervisorId": body.SupervisorID,
				"auditLogged":  true,
				"auditEventId": ev.ID,
				"auditHash":    ev.CurrentHash,
				"accessedAt":   time.Now().UTC().Format(time.RFC3339),
			})
		})
	})
}
