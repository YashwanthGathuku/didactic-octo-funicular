package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"sentinel-gateway/internal/telemetry"
)

var (
	// ErrAITierNotConfigured indicates AI_TIER_URL is empty.
	ErrAITierNotConfigured = errors.New("ai tier is not configured")

	// ErrAITierUnavailable indicates the AI tier is unreachable, timed out, or returned 5xx.
	ErrAITierUnavailable = errors.New("ai tier is unavailable")

	// ErrAITierBadRequest indicates the AI tier rejected the input (e.g. Model Armor blocked or header mismatch).
	ErrAITierBadRequest = errors.New("ai tier rejected request")

	// ErrAITierInvalidResponse indicates non-decodable JSON response from AI tier.
	ErrAITierInvalidResponse = errors.New("ai tier returned invalid response")

	// ErrResponseTooLarge indicates the response exceeded the maximum bounded size.
	ErrResponseTooLarge = errors.New("ai tier response exceeded size limit")
)

// AIClientConfig configures bounded outbound communication with the AI control plane.
type AIClientConfig struct {
	BaseURL          string
	Timeout          time.Duration
	MaxRequestBytes  int64
	MaxResponseBytes int64
	MaxRetries       int
}

// DefaultAIClientConfig provides hardened default bounds.
func DefaultAIClientConfig(baseURL string) AIClientConfig {
	return AIClientConfig{
		BaseURL:          strings.TrimRight(baseURL, "/"),
		Timeout:          5 * time.Second,
		MaxRequestBytes:  1024 * 1024,     // 1 MiB ceiling
		MaxResponseBytes: 2 * 1024 * 1024, // 2 MiB ceiling
		MaxRetries:       2,
	}
}

// AIClient is the dedicated service client for AI control plane communication.
type AIClient struct {
	cfg        AIClientConfig
	httpClient *http.Client
}

// NewAIClient creates an AI client with explicit timeout, response limits, and connection pooling.
func NewAIClient(cfg AIClientConfig) *AIClient {
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Second
	}
	if cfg.MaxRequestBytes == 0 {
		cfg.MaxRequestBytes = 1024 * 1024
	}
	if cfg.MaxResponseBytes == 0 {
		cfg.MaxResponseBytes = 2 * 1024 * 1024
	}
	return &AIClient{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// TriageIncident dispatches a canonical AgentContextEnvelope to the AI tier with stable idempotency keys.
func (c *AIClient) TriageIncident(ctx context.Context, env *AgentContextEnvelope) (*AnalystResponse, error) {
	if c.cfg.BaseURL == "" {
		return nil, ErrAITierNotConfigured
	}
	if err := env.Validate(); err != nil {
		return nil, fmt.Errorf("invalid envelope: %w", err)
	}

	payload, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("marshal envelope: %w", err)
	}

	if int64(len(payload)) > c.cfg.MaxRequestBytes {
		return nil, fmt.Errorf("envelope size %d exceeds ceiling %d", len(payload), c.cfg.MaxRequestBytes)
	}

	targetURL := c.cfg.BaseURL + "/analyze"
	var lastErr error

	// Stable idempotency key derived from correlation ID and incident ID
	idempotencyKey := env.CorrelationID
	if idempotencyKey == "" {
		idempotencyKey = fmt.Sprintf("ai-req-%s-%d-%d", env.TenantID, env.IncidentID, time.Now().UnixNano())
	}

	for attempt := 0; attempt <= c.cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt*100) * time.Millisecond):
			}
		}

		ctx, span := telemetry.StartSpan(ctx, "sentinelflow.gateway.ai_client.triage")
		defer span.End()
		span.SetAttribute("tenant_id", env.TenantID)
		span.SetAttribute("incident_id", fmt.Sprintf("%d", env.IncidentID))

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Correlation-ID", env.CorrelationID)
		req.Header.Set("X-Sentinel-Tenant", env.TenantID)
		req.Header.Set("X-Idempotency-Key", idempotencyKey)
		req.Header.Set(telemetry.TraceParentHeader, span.FormatW3CTraceParent())
		if env.TraceID != "" {
			req.Header.Set("X-Trace-ID", env.TraceID)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("%w: %v", ErrAITierUnavailable, err)
			continue
		}

		// Strictly bounded response reading
		bodyReader := io.LimitReader(resp.Body, c.cfg.MaxResponseBytes+1)
		bodyBytes, readErr := io.ReadAll(bodyReader)
		resp.Body.Close()

		if readErr != nil {
			lastErr = fmt.Errorf("%w: failed reading response: %v", ErrAITierUnavailable, readErr)
			continue
		}

		if int64(len(bodyBytes)) > c.cfg.MaxResponseBytes {
			return nil, fmt.Errorf("%w: received %d bytes (limit %d)", ErrResponseTooLarge, len(bodyBytes), c.cfg.MaxResponseBytes)
		}

		if resp.StatusCode == http.StatusOK {
			var aiRes AnalystResponse
			if err := json.Unmarshal(bodyBytes, &aiRes); err != nil {
				return nil, fmt.Errorf("%w: decode JSON: %v", ErrAITierInvalidResponse, err)
			}
			return &aiRes, nil
		}

		if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusForbidden {
			return nil, fmt.Errorf("%w: %s", ErrAITierBadRequest, string(bodyBytes))
		}

		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("%w: server returned %d: %s", ErrAITierUnavailable, resp.StatusCode, string(bodyBytes))
			continue
		}

		return nil, fmt.Errorf("%w: unexpected HTTP status %d: %s", ErrAITierUnavailable, resp.StatusCode, string(bodyBytes))
	}

	return nil, lastErr
}

// RunEvals proxies evaluation suite execution to the AI tier.
func (c *AIClient) RunEvals(ctx context.Context) ([]byte, error) {
	if c.cfg.BaseURL == "" {
		return nil, ErrAITierNotConfigured
	}

	ctx, span := telemetry.StartSpan(ctx, "sentinelflow.gateway.ai_client.evals")
	defer span.End()

	targetURL := c.cfg.BaseURL + "/evals/run"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create evals request: %w", err)
	}
	req.Header.Set(telemetry.TraceParentHeader, span.FormatW3CTraceParent())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAITierUnavailable, err)
	}
	defer resp.Body.Close()

	bodyReader := io.LimitReader(resp.Body, c.cfg.MaxResponseBytes)
	bodyBytes, err := io.ReadAll(bodyReader)
	if err != nil {
		return nil, fmt.Errorf("%w: reading evals response: %v", ErrAITierUnavailable, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: evals returned status %d: %s", ErrAITierUnavailable, resp.StatusCode, string(bodyBytes))
	}

	return bodyBytes, nil
}
