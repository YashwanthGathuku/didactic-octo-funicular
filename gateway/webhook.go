package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type WebhookSubscription struct {
	ID        int64     `json:"id"`
	URL       string    `json:"url"`
	Secret    string    `json:"secret"`
	Events    []string  `json:"events"`
	Status    string    `json:"status"` // ACTIVE, DISABLED
	CreatedAt time.Time `json:"createdAt"`
}

type WebhookDeliveryEvent struct {
	EventID       string      `json:"eventId"`
	EventType     string      `json:"eventType"`
	TimestampUtc  string      `json:"timestampUtc"`
	TenantID      string      `json:"tenantId"`
	PayloadDigest string      `json:"payloadDigest"`
	Data          interface{} `json:"data"`
}

type WebhookDeliveryLog struct {
	ID           int64     `json:"id"`
	WebhookID    int64     `json:"webhookId"`
	EventType    string    `json:"eventType"`
	ResponseCode int       `json:"responseCode"`
	Status       string    `json:"status"` // DELIVERED, FAILED
	DeliveredAt  time.Time `json:"deliveredAt"`
	DurationMs   int64     `json:"durationMs"`
}

// ComputeWebhookHmac generates the cryptographic HMAC-SHA256 signature for downstream validation.
func ComputeWebhookHmac(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// DispatchWebhookEvent sends an asynchronous HTTP POST with HMAC signature header.
func DispatchWebhookEvent(url string, secret string, event WebhookDeliveryEvent) (*WebhookDeliveryLog, error) {
	payloadBytes, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal webhook event: %w", err)
	}

	sig := ComputeWebhookHmac(payloadBytes, secret)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Sentinel-Flow-Webhook-Dispatcher/1.0")
	req.Header.Set("X-Sentinel-Signature", fmt.Sprintf("sha256=%s", sig))
	req.Header.Set("X-Sentinel-Event", event.EventType)
	req.Header.Set("X-Sentinel-Event-Id", event.EventID)

	client := &http.Client{Timeout: 5 * time.Second}
	startTime := time.Now()

	resp, err := client.Do(req)
	duration := time.Since(startTime).Milliseconds()

	if err != nil {
		return &WebhookDeliveryLog{
			EventType:    event.EventType,
			ResponseCode: 0,
			Status:       "FAILED",
			DeliveredAt:  time.Now().UTC(),
			DurationMs:   duration,
		}, err
	}
	defer resp.Body.Close()

	status := "DELIVERED"
	if resp.StatusCode >= 400 {
		status = "FAILED"
	}

	return &WebhookDeliveryLog{
		EventType:    event.EventType,
		ResponseCode: resp.StatusCode,
		Status:       status,
		DeliveredAt:  time.Now().UTC(),
		DurationMs:   duration,
	}, nil
}

// InitWebhookSchema ensures webhook subscription table exists in SQLite.
func InitWebhookSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS webhook_subscriptions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			url TEXT NOT NULL,
			secret TEXT NOT NULL,
			events TEXT NOT NULL DEFAULT '["ALL"]',
			status TEXT NOT NULL DEFAULT 'ACTIVE',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
	`)
	return err
}
