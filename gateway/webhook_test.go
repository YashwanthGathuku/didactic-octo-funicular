package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebhookHmacComputationAndDispatch(t *testing.T) {
	secret := "test-secret-key-9988"
	event := WebhookDeliveryEvent{
		EventID:       "EVT-TEST-001",
		EventType:     "FILE_VALIDATED_AND_RELEASED",
		TimestampUtc:  "2026-08-14T10:00:00Z",
		TenantID:      "TENANT-DEFAULT",
		PayloadDigest: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		Data: map[string]interface{}{
			"filename": "MERIDIAN_ACH_20260814.ach",
			"status":   "RELEASED",
		},
	}

	payloadBytes, _ := json.Marshal(event)
	expectedSig := ComputeWebhookHmac(payloadBytes, secret)

	if len(expectedSig) != 64 {
		t.Errorf("Expected 64-char hex HMAC signature, got length %d", len(expectedSig))
	}

	// Create test HTTP receiver server
	var receivedSignature string
	var receivedEventHeader string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSignature = r.Header.Get("X-Sentinel-Signature")
		receivedEventHeader = r.Header.Get("X-Sentinel-Event")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"received": true}`))
	}))
	defer ts.Close()

	log, err := DispatchWebhookEvent(ts.URL, secret, event)
	if err != nil {
		t.Fatalf("Unexpected webhook dispatch error: %v", err)
	}

	if log.Status != "DELIVERED" || log.ResponseCode != 200 {
		t.Errorf("Expected DELIVERED 200, got status=%s code=%d", log.Status, log.ResponseCode)
	}

	if receivedEventHeader != "FILE_VALIDATED_AND_RELEASED" {
		t.Errorf("Expected event header FILE_VALIDATED_AND_RELEASED, got %s", receivedEventHeader)
	}

	if !strings.HasSuffix(receivedSignature, expectedSig) {
		t.Errorf("Expected signature header to contain %s, got %s", expectedSig, receivedSignature)
	}
}
