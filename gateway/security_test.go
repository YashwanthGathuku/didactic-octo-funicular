package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
)

func TestPrometheusMetricsExposition(t *testing.T) {
	GlobalMetrics.RecordFileIngested("VALID", 1024)
	GlobalMetrics.RecordFileIngested("QUARANTINED", 2048)
	GlobalMetrics.SetAuditChainHeight(42)
	GlobalMetrics.SetActiveIncidents(1)

	req := httptest.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()

	ServePrometheusMetrics(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rr.Code)
	}

	body := rr.Body.String()
	requiredSubstrings := []string{
		"sentinel_files_ingested_total{status=\"VALID\"}",
		"sentinel_files_ingested_total{status=\"QUARANTINED\"}",
		"sentinel_bytes_ingested_total",
		"sentinel_audit_chain_height 42",
	}

	// The parse-rate gauge must reflect a real measurement, never a constant.
	GlobalMetrics.RecordMeasuredParseRate(12345)
	rr2 := httptest.NewRecorder()
	ServePrometheusMetrics(rr2, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(rr2.Body.String(), "sentinel_streaming_parse_rate_records_per_sec 12345") {
		t.Errorf("parse-rate gauge did not reflect the measured value")
	}

	for _, sub := range requiredSubstrings {
		if !strings.Contains(body, sub) {
			t.Errorf("Expected metrics body to contain %q, but was missing", sub)
		}
	}
}

func TestSshPublicKeyValidation(t *testing.T) {
	// Valid OpenSSH Ed25519 public key
	ed25519Key := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIH1N8sDqR3kH8m9qWeRt7y4a3F9cKz8eL1mQxWpTvBn9 treasury-operator@meridian.internal"
	res, err := VerifySshPublicKey(ed25519Key)
	if err != nil {
		t.Fatalf("Unexpected error verifying valid SSH key: %v", err)
	}
	if !res.IsValid || res.KeyType != "ssh-ed25519" || !strings.HasPrefix(res.Fingerprint, "SHA256:") {
		t.Errorf("Unexpected result: %+v", res)
	}

	// Invalid SSH key format
	_, err = VerifySshPublicKey("bogus-malformed-key")
	if err == nil {
		t.Errorf("Expected error on malformed SSH key, got nil")
	}
}

func TestPgpDetachedSignatureValidation(t *testing.T) {
	payload := []byte("101 021000018 0210000212608141000A094101MERIDIAN CUSTODY     SENTINEL FLOW GATEWAY")

	// 1. FAIL CLOSED: no keyring configured -> must NOT assert authenticity.
	t.Setenv("SENTINEL_PGP_KEYRING", "")
	res, err := VerifyDetachedPgpSignature(payload, "-----BEGIN PGP SIGNATURE-----\nnonsense\n-----END PGP SIGNATURE-----")
	if err == nil || res.IsAuthentic {
		t.Fatalf("must fail closed without a keyring; got authentic=%v err=%v", res.IsAuthentic, err)
	}

	// 2. Generate a real key, sign real bytes, verify.
	entity, err := openpgp.NewEntity("Meridian Custody", "counterparty test key", "ops@meridian.test", nil)
	if err != nil {
		t.Fatalf("keygen failed: %v", err)
	}

	dir := t.TempDir()
	ringPath := filepath.Join(dir, "counterparties.asc")
	f, err := os.Create(ringPath)
	if err != nil {
		t.Fatal(err)
	}
	aw, err := armor.Encode(f, openpgp.PublicKeyType, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := entity.Serialize(aw); err != nil {
		t.Fatal(err)
	}
	aw.Close()
	f.Close()
	t.Setenv("SENTINEL_PGP_KEYRING", ringPath)
	resetKeyringCacheForTest()

	var sig bytes.Buffer
	if err := openpgp.ArmoredDetachSign(&sig, entity, bytes.NewReader(payload), nil); err != nil {
		t.Fatalf("signing failed: %v", err)
	}

	res, err = VerifyDetachedPgpSignature(payload, sig.String())
	if err != nil || !res.IsAuthentic {
		t.Fatalf("genuine signature must verify; got authentic=%v err=%v reason=%s", res.IsAuthentic, err, res.StatusReason)
	}
	if len(res.PayloadHash) != 64 {
		t.Errorf("expected 64-hex payload digest, got %q", res.PayloadHash)
	}

	// 3. TAMPER: flip one byte of the payload -> verification must fail.
	tampered := append([]byte(nil), payload...)
	tampered[10] ^= 0x01
	res, err = VerifyDetachedPgpSignature(tampered, sig.String())
	if err == nil || res.IsAuthentic {
		t.Fatalf("tampered payload must NOT verify; got authentic=%v err=%v", res.IsAuthentic, err)
	}

	// 4. Garbage armor must be rejected.
	if _, err := VerifyDetachedPgpSignature(payload, "INVALID SIGNATURE STRING"); err == nil {
		t.Errorf("expected error on malformed armor")
	}
}
