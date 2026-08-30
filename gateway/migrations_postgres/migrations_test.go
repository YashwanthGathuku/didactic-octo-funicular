package migrations_postgres_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestPostgresMigrationParity checks that all migrations from 001 through 023
// exist in migrations_postgres/ and match the SQLite migration count and numbering.
func TestPostgresMigrationParity(t *testing.T) {
	sqliteDir := filepath.Join("..", "migrations")
	pgDir := "."

	sqliteEntries, err := os.ReadDir(sqliteDir)
	if err != nil {
		t.Fatalf("read sqlite migrations dir: %v", err)
	}

	pgEntries, err := os.ReadDir(pgDir)
	if err != nil {
		t.Fatalf("read postgres migrations dir: %v", err)
	}

	var sqliteFiles, pgFiles []string
	for _, e := range sqliteEntries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			sqliteFiles = append(sqliteFiles, e.Name())
		}
	}
	for _, e := range pgEntries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			pgFiles = append(pgFiles, e.Name())
		}
	}

	sort.Strings(sqliteFiles)
	sort.Strings(pgFiles)

	if len(sqliteFiles) != 24 {
		t.Errorf("expected 24 SQLite migrations, found %d", len(sqliteFiles))
	}
	if len(pgFiles) != 24 {
		t.Errorf("expected 24 PostgreSQL migrations, found %d: %v", len(pgFiles), pgFiles)
	}

	// Verify all 001 through 024 prefixes exist in both
	for i := 1; i <= 24; i++ {
		prefix := fmt.Sprintf("%03d_", i)
		hasSqlite := false
		for _, f := range sqliteFiles {
			if strings.HasPrefix(f, prefix) {
				hasSqlite = true
				break
			}
		}
		if !hasSqlite {
			t.Errorf("missing SQLite migration for prefix %s", prefix)
		}

		hasPg := false
		for _, f := range pgFiles {
			if strings.HasPrefix(f, prefix) {
				hasPg = true
				break
			}
		}
		if !hasPg {
			t.Errorf("missing PostgreSQL migration for prefix %s", prefix)
		}
	}
}

// TestPostgresMigrationDialectAndSyntax verifies PostgreSQL dialect requirements across all migrations:
// - No SQLite-specific keywords (AUTOINCREMENT, BLOB, DATETIME without timezone)
// - Uses TIMESTAMPTZ for timestamps
// - Uses BYTEA for binary data
// - Proper PL/pgSQL function syntax
// - Balanced quotes and dollar-quotes
func TestPostgresMigrationDialectAndSyntax(t *testing.T) {
	pgDir := "."
	entries, err := os.ReadDir(pgDir)
	if err != nil {
		t.Fatalf("read postgres migrations dir: %v", err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}

		t.Run(e.Name(), func(t *testing.T) {
			contentBytes, err := os.ReadFile(filepath.Join(pgDir, e.Name()))
			if err != nil {
				t.Fatalf("read file %s: %v", e.Name(), err)
			}
			content := string(contentBytes)
			codeOnly := stripComments(content)

			// 1. Prohibited SQLite keywords in actual SQL code
			if regexp.MustCompile(`(?i)\bAUTOINCREMENT\b`).MatchString(codeOnly) {
				t.Errorf("%s contains SQLite 'AUTOINCREMENT'; use 'GENERATED ALWAYS AS IDENTITY'", e.Name())
			}
			if regexp.MustCompile(`(?i)\bBLOB\b`).MatchString(codeOnly) {
				t.Errorf("%s contains SQLite 'BLOB'; use 'BYTEA'", e.Name())
			}
			if regexp.MustCompile(`(?i)\bDATETIME\b`).MatchString(codeOnly) {
				t.Errorf("%s contains 'DATETIME'; use 'TIMESTAMPTZ'", e.Name())
			}
			if regexp.MustCompile(`(?i)\bINSERT\s+OR\s+IGNORE\b`).MatchString(codeOnly) {
				t.Errorf("%s contains SQLite 'INSERT OR IGNORE'; use 'ON CONFLICT DO NOTHING'", e.Name())
			}
			if regexp.MustCompile(`(?i)\bRAISE\s*\(\s*ABORT\b`).MatchString(codeOnly) {
				t.Errorf("%s contains SQLite 'RAISE(ABORT)'; use PL/pgSQL 'RAISE EXCEPTION'", e.Name())
			}

			// 2. Syntax integrity: Dollar quoting balance
			dollarQuotes := regexp.MustCompile(`\$\$`).FindAllString(content, -1)
			if len(dollarQuotes)%2 != 0 {
				t.Errorf("%s has unbalanced $$ dollar quotes (%d count)", e.Name(), len(dollarQuotes))
			}

			// 3. Syntax integrity: Parenthesis balance outside comments and string literals
			cleaned := stripCommentsAndStrings(content)
			openParen := strings.Count(cleaned, "(")
			closeParen := strings.Count(cleaned, ")")
			if openParen != closeParen {
				t.Errorf("%s has unbalanced parentheses: %d '(' vs %d ')'", e.Name(), openParen, closeParen)
			}
		})
	}
}

