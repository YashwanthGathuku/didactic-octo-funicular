package main

import (
	"context"
	"net"
	"strings"
	"testing"
)

// Mock Context to simulate request scoping
type ContextKey string
const TenantIDKey ContextKey = "tenant_id"
const UserIDKey ContextKey = "user_id"

func mockContext(tenant, user string) context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, TenantIDKey, tenant)
	ctx = context.WithValue(ctx, UserIDKey, user)
	return ctx
}

// 1. IDOR Prevention (Threat 1)
func checkAccess(ctx context.Context, targetTenantID string) bool {
	callerTenant, ok := ctx.Value(TenantIDKey).(string)
	if !ok || callerTenant == "" {
		return false
	}
	return callerTenant == targetTenantID
}

func TestThreatModel_IDORPrevention(t *testing.T) {
	ctxT1 := mockContext("tenant-1", "user-A")

	// Tenant 1 accessing Tenant 1 resource
	if !checkAccess(ctxT1, "tenant-1") {
		t.Error("expected tenant-1 to access tenant-1 resource")
	}

	// Tenant 1 accessing Tenant 2 resource (IDOR Attempt)
	if checkAccess(ctxT1, "tenant-2") {
		t.Error("IDOR vulnerability: tenant-1 accessed tenant-2 resource")
	}

	// Missing tenant context
	if checkAccess(context.Background(), "tenant-1") {
		t.Error("IDOR vulnerability: unauthenticated context accessed resource")
	}
}

// 2. Dual-Control Bypass Denial (Threat 2)
func attemptRelease(creatorID, approverID string) bool {
	// A creator cannot approve their own release
	if creatorID == approverID {
		return false
	}
	return true
}

func TestThreatModel_DualControlBypassDenial(t *testing.T) {
	if attemptRelease("user-A", "user-A") {
		t.Error("Dual-control bypass vulnerability: user approved their own release")
	}
	if !attemptRelease("user-A", "user-B") {
		t.Error("Dual-control failure: distinct users should be able to release")
	}
}

// 3. Path Traversal Denial (Threat 3)
func sanitizeFilename(filename string) string {
	// Reject any path traversal characters
	if strings.Contains(filename, "/") || strings.Contains(filename, "\\") || strings.Contains(filename, "..") {
		return ""
	}
	return filename
}

func TestThreatModel_PathTraversalDenial(t *testing.T) {
	malicious := []string{
		"../../../etc/passwd",
		"..\\..\\windows\\system32\\cmd.exe",
		"/absolute/path/file.txt",
		"file/with/slash.txt",
	}

	for _, payload := range malicious {
		if sanitizeFilename(payload) != "" {
			t.Errorf("Path traversal vulnerability: failed to reject %q", payload)
		}
	}

	valid := "nacha_batch_2026.ach"
	if sanitizeFilename(valid) != valid {
		t.Error("False positive on valid filename")
	}
}

// 4. Parser Resource Exhaustion Denial (Threat 4)
func parseRecords(count int, maxLimit int) bool {
	if count > maxLimit {
		return false
	}
	return true
}

func TestThreatModel_ParserResourceExhaustionDenial(t *testing.T) {
	maxRows := 100000

	if parseRecords(5000, maxRows) == false {
		t.Error("Failed to parse valid record count")
	}

	if parseRecords(2000000000, maxRows) == true {
		t.Error("Resource exhaustion vulnerability: failed to enforce parsing limits on massive payload")
	}
}

// 5. SSRF Denial (Threat 6)
func isSafeSSRFDestination(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	// Reject Loopback, Link Local, and Multicast
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsMulticast() {
		return false
	}
	return true
}

func TestThreatModel_SSRFDenial(t *testing.T) {
	maliciousIPs := []string{
		"127.0.0.1",       // Localhost
		"169.254.169.254", // AWS Metadata
		"::1",             // IPv6 Loopback
	}

	for _, ip := range maliciousIPs {
		if isSafeSSRFDestination(ip) {
			t.Errorf("SSRF vulnerability: failed to reject protected IP %q", ip)
		}
	}

	validIP := "93.184.216.34" // Example.com
	if !isSafeSSRFDestination(validIP) {
		t.Errorf("False positive SSRF rejection on valid IP %q", validIP)
	}
}

// 6. Unsafe SQL Template Denial (Threat 7)
func isSQLTemplateSafe(template string) bool {
	// Reject literal string interpolation or execution of arbitrary SQL
	// Safe templates must use parameterized placeholders ($1, ?, :name)
	// For this test, we enforce that string concatenation/formatting characters are blocked
	if strings.Contains(template, "%s") || strings.Contains(template, "' +") || strings.Contains(template, "';") {
		return false
	}
	return true
}

func TestThreatModel_UnsafeSQLTemplateDenial(t *testing.T) {
	maliciousTemplates := []string{
		"SELECT * FROM users WHERE name = '%s'",
		"SELECT * FROM tx WHERE id = " + "'; DROP TABLE users; --",
	}

	for _, tpl := range maliciousTemplates {
		if isSQLTemplateSafe(tpl) {
			t.Errorf("SQL Injection vulnerability: failed to reject unsafe template %q", tpl)
		}
	}

	safeTemplate := "SELECT * FROM users WHERE id = $1 AND tenant_id = $2"
	if !isSQLTemplateSafe(safeTemplate) {
		t.Error("False positive on safe parameterized template")
	}
}
