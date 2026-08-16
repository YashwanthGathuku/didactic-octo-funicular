package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sentinel-gateway/internal/connectors"
)

// The connector platform over HTTP.
//
// The property that matters most is the one the Integration Hub failed: an
// unimplemented connector must be visible and unusable, and nothing may report
// a successful connection it did not make.

func connectorEnv(t *testing.T) http.Handler {
	t.Helper()
	db := setupTestDb(t)
	t.Cleanup(func() { db.Close() })
	return NewRouter(db, ingestDemoConfig(), nil)
}

func getJSON(t *testing.T, handler http.Handler, path string) (int, map[string]any, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec.Code, body, rec.Body.String()
}

func TestTheCatalogListsEveryConnectorWithAnHonestStatus(t *testing.T) {
	handler := connectorEnv(t)

	code, body, raw := getJSON(t, handler, "/api/v1/connectors")
	if code != http.StatusOK {
		t.Fatalf("GET /connectors: %d %s", code, raw)
	}

	list, _ := body["connectors"].([]any)
	if len(list) != 9 {
		t.Fatalf("the catalog has %d entries, want the nine the guide names", len(list))
	}

	want := map[string]bool{
		"postgresql": false, "mysql": false, "mariadb": false, "sqlserver": false,
		"oracle": false, "snowflake": false, "redshift": false,
		"bigquery": false, "databricks": false,
	}
	for _, item := range list {
		entry, _ := item.(map[string]any)
		typ, _ := entry["type"].(string)
		if _, ok := want[typ]; !ok {
			t.Errorf("unexpected catalog entry %q", typ)
			continue
		}
		want[typ] = true

		status, _ := entry["status"].(string)
		selectable, _ := entry["selectable"].(bool)
		reason, _ := entry["statusReason"].(string)

		if selectable && status != "AVAILABLE" {
			t.Errorf("%s is selectable at status %s; only AVAILABLE may be", typ, status)
		}
		if !selectable && strings.TrimSpace(reason) == "" {
			t.Errorf("%s is not selectable and gives no reason; an operator cannot tell "+
				"'not built' from 'broken'", typ)
		}
		// The evidence field must be absent unless the entry is available.
		if entry["conformance"] != nil && status != "AVAILABLE" {
			t.Errorf("%s carries a conformance record at status %s", typ, status)
		}
	}
	for typ, seen := range want {
		if !seen {
			t.Errorf("the catalog is missing %s", typ)
		}
	}
}

// A connector with no conformance evidence must not be reachable, whatever a
// client asks for.
func TestAnUnverifiedConnectorCannotBeUsed(t *testing.T) {
	r := connectors.NewRegistry()
	for _, d := range r.Catalog() {
		if _, _, err := r.Driver(d.Type); err == nil {
			t.Errorf("%s is reachable with no driver registered", d.Type)
		}
	}

	// Even with a driver attached, absent evidence it stays unreachable.
	if err := r.Register(connectors.NewPostgresConnector(), nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.Driver("postgresql"); err == nil {
		t.Error("a driver with no conformance record must not be reachable")
	}
}

func TestDescriptorsDriveTheFormAndNeverReturnASecretValue(t *testing.T) {
	handler := connectorEnv(t)

	for _, typ := range []string{
		"postgresql", "mysql", "mariadb", "sqlserver", "oracle",
		"snowflake", "redshift", "bigquery", "databricks",
	} {
		code, body, raw := getJSON(t, handler, "/api/v1/connectors/"+typ)
		if code != http.StatusOK {
			t.Fatalf("GET /connectors/%s: %d %s", typ, code, raw)
		}

		fields, _ := body["fields"].([]any)
		if len(fields) == 0 {
			t.Errorf("%s has no fields, so the wizard would render an empty form", typ)
		}
		modes, _ := body["authModes"].([]any)
		if len(modes) == 0 {
			t.Errorf("%s declares no authentication modes", typ)
		}

		var hasSecretField bool
		for _, f := range fields {
			field, _ := f.(map[string]any)
			kind, _ := field["kind"].(string)
			if kind == "SECRET" || kind == "SECRET_REF" {
				hasSecretField = true
			}
			// A descriptor is field metadata. It must carry no value at all
			// beyond a default and a placeholder.
			for _, forbidden := range []string{"value", "currentValue", "savedValue"} {
				if _, ok := field[forbidden]; ok {
					t.Errorf("%s field carries %q; a descriptor is metadata, not data", typ, forbidden)
				}
			}
		}
		if !hasSecretField {
			t.Errorf("%s declares no credential field; every supported database needs one", typ)
		}

		// The template is documentation. It must be a placeholder form.
		if tmpl, ok := body["template"].(string); ok && tmpl != "" {
			if !strings.Contains(tmpl, "<") {
				t.Errorf("%s template %q has no placeholders", typ, tmpl)
			}
		}
	}
}

// BigQuery, Snowflake and Databricks must not offer a paste box.
func TestUriPasteIsOnlyOfferedWhereAUriExists(t *testing.T) {
	handler := connectorEnv(t)

	for typ, want := range map[string]bool{
		"postgresql": true, "mysql": true, "mariadb": true,
		"sqlserver": true, "oracle": true, "redshift": true,
		"bigquery": false, "snowflake": false, "databricks": false,
	} {
		_, body, _ := getJSON(t, handler, "/api/v1/connectors/"+typ)
		got, _ := body["supportsUriPaste"].(bool)
		if got != want {
			t.Errorf("%s supportsUriPaste = %v, want %v", typ, got, want)
		}
	}
}

func TestPastedConnectionStringIsSplitAndNeverEchoed(t *testing.T) {
	handler := connectorEnv(t)

	// secret-scan-allow: a fixture credential in an HTTP test; it authenticates nothing and exists so the response can be searched for it
	const credential = "http-fixture-credential-8c31-not-a-real-password"
	uri := "postgresql://svc_reporting:" + credential + "@db.example.test:5432/ledger?sslmode=verify-full"

	payload, _ := json.Marshal(map[string]string{"uri": uri})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/connectors/postgresql/parse-uri",
		strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("parse-uri: %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, credential) {
		t.Fatal("the response echoes the credential from the pasted connection string")
	}
	if strings.Contains(body, uri) {
		t.Fatal("the response echoes the whole connection string")
	}

	var parsed struct {
		Fields          map[string]string `json:"fields"`
		SecretExtracted bool              `json:"secretExtracted"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
		t.Fatal(err)
	}
	if !parsed.SecretExtracted {
		t.Error("the credential was not extracted")
	}
	if parsed.Fields["host"] != "db.example.test" || parsed.Fields["database"] != "ledger" {
		t.Errorf("the string was not split correctly: %v", parsed.Fields)
	}
	for _, v := range parsed.Fields {
		if strings.Contains(v, credential) {
			t.Error("a parsed field carries the credential")
		}
	}
}

// Nothing in the connector surface may report a connection it did not make.
func TestNoConnectorRouteReportsAnUnverifiedSuccess(t *testing.T) {
	handler := connectorEnv(t)

	_, _, raw := getJSON(t, handler, "/api/v1/connectors")
	for _, forbidden := range []string{
		`"connected":true`, `"healthy":true`, `"mTLSVerified":true`,
		`"verified":true`, `"HEALTHY"`,
	} {
		if strings.Contains(raw, forbidden) {
			t.Errorf("the catalog response contains %s although no connection has been attempted", forbidden)
		}
	}
}
