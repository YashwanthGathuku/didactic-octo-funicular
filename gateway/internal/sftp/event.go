package sftp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// FinalizedUploadEvent represents a real finalized upload notification.
//
// Invariant: Ingestion triggers ONLY when an upload has completed and its
// file descriptor has been closed cleanly (action == "upload" and status == 1).
type FinalizedUploadEvent struct {
	EventID     string `json:"event_id"`
	Action      string `json:"action"` // Must be "upload"
	Status      int    `json:"status"` // 1 = success, 0 = failure / in-flight
	Timestamp   int64  `json:"timestamp"`
	Username    string `json:"username"`
	IPAddress   string `json:"ip_address"`
	VirtualPath string `json:"virtual_path"`
	FSPath      string `json:"fs_path"`
	SizeBytes   int64  `json:"file_size"`
	SHA256Hash  string `json:"sha256_hash"`
	ElapsedMs   int64  `json:"elapsed_ms"`
}

// Validate verifies that the event represents a legitimate, completed file arrival.
func (e *FinalizedUploadEvent) Validate() error {
	if e.Action != "upload" {
		return fmt.Errorf("invalid action %q: only finalized uploads are processed", e.Action)
	}
	if e.Status != 1 {
		return fmt.Errorf("upload status %d: transfer did not succeed or is still in flight", e.Status)
	}
	if e.VirtualPath == "" && e.FSPath == "" {
		return errors.New("missing file path in upload event")
	}
	lowerV := strings.ToLower(e.VirtualPath)
	lowerF := strings.ToLower(e.FSPath)
	if strings.HasSuffix(lowerV, ".tmp") || strings.HasSuffix(lowerF, ".tmp") ||
		strings.Contains(lowerV, ".sftpgo-tmp") || strings.Contains(lowerF, ".sftpgo-tmp") ||
		strings.HasSuffix(lowerV, ".filepart") || strings.HasSuffix(lowerF, ".filepart") ||
		strings.HasSuffix(lowerV, ".part") || strings.HasSuffix(lowerF, ".part") {
		return errors.New("cannot ingest temporary/in-flight file transfer (WinSCP .filepart, OpenSSH .part, or .tmp)")
	}
	if e.SizeBytes < 0 {
		return fmt.Errorf("invalid file size: %d", e.SizeBytes)
	}
	if e.SHA256Hash == "" {
		return errors.New("missing SHA-256 hash in upload event")
	}
	if len(e.SHA256Hash) != 64 {
		return fmt.Errorf("invalid SHA-256 hash length %d, expected 64 hex chars", len(e.SHA256Hash))
	}
	return nil
}

// Filename returns the basename of the arrived file.
func (e *FinalizedUploadEvent) Filename() string {
	if e.VirtualPath != "" {
		return filepath.Base(e.VirtualPath)
	}
	return filepath.Base(e.FSPath)
}

// DedupeKey generates a deterministic deduplication token for idempotent ingestion.
func (e *FinalizedUploadEvent) DedupeKey(tenantID string) string {
	raw := fmt.Sprintf("%s:%s:%s:%d", tenantID, e.VirtualPath, e.SHA256Hash, e.SizeBytes)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// ComputeSignature calculates the HMAC-SHA256 signature for webhook authentication.
func ComputeSignature(secret string, timestamp int64, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("%d\n", timestamp)))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature validates incoming webhook authentication against replay attacks.
func VerifySignature(secret string, signature string, timestamp int64, payload []byte, maxSkew time.Duration) error {
	now := time.Now().Unix()
	diff := now - timestamp
	if diff < 0 {
		diff = -diff
	}
	if time.Duration(diff)*time.Second > maxSkew {
		return fmt.Errorf("timestamp skew %ds exceeds allowed window %v", diff, maxSkew)
	}

	expected := ComputeSignature(secret, timestamp, payload)
	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return errors.New("invalid HMAC signature")
	}
	return nil
}
