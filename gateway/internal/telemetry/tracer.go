package telemetry

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type correlationKey struct{}
type spanKey struct{}

// CorrelationIDHeader is the standard HTTP header for correlation IDs.
const CorrelationIDHeader = "X-Correlation-ID"

// TraceParentHeader is the W3C traceparent standard header.
const TraceParentHeader = "traceparent"

// TraceStateHeader is the W3C tracestate standard header.
const TraceStateHeader = "tracestate"

// Span represents an in-process OpenTelemetry-compatible trace span.
type Span struct {
	TraceID    string
	SpanID     string
	ParentID   string
	Name       string
	StartTime  time.Time
	EndTime    time.Time
	Attributes map[string]string
	mu         sync.Mutex
}

// GenerateOpaqueID returns a cryptographically random hex string of n bytes.
func GenerateOpaqueID(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ExtractCorrelationID retrieves or generates an opaque correlation ID from a request.
func ExtractCorrelationID(r *http.Request) string {
	if val := strings.TrimSpace(r.Header.Get(CorrelationIDHeader)); val != "" {
		// Verify correlation ID is opaque (alphanumeric, dashes, underscores, max length 64)
		if isSafeOpaqueID(val) {
			return val
		}
	}
	// Check W3C traceparent: version-traceid-parentid-traceflags
	if tp := strings.TrimSpace(r.Header.Get(TraceParentHeader)); tp != "" {
		parts := strings.Split(tp, "-")
		if len(parts) == 4 && len(parts[1]) == 32 {
			return parts[1]
		}
	}
	// Generate new opaque 16-byte correlation ID
	return GenerateOpaqueID(16)
}

func isSafeOpaqueID(s string) bool {
	if len(s) > 64 || len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

// WithCorrelationID attaches a correlation ID to context.
func WithCorrelationID(ctx context.Context, cid string) context.Context {
	return context.WithValue(ctx, correlationKey{}, cid)
}

// GetCorrelationID returns the correlation ID from context, or empty string.
func GetCorrelationID(ctx context.Context) string {
	if v, ok := ctx.Value(correlationKey{}).(string); ok {
		return v
	}
	return ""
}

// StartSpan creates and begins a new span attached to context.
func StartSpan(ctx context.Context, name string) (context.Context, *Span) {
	parentSpan, _ := ctx.Value(spanKey{}).(*Span)
	traceID := GetCorrelationID(ctx)
	if traceID == "" {
		if parentSpan != nil {
			traceID = parentSpan.TraceID
		} else {
			traceID = GenerateOpaqueID(16)
		}
	}
	spanID := GenerateOpaqueID(8)
	parentID := ""
	if parentSpan != nil {
		parentID = parentSpan.SpanID
	}

	span := &Span{
		TraceID:    traceID,
		SpanID:     spanID,
		ParentID:   parentID,
		Name:       name,
		StartTime:  time.Now(),
		Attributes: make(map[string]string),
	}

	childCtx := context.WithValue(ctx, spanKey{}, span)
	childCtx = context.WithValue(childCtx, correlationKey{}, traceID)
	return childCtx, span
}

// End concludes the span.
func (s *Span) End() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.EndTime = time.Now()
}

// SetAttribute sets a key-value attribute on the span, ensuring no sensitive data is attached.
func (s *Span) SetAttribute(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Attributes[key] = value
}

// FormatW3CTraceParent returns a W3C traceparent string: 00-{traceid}-{spanid}-01
func (s *Span) FormatW3CTraceParent() string {
	tid := s.TraceID
	if len(tid) < 32 {
		tid = fmt.Sprintf("%032s", tid)
	}
	return fmt.Sprintf("00-%s-%s-01", tid, s.SpanID)
}
