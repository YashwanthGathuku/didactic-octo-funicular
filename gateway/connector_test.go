package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestIntegrationHubSanitizedConnections(t *testing.T) {
	r := chi.NewRouter()
	RegisterIntegrationHubRoutes(r, nil)

	req := httptest.NewRequest("GET", "/hub/connections", nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected HTTP 200 from /hub/connections, got %d", rr.Code)
	}

	body := rr.Body.String()

	// Strict OWASP Secrets Management Check:
	// Verify that raw passwords, raw private keys, and decrypted tokens NEVER appear in the response payload
	forbiddenKeywords := []string{"password", "BEGIN RSA PRIVATE KEY", "BEGIN OPENSSH PRIVATE KEY", "client_secret_value"}
	for _, kw := range forbiddenKeywords {
		if strings.Contains(strings.ToLower(body), strings.ToLower(kw)) {
			t.Errorf("Security Violation: Found forbidden secret keyword '%s' in connection payload", kw)
		}
	}

	var conns []Connection
	if err := json.Unmarshal([]byte(body), &conns); err != nil {
		t.Fatalf("Failed to parse connections JSON: %v", err)
	}

	if len(conns) < 3 {
		t.Errorf("Expected at least 3 registered connections, got %d", len(conns))
	}

	// Verify secret reference is present as a decoupled pointer
	for _, c := range conns {
		if !strings.HasPrefix(c.SecretRef.VaultKey, "vault://") && !strings.HasPrefix(c.SecretRef.VaultKey, "aws://") {
			t.Errorf("Expected decoupled Vault/AWS secret pointer, got %s", c.SecretRef.VaultKey)
		}
	}
}

func TestCatalogAssetsAndMaskedPreview(t *testing.T) {
	r := chi.NewRouter()
	RegisterIntegrationHubRoutes(r, nil)

	// Test GET /hub/assets
	req := httptest.NewRequest("GET", "/hub/assets", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected HTTP 200, got %d", rr.Code)
	}

	var assets []CatalogAsset
	if err := json.Unmarshal(rr.Body.Bytes(), &assets); err != nil {
		t.Fatalf("Failed to parse assets: %v", err)
	}
	if len(assets) < 3 {
		t.Errorf("Expected at least 3 catalog assets, got %d", len(assets))
	}

	// Test GET /hub/assets/ASSET-001/sample (Masked PII)
	reqSample := httptest.NewRequest("GET", "/hub/assets/ASSET-001/sample", nil)
	rrSample := httptest.NewRecorder()
	r.ServeHTTP(rrSample, reqSample)

	if rrSample.Code != http.StatusOK {
		t.Fatalf("Expected HTTP 200 for masked sample, got %d", rrSample.Code)
	}

	sampleBody := rrSample.Body.String()
	if !strings.Contains(sampleBody, "(MASKED)") && !strings.Contains(sampleBody, "(REDACTED)") {
		t.Errorf("Expected PII masking in sample payload, got: %s", sampleBody)
	}
}

func TestDataLineageGraph(t *testing.T) {
	r := chi.NewRouter()
	RegisterIntegrationHubRoutes(r, nil)

	req := httptest.NewRequest("GET", "/hub/lineage", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected HTTP 200 for lineage, got %d", rr.Code)
	}

	var lineageMap map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &lineageMap); err != nil {
		t.Fatalf("Failed to parse lineage DAG: %v", err)
	}

	if lineageMap["edges"] == nil || lineageMap["nodes"] == nil {
		t.Errorf("Lineage DAG missing nodes or edges")
	}
}