// TestPostgresTenantRLSAndPolicyCompleteness checks that all tenant-bound tables
// have ENABLE ROW LEVEL SECURITY, FORCE ROW LEVEL SECURITY, and tenant_isolation policies.
func TestPostgresTenantRLSAndPolicyCompleteness(t *testing.T) {
	pgDir := "."
	entries, err := os.ReadDir(pgDir)
	if err != nil {
		t.Fatalf("read postgres migrations dir: %v", err)
	}

	createTableRegex := regexp.MustCompile(`(?i)CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-zA-Z0-9_]+)\s*\(([\s\S]*?)\);`)

	// Tables known to be global or system tables without tenant_id or with special policy
	globalTables := map[string]bool{
		"tenants":                true,
		"policy_bundle_versions": true,
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}

		contentBytes, err := os.ReadFile(filepath.Join(pgDir, e.Name()))
		if err != nil {
			t.Fatalf("read file %s: %v", e.Name(), err)
		}
		content := string(contentBytes)

		matches := createTableRegex.FindAllStringSubmatch(content, -1)
		for _, m := range matches {
			tableName := strings.ToLower(m[1])
			tableBody := m[2]

			if globalTables[tableName] {
				// Global table: verify PUBLIC grant or appropriate handling
				if tableName == "tenants" {
					if !strings.Contains(content, "GRANT SELECT ON tenants TO PUBLIC") {
						t.Errorf("%s creates tenants table but missing GRANT SELECT ON tenants TO PUBLIC", e.Name())
					}
				}
				if tableName == "policy_bundle_versions" {
					if !strings.Contains(content, "GRANT SELECT ON policy_bundle_versions TO PUBLIC") {
						t.Errorf("%s creates policy_bundle_versions but missing GRANT SELECT ON policy_bundle_versions TO PUBLIC", e.Name())
					}
				}
				continue
			}

			// If the table contains tenant_id, it MUST have RLS enabled, forced, and policy defined
			if strings.Contains(strings.ToLower(tableBody), "tenant_id") {
				enablePattern := fmt.Sprintf(`(?i)ALTER\s+TABLE\s+%s\s+ENABLE\s+ROW\s+LEVEL\s+SECURITY`, tableName)
				if !regexp.MustCompile(enablePattern).MatchString(content) {
					t.Errorf("%s creates table %s with tenant_id but missing ENABLE ROW LEVEL SECURITY", e.Name(), tableName)
				}

				forcePattern := fmt.Sprintf(`(?i)ALTER\s+TABLE\s+%s\s+FORCE\s+ROW\s+LEVEL\s+SECURITY`, tableName)
				if !regexp.MustCompile(forcePattern).MatchString(content) {
					t.Errorf("%s creates table %s with tenant_id but missing FORCE ROW LEVEL SECURITY", e.Name(), tableName)
				}

				policyPattern := fmt.Sprintf(`(?i)CREATE\s+POLICY\s+tenant_isolation_%s\s+ON\s+%s`, tableName, tableName)
				if !regexp.MustCompile(policyPattern).MatchString(content) {
					t.Errorf("%s creates table %s with tenant_id but missing policy tenant_isolation_%s", e.Name(), tableName, tableName)
				}

				// Check that policy uses current_setting('sentinel.tenant_id', true)
				if !strings.Contains(content, "current_setting('sentinel.tenant_id', true)") {
					t.Errorf("%s policy for table %s does not use current_setting('sentinel.tenant_id', true)", e.Name(), tableName)
				}
			}
		}
	}
}

