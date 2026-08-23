package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Prompt 05 requires a scan proving no default or literal secret exists in
// source or configuration fixtures.
//
// This is a test rather than a CI-only tool on purpose. gitleaks runs in CI and
// catches material that matches a published detector; this catches the thing
// gitleaks is weakest at -- a credential that looks like ordinary text because
// it was chosen by a developer rather than issued by a provider. `password`,
// `minioadmin` and `changeme` were all present in this repository's compose
// files, and none of them has the entropy a scanner looks for.

// repoRoot is the parent of the gateway module.
const repoRoot = ".."

// scannedExtensions are the file types that can carry a credential into a
// deployment. Anything not listed is skipped.
var scannedExtensions = map[string]bool{
	".go": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true,
	".py": true, ".sql": true, ".yml": true, ".yaml": true, ".json": true,
	".toml": true, ".ini": true, ".env": true, ".sh": true, ".conf": true,
	"": true, // extensionless: Containerfile, Dockerfile, Makefile
}

// skippedDirs are excluded, each for a stated reason. An unexplained exclusion
// in a security scan is where the finding hides.
var skippedDirs = map[string]string{
	".git":         "object storage, not source",
	"node_modules": "third-party dependencies; covered by dependency scanning instead",
	"dist":         "build output",
	"build":        "build output",
	"data":         "runtime database files",
	"inbox":        "runtime input directory",
	".venv":        "third-party dependencies",
	"__pycache__":  "build output",
	"evals":        "adversarial evaluation test suites and runners",

	// docs/engineering holds the verbatim archives of code removed in Prompt 01
	// -- including the webhook subsystem that generated secrets from
	// time.Now().UnixNano() and returned them in responses. Those archives are
	// the record of the defect and necessarily contain its source. They are not
	// built, not imported and not deployed.
	"docs": "documentation, including the verbatim archive of removed code",
}

// wellKnownDefaults are credentials that shipped in this repository's compose
// files. A scanner keyed on entropy does not flag any of them.
var wellKnownDefaults = []string{
	"minioadmin", "changeme", "hunter2", "letmein",
	"admin:admin", "postgres:postgres", "root:root",
	"password123", "sentinel123",
}

// assignmentPatterns match a credential-shaped name being given a literal.
//
// The name is what carries the signal. A high-entropy string on its own is
// usually a hash, a fixture or a base64 asset; the same string assigned to
// something called `apiToken` is a credential.
//
// Patterns are scoped by file type. The unquoted `name: value` form is how YAML
// and env files express an assignment, and how Go expresses a struct field
// holding a variable -- so applying it to Go source reports `SecretKey:
// secretKey` as a literal credential. Scoping is what keeps the scan's findings
// worth reading.
var assignmentPatterns = []*regexp.Regexp{
	// Go, TypeScript, Python: token = "literal", Token: "literal"
	regexp.MustCompile(`(?i)\b(\w*(?:password|passwd|secret|token|apikey|api_key|credential|passphrase|private_key|privatekey)\w*)\s*[:=]+\s*"([^"]{8,})"`),
	// Shell and env files: SECRET=literal
	regexp.MustCompile(`(?i)^\s*(?:export\s+)?(\w*(?:PASSWORD|SECRET|TOKEN|APIKEY|API_KEY|CREDENTIAL|PASSPHRASE)\w*)\s*=\s*([^\s#$'"]{8,})\s*$`),
	// SQL: CREATE ROLE ... PASSWORD 'literal'. A distinct form from the
	// assignment patterns above, and one that lands in a migration file.
	regexp.MustCompile(`(?i)\b(password)\s+'([^']{8,})'`),
}

// unquotedAssignmentPatterns apply only to configuration formats, where an
// unquoted bare word after a colon really is a value rather than a variable.
var unquotedAssignmentPatterns = []*regexp.Regexp{
	// YAML: password: literal
	regexp.MustCompile(`(?i)^\s*(\w*(?:password|secret|token|apikey|api_key|credential|passphrase)\w*)\s*:\s*([^\s#{$\[]{8,})\s*$`),
}

// configExtensions are the file types the unquoted patterns apply to.
var configExtensions = map[string]bool{
	".yml": true, ".yaml": true, ".env": true, ".ini": true,
	".conf": true, ".toml": true, "": true,
}

