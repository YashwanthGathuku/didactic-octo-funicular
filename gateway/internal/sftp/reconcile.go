package sftp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ReconciliationReport summarizes the outcome of a directory/bucket reconciliation sweep.
type ReconciliationReport struct {
	TenantID       string    `json:"tenant_id"`
	ScannedCount   int       `json:"scanned_count"`
	RecoveredCount int       `json:"recovered_count"`
	SkippedCount   int       `json:"skipped_count"`
	DurationMs     int64     `json:"duration_ms"`
	Timestamp      time.Time `json:"timestamp"`
	RecoveredFiles []string  `json:"recovered_files,omitempty"`
}

// ReconciliationScanner scans SFTP storage directories to recover from dropped webhooks.
type ReconciliationScanner struct {
	ingress      *IngressService
	settleWindow time.Duration
}

// NewReconciliationScanner builds a reconciliation scanner.
func NewReconciliationScanner(ingress *IngressService, settleWindow time.Duration) *ReconciliationScanner {
	if settleWindow <= 0 {
		settleWindow = 3 * time.Second
	}
	return &ReconciliationScanner{
		ingress:      ingress,
		settleWindow: settleWindow,
	}
}

// ScanDirectory walks a local SFTP directory and ingests any un-recorded finalized files.
func (r *ReconciliationScanner) ScanDirectory(
	ctx context.Context,
	tenantID string,
	rootPath string,
) (*ReconciliationReport, error) {
	start := time.Now()
	report := &ReconciliationReport{
		TenantID:  tenantID,
		Timestamp: start.UTC(),
	}

	if _, err := os.Stat(rootPath); err != nil {
		return nil, fmt.Errorf("inaccessible scan root %s: %w", rootPath, err)
	}

	now := time.Now()

	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		name := info.Name()
		nameLower := strings.ToLower(name)
		// 1. Skip temporary / in-flight transfer files (WinSCP .filepart, OpenSSH .part, .tmp)
		if strings.HasPrefix(name, ".") || strings.HasSuffix(nameLower, ".tmp") ||
			strings.Contains(nameLower, ".sftpgo-tmp") || strings.HasSuffix(nameLower, ".filepart") ||
			strings.HasSuffix(nameLower, ".part") {
			report.SkippedCount++
			return nil
		}

		// 2. Skip files modified within the settle window (potential active transfer)
		if now.Sub(info.ModTime()) < r.settleWindow {
			report.SkippedCount++
			return nil
		}

		report.ScannedCount++

		// 3. Compute file SHA-256 hash
		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open file %s: %w", path, err)
		}
		hasher := sha256.New()
		size, err := io.Copy(hasher, f)
		_ = f.Close()
		if err != nil {
			return fmt.Errorf("hash file %s: %w", path, err)
		}
		hashHex := hex.EncodeToString(hasher.Sum(nil))

		// 4. Ingest via IngressService
		relPath, _ := filepath.Rel(rootPath, path)
		virtualPath := "/" + filepath.ToSlash(relPath)

		event := FinalizedUploadEvent{
			EventID:     fmt.Sprintf("RECONCILE-%d", time.Now().UnixNano()),
			Action:      "upload",
			Status:      1,
			Timestamp:   info.ModTime().UnixMilli(),
			Username:    "reconciliation_scanner",
			IPAddress:   "127.0.0.1",
			VirtualPath: virtualPath,
			FSPath:      path,
			SizeBytes:   size,
			SHA256Hash:  hashHex,
		}

		rawJSON, _ := eventBytes(event)
		result, err := r.ingress.HandleWebhook(ctx, tenantID, "", 0, rawJSON)
		if err != nil {
			return fmt.Errorf("reconciliation ingestion failed for %s: %w", path, err)
		}

		if !result.Deduplicated {
			report.RecoveredCount++
			report.RecoveredFiles = append(report.RecoveredFiles, path)
		}

		return nil
	})

	report.DurationMs = time.Since(start).Milliseconds()
	return report, err
}

func eventBytes(e FinalizedUploadEvent) ([]byte, error) {
	return []byte(fmt.Sprintf(`{
		"event_id": %q,
		"action": %q,
		"status": %d,
		"timestamp": %d,
		"username": %q,
		"ip_address": %q,
		"virtual_path": %q,
		"fs_path": %q,
		"file_size": %d,
		"sha256_hash": %q
	}`, e.EventID, e.Action, e.Status, e.Timestamp, e.Username, e.IPAddress, e.VirtualPath, e.FSPath, e.SizeBytes, e.SHA256Hash)), nil
}