// TestPostgresLensLiteParity validates Lens Lite tables (023_lens_lite.sql)
// for exact schema constraints, triggers, and foreign keys.
func TestPostgresLensLiteParity(t *testing.T) {
	contentBytes, err := os.ReadFile("023_lens_lite.sql")
	if err != nil {
		t.Fatalf("read 023_lens_lite.sql: %v", err)
	}
	content := string(contentBytes)

	requiredTables := []string{
		"lens_return_events",
		"lens_investigations",
		"lens_investigation_nodes",
	}

	for _, tbl := range requiredTables {
		if !strings.Contains(content, tbl) {
			t.Errorf("023_lens_lite.sql missing required table %s", tbl)
		}
		enableRLS := fmt.Sprintf(`(?i)ALTER\s+TABLE\s+%s\s+ENABLE\s+ROW\s+LEVEL\s+SECURITY`, tbl)
		forceRLS := fmt.Sprintf(`(?i)ALTER\s+TABLE\s+%s\s+FORCE\s+ROW\s+LEVEL\s+SECURITY`, tbl)
		policy := fmt.Sprintf(`(?i)CREATE\s+POLICY\s+tenant_isolation_%s\s+ON\s+%s`, tbl, tbl)

		if !regexp.MustCompile(enableRLS).MatchString(content) {
			t.Errorf("023_lens_lite.sql missing ENABLE ROW LEVEL SECURITY on %s", tbl)
		}
		if !regexp.MustCompile(forceRLS).MatchString(content) {
			t.Errorf("023_lens_lite.sql missing FORCE ROW LEVEL SECURITY on %s", tbl)
		}
		if !regexp.MustCompile(policy).MatchString(content) {
			t.Errorf("023_lens_lite.sql missing policy on %s", tbl)
		}
	}

	// Verify synthetic demo check constraint
	if !strings.Contains(content, "CHECK (NOT (source_type = 'SYNTHETIC_DEMO' AND verified = 1))") {
		t.Errorf("023_lens_lite.sql missing synthetic demo verification check constraint")
	}

	// Verify append-only trigger for lens_investigation_nodes
	if !strings.Contains(content, "lens_investigation_nodes_append_only") {
		t.Errorf("023_lens_lite.sql missing append-only trigger function for lens_investigation_nodes")
	}
}

// Helper to remove comments when checking SQL keywords
func stripComments(s string) string {
	s = regexp.MustCompile(`--.*`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`/\*[\s\S]*?\*/`).ReplaceAllString(s, "")
	return s
}

// Helper to remove comments and string literals when checking parenthesis balancing.
func stripCommentsAndStrings(s string) string {
	// Remove single-line comments -- ...
	s = regexp.MustCompile(`--.*`).ReplaceAllString(s, "")
	// Remove /* ... */ comments
	s = regexp.MustCompile(`/\*[\s\S]*?\*/`).ReplaceAllString(s, "")
	// Remove $$ ... $$ function bodies
	s = regexp.MustCompile(`\$\$[\s\S]*?\$\$`).ReplaceAllString(s, "")
	// Remove single-quoted strings '...'
	s = regexp.MustCompile(`'([^'\\]|\\.)*'`).ReplaceAllString(s, "")
	return s
}