// nonCredentialFields are field names that contain substrings like 'token'
// but represent counts, tokenizers, or tenant scopes rather than credentials.
var nonCredentialFields = map[string]bool{
	"token":              true,
	"prompt_tokens":      true,
	"completion_tokens":  true,
	"total_tokens":       true,
	"max_output_tokens":  true,
	"q_tokens":           true,
	"f_tokens":           true,
	"tenant_scope_token": true,
}

// benignValues are the right-hand sides that are not credentials. Each entry is
// a placeholder, an environment reference or a well-known non-secret.
var benignValues = regexp.MustCompile(`(?i)^(` +
	`\$\{.*|` + // ${VAR} interpolation
	`\$\(.*|` + // $(command)
	`process\.env\..*|os\.getenv.*|os\.environ.*|` +
	`env\(.*|Getenv\(.*|` +
	`\[REDACTED\]|\[UNSET\]|` +
	`(x-)?(csrf|xsrf)-token|authorization|bearer|basic|` +
	`(change|replace|set)[-_ ]?(me|this)|` +
	`your[-_].*|example[-_].*|placeholder.*|redacted.*|` +
	`<[^>]+>|` + // <your-token-here>
	`mock-.*|test-.*|` +
	`true|false|null|none|nil|undefined` +
	`)$`)

func TestNoLiteralSecretsInSourceOrConfig(t *testing.T) {
	var findings []string

	err := filepath.Walk(repoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		name := info.Name()
		if info.IsDir() {
			if _, skip := skippedDirs[name]; skip {
				return filepath.SkipDir
			}
			return nil
		}

		// Test files are excluded: a test needs literal credential-shaped
		// strings to prove that credentials do not leak, and this file itself
		// contains a list of well-known passwords.
		if strings.HasSuffix(name, "_test.go") || strings.Contains(name, ".test.") ||
			strings.Contains(name, ".spec.") || strings.HasSuffix(name, "_test.py") ||
			strings.HasPrefix(name, "test_") {
			return nil
		}
		if !scannedExtensions[strings.ToLower(filepath.Ext(name))] {
			return nil
		}
		// package-lock.json is a dependency manifest of hashes.
		if name == "package-lock.json" || name == "go.sum" {
			return nil
		}

		body, err := os.ReadFile(path)
		if err != nil {
			return nil // unreadable files are not a hygiene finding
		}
		rel, _ := filepath.Rel(repoRoot, path)
		findings = append(findings, scanFile(rel, string(body))...)
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	if len(findings) > 0 {
		t.Errorf("literal credential material found in source or configuration:\n  - %s",
			strings.Join(findings, "\n  - "))
	}
}

// reAllowMarker suppresses a finding on the line it appears on, or on the line
// immediately following it.
//
// It is an annotation rather than a path exclusion because the reason has to be
// written down. A directory added to a skip list stays skipped forever and
// nobody revisits it; a marker with a stated reason is visible in the diff that
// adds it and in the file it protects. The reason is mandatory -- a bare marker
// does not suppress anything.
//
// The legitimate use is narrow: code that names credential shapes in order to
// detect or refuse them. A redaction pattern, a rejection list and a detector
// regex all look exactly like the thing they defend against.
var reAllowMarker = regexp.MustCompile(`secret-scan-allow:\s*(\S.*)`)

func allowedOnLine(lines []string, i int) bool {
	check := func(n int) bool {
		if n < 0 || n >= len(lines) {
			return false
		}
		m := reAllowMarker.FindStringSubmatch(lines[n])
		return m != nil && len(strings.TrimSpace(m[1])) >= 10 // a reason, not a word
	}
	return check(i) || check(i-1)
}

func scanFile(rel, body string) []string {
	var findings []string
	lines := strings.Split(body, "\n")

	// A private key committed to a repository is unambiguous.
	if strings.Contains(body, "-----BEGIN") && strings.Contains(body, "PRIVATE KEY-----") {
		suppressed := false
		for i, line := range lines {
			if strings.Contains(line, "-----BEGIN") && allowedOnLine(lines, i) {
				suppressed = true
			}
		}
		if !suppressed {
			findings = append(findings, rel+": contains a PEM private key block")
		}
	}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || isComment(trimmed) || allowedOnLine(lines, i) {
			continue
		}

		for _, bad := range wellKnownDefaults {
			if strings.Contains(strings.ToLower(trimmed), bad) {
				findings = append(findings, fmtFinding(rel, i+1, "well-known default credential "+bad))
			}
		}

		patterns := assignmentPatterns
		if configExtensions[strings.ToLower(filepath.Ext(rel))] {
			patterns = append(append([]*regexp.Regexp{}, patterns...), unquotedAssignmentPatterns...)
		}
		for _, re := range patterns {
			m := re.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			if nonCredentialFields[strings.ToLower(m[1])] {
				continue
			}
			// Strip surrounding quotes before the benign check: a YAML value is
			// captured with them and a quoted placeholder would otherwise read
			// as a literal.
			value := strings.Trim(strings.TrimSpace(m[2]), `"'`)
			if benignValues.MatchString(value) {
				continue
			}
			findings = append(findings, fmtFinding(rel, i+1,
				"the field "+m[1]+" is assigned a literal value"))
		}
	}
	return findings
}

func isComment(line string) bool {
	for _, prefix := range []string{"//", "#", "--", "/*", "*"} {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

func fmtFinding(rel string, line int, why string) string {
	return rel + ":" + itoa(line) + ": " + why
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// The scanner must actually detect the things it claims to. A hygiene test that
// passes because its patterns match nothing is worse than no test: it reports a
// clean result it did not earn.
func TestTheSecretScannerDetectsWhatItClaimsTo(t *testing.T) {
	cases := []struct {
		name string
		file string
		body string
	}{
		{"a Go string literal", "fixture.go", `apiToken := "s0me-rea1-token-value-here"`},
		{"a Go struct field", "fixture.go", `cfg := Config{APIToken: "s0me-rea1-token-value-here"}`},
		{"a shell export", "fixture.sh", "export SENTINEL_API_TOKEN=s0me-rea1-token-value"},
		{"a YAML value", "compose.yml", "  password: supersecretvalue"},
		{"a well-known default", "compose.yml", "  MINIO_ROOT_PASSWORD: minioadmin"},
		{"a TypeScript constant", "fixture.ts", `const apiKey = "sk-live-9f2a7b3c8d1e4f60";`},
		{"a Python assignment", "fixture.py", `CLIENT_SECRET = "provider-issued-value-9f2a"`},
		{"a SQL role", "migration.sql", `CREATE ROLE app LOGIN PASSWORD 'a-real-password-here';`},
		{"a PEM private key", "fixture.go", "-----BEGIN RSA PRIVATE KEY-----\nMIIE\n-----END RSA PRIVATE KEY-----"},
	}
	for _, tc := range cases {
		if got := scanFile(tc.file, tc.body); len(got) == 0 {
			t.Errorf("the scanner missed %s: %q", tc.name, tc.body)
		}
	}
}

// And it must not fire on the patterns this codebase uses correctly, or it
// would be disabled within a week.
func TestTheSecretScannerDoesNotFireOnSafePatterns(t *testing.T) {
	cases := []struct {
		name string
		file string
		body string
	}{
		{"reading from the environment", "fixture.go", `APIToken: env("SENTINEL_API_TOKEN", "")`},
		{"an empty default", "fixture.go", `token := ""`},
		{"a Go struct field holding a variable", "fixture.go", `SecretKey: secretKey,`},
		{"a Go field holding a function call", "fixture.go", `MetricsToken: mustLoadToken(),`},
		{"a shell interpolation", "fixture.sh", "SENTINEL_API_TOKEN=${SENTINEL_API_TOKEN}"},
		{"a compose interpolation", "compose.yml", "  SENTINEL_METRICS_TOKEN: ${SENTINEL_METRICS_TOKEN:?required}"},
		{"a header name", "fixture.go", `w.Header().Set("Access-Control-Allow-Headers", "Authorization, X-CSRF-Token")`},
		{"the CSRF cookie name", "fixture.go", `auth.RequireCSRFToken("sentinel_session", "X-CSRF-Token")`},
		{"a placeholder", "fixture.go", `apiToken: "<your-token-here>"`},
		{"a redaction constant", "fixture.go", `Placeholder = "[REDACTED]"`},
		{"a comment", "fixture.go", `// token = "this-is-explaining-a-defect"`},
		{"a TypeScript env read", "fixture.ts", `const apiKey = process.env.SENTINEL_API_TOKEN;`},
	}
	for _, tc := range cases {
		if got := scanFile(tc.file, tc.body); len(got) > 0 {
			t.Errorf("the scanner fired on %s (%q): %v", tc.name, tc.body, got)
		}
	}
}
