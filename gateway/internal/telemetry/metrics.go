package telemetry

import (
	"bytes"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// MetricType denotes the Prometheus metric type.
type MetricType string

const (
	TypeCounter   MetricType = "counter"
	TypeGauge     MetricType = "gauge"
	TypeHistogram MetricType = "histogram"
)

// DefaultLatencyBuckets are standard second-based latency buckets for Prometheus histograms.
var DefaultLatencyBuckets = []float64{
	0.001, 0.005, 0.010, 0.025, 0.050, 0.100, 0.250, 0.500, 1.000, 2.500, 5.000, 10.000,
}

// Label represents a single key-value metric label.
type Label struct {
	Key   string
	Value string
}

// LabelSet is a sorted slice of labels.
type LabelSet []Label

func (ls LabelSet) String() string {
	if len(ls) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteByte('{')
	for i, l := range ls {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(l.Key)
		b.WriteString(`="`)
		b.WriteString(escapeLabelValue(l.Value))
		b.WriteByte('"')
	}
	b.WriteByte('}')
	return b.String()
}

func escapeLabelValue(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}

// Counter is a thread-safe Prometheus counter.
type Counter struct {
	name   string
	help   string
	mu     sync.RWMutex
	values map[string]*atomic.Uint64
}

// NewCounter creates a new named counter.
func NewCounter(name, help string) *Counter {
	return &Counter{
		name:   name,
		help:   help,
		values: make(map[string]*atomic.Uint64),
	}
}

// Inc increments the counter with the given labels by 1.
func (c *Counter) Inc(labels ...Label) {
	c.Add(1, labels...)
}

// Add adds delta to the counter with the given labels.
func (c *Counter) Add(delta uint64, labels ...Label) {
	key := LabelSet(labels).String()
	c.mu.RLock()
	val, ok := c.values[key]
	c.mu.RUnlock()

	if !ok {
		c.mu.Lock()
		val, ok = c.values[key]
		if !ok {
			val = &atomic.Uint64{}
			c.values[key] = val
		}
		c.mu.Unlock()
	}
	val.Add(delta)
}

// Get returns the current value for the given labels.
func (c *Counter) Get(labels ...Label) uint64 {
	key := LabelSet(labels).String()
	c.mu.RLock()
	defer c.mu.RUnlock()
	if val, ok := c.values[key]; ok {
		return val.Load()
	}
	return 0
}

// Gauge is a thread-safe Prometheus gauge.
type Gauge struct {
	name    string
	help    string
	mu      sync.RWMutex
	values  map[string]*atomic.Uint64 // stored as float64 bits
	isFloat bool
}

// NewGauge creates a new named gauge.
func NewGauge(name, help string) *Gauge {
	return &Gauge{
		name:    name,
		help:    help,
		values:  make(map[string]*atomic.Uint64),
		isFloat: true,
	}
}

// Set sets the gauge value.
func (g *Gauge) Set(val float64, labels ...Label) {
	key := LabelSet(labels).String()
	bits := math.Float64bits(val)

	g.mu.RLock()
	v, ok := g.values[key]
	g.mu.RUnlock()

	if !ok {
		g.mu.Lock()
		v, ok = g.values[key]
		if !ok {
			v = &atomic.Uint64{}
			g.values[key] = v
		}
		g.mu.Unlock()
	}
	v.Store(bits)
}

// Get returns the gauge value.
func (g *Gauge) Get(labels ...Label) float64 {
	key := LabelSet(labels).String()
	g.mu.RLock()
	defer g.mu.RUnlock()
	if v, ok := g.values[key]; ok {
		return math.Float64frombits(v.Load())
	}
	return 0.0
}

// Histogram is a thread-safe Prometheus histogram.
type Histogram struct {
	name    string
	help    string
	buckets []float64
	mu      sync.RWMutex
	series  map[string]*histogramSeries
}

type histogramSeries struct {
	mu      sync.Mutex
	buckets []uint64
	count   uint64
	sum     float64
}

// NewHistogram creates a new named histogram with explicit upper-bound buckets.
func NewHistogram(name, help string, buckets []float64) *Histogram {
	if len(buckets) == 0 {
		buckets = DefaultLatencyBuckets
	}
	sortedBuckets := make([]float64, len(buckets))
	copy(sortedBuckets, buckets)
	sort.Float64s(sortedBuckets)

	return &Histogram{
		name:    name,
		help:    help,
		buckets: sortedBuckets,
		series:  make(map[string]*histogramSeries),
	}
}

// Observe adds an observation to the histogram.
func (h *Histogram) Observe(val float64, labels ...Label) {
	key := LabelSet(labels).String()
	h.mu.RLock()
	s, ok := h.series[key]
	h.mu.RUnlock()

	if !ok {
		h.mu.Lock()
		s, ok = h.series[key]
		if !ok {
			s = &histogramSeries{
				buckets: make([]uint64, len(h.buckets)),
			}
			h.series[key] = s
		}
		h.mu.Unlock()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.count++
	s.sum += val
	for i, b := range h.buckets {
		if val <= b {
			s.buckets[i]++
		}
	}
}

// Registry manages all registered Prometheus metrics.
type Registry struct {
	mu         sync.RWMutex
	counters   map[string]*Counter
	gauges     map[string]*Gauge
	histograms map[string]*Histogram
	startTime  time.Time
}

// NewRegistry initializes an empty metrics registry.
func NewRegistry() *Registry {
	return &Registry{
		counters:   make(map[string]*Counter),
		gauges:     make(map[string]*Gauge),
		histograms: make(map[string]*Histogram),
		startTime:  time.Now(),
	}
}

// RegisterCounter registers a counter.
func (r *Registry) RegisterCounter(c *Counter) *Counter {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counters[c.name] = c
	return c
}

// RegisterGauge registers a gauge.
func (r *Registry) RegisterGauge(g *Gauge) *Gauge {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gauges[g.name] = g
	return g
}

// RegisterHistogram registers a histogram.
func (r *Registry) RegisterHistogram(h *Histogram) *Histogram {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.histograms[h.name] = h
	return h
}

// Render writes all metrics in Prometheus text format 0.0.4.
func (r *Registry) Render() []byte {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var buf bytes.Buffer

	// Sort metric names for deterministic output
	counterNames := make([]string, 0, len(r.counters))
	for name := range r.counters {
		counterNames = append(counterNames, name)
	}
	sort.Strings(counterNames)

	for _, name := range counterNames {
		c := r.counters[name]
		fmt.Fprintf(&buf, "# HELP %s %s\n", c.name, c.help)
		fmt.Fprintf(&buf, "# TYPE %s counter\n", c.name)
		c.mu.RLock()
		keys := make([]string, 0, len(c.values))
		for k := range c.values {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if len(keys) == 0 {
			fmt.Fprintf(&buf, "%s 0\n", c.name)
		} else {
			for _, k := range keys {
				fmt.Fprintf(&buf, "%s%s %d\n", c.name, k, c.values[k].Load())
			}
		}
		c.mu.RUnlock()
		buf.WriteByte('\n')
	}

	gaugeNames := make([]string, 0, len(r.gauges))
	for name := range r.gauges {
		gaugeNames = append(gaugeNames, name)
	}
	sort.Strings(gaugeNames)

	for _, name := range gaugeNames {
		g := r.gauges[name]
		fmt.Fprintf(&buf, "# HELP %s %s\n", g.name, g.help)
		fmt.Fprintf(&buf, "# TYPE %s gauge\n", g.name)
		g.mu.RLock()
		keys := make([]string, 0, len(g.values))
		for k := range g.values {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if len(keys) == 0 {
			fmt.Fprintf(&buf, "%s 0\n", g.name)
		} else {
			for _, k := range keys {
				val := math.Float64frombits(g.values[k].Load())
				if math.IsNaN(val) {
					fmt.Fprintf(&buf, "%s%s NaN\n", g.name, k)
				} else if math.Trunc(val) == val && !math.IsInf(val, 0) {
					fmt.Fprintf(&buf, "%s%s %.0f\n", g.name, k, val)
				} else {
					fmt.Fprintf(&buf, "%s%s %g\n", g.name, k, val)
				}
			}
		}
		g.mu.RUnlock()
		buf.WriteByte('\n')
	}

	histogramNames := make([]string, 0, len(r.histograms))
	for name := range r.histograms {
		histogramNames = append(histogramNames, name)
	}
	sort.Strings(histogramNames)

	for _, name := range histogramNames {
		h := r.histograms[name]
		fmt.Fprintf(&buf, "# HELP %s %s\n", h.name, h.help)
		fmt.Fprintf(&buf, "# TYPE %s histogram\n", h.name)
		h.mu.RLock()
		keys := make([]string, 0, len(h.series))
		for k := range h.series {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			s := h.series[k]
			s.mu.Lock()
			baseLabels := strings.TrimSuffix(strings.TrimPrefix(k, "{"), "}")
			for i, b := range h.buckets {
				leLabel := fmt.Sprintf(`le="%g"`, b)
				if baseLabels == "" {
					fmt.Fprintf(&buf, "%s_bucket{%s} %d\n", h.name, leLabel, s.buckets[i])
				} else {
					fmt.Fprintf(&buf, "%s_bucket{%s,%s} %d\n", h.name, baseLabels, leLabel, s.buckets[i])
				}
			}
			infLabel := `le="+Inf"`
			if baseLabels == "" {
				fmt.Fprintf(&buf, "%s_bucket{%s} %d\n", h.name, infLabel, s.count)
				fmt.Fprintf(&buf, "%s_sum %g\n", h.name, s.sum)
				fmt.Fprintf(&buf, "%s_count %d\n", h.name, s.count)
			} else {
				fmt.Fprintf(&buf, "%s_bucket{%s,%s} %d\n", h.name, baseLabels, infLabel, s.count)
				fmt.Fprintf(&buf, "%s_sum{%s} %g\n", h.name, baseLabels, s.sum)
				fmt.Fprintf(&buf, "%s_count{%s} %d\n", h.name, baseLabels, s.count)
			}
			s.mu.Unlock()
		}
		h.mu.RUnlock()
		buf.WriteByte('\n')
	}

	return buf.Bytes()
}

// Low-cardinality label validator.
//
// Invariant: No high-cardinality data (UUIDs, tenant names, database IDs, filenames,
// routing numbers, account numbers, secrets, query parameters) may ever be used as
// a Prometheus label value.

var permittedRoutes = map[string]bool{
	"/metrics":                                 true,
	"/api/v1/health":                           true,
	"/api/v1/ready":                            true,
	"/api/v1/stream":                           true,
	"/api/v1/session":                          true,
	"/api/v1/sla-board":                        true,
	"/api/v1/contracts":                        true,
	"/api/v1/contracts/{id}/versions":          true,
	"/api/v1/partners":                         true,
	"/api/v1/incidents":                        true,
	"/api/v1/incidents/{id}/triage":            true,
	"/api/v1/incidents/{id}/approve":           true,
	"/api/v1/reviews/{id}/approve":             true,
	"/api/v1/ledger":                           true,
	"/api/v1/evidence":                         true,
	"/api/v1/compliance/export":                true,
	"/api/v1/files/upload":                     true,
	"/api/v1/files/ingest-raw":                 true,
	"/api/v1/artifacts":                        true,
	"/api/v1/artifacts/{id}":                   true,
	"/api/v1/artifacts/{id}/content":           true,
	"/api/v1/connectors":                       true,
	"/api/v1/connections":                      true,
	"/api/v1/connections/{id}":                 true,
	"/api/v1/connections/{id}/test":            true,
	"/api/v1/connections/{id}/secrets/{field}": true,
	"/api/v1/service-health":                   true,
	"/api/v1/security/verify-key":              true,
	"/api/v1/security/verify-signature":        true,
	"/api/v1/generator/sample":                 true,
	"/api/v1/evals/run":                        true,
	"UNKNOWN_ROUTE":                            true,
}

// NormalizeRoute maps an incoming route path or pattern to an approved low-cardinality route.
func NormalizeRoute(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	if permittedRoutes[pattern] {
		return pattern
	}
	// Normalize dynamic paths if concrete path passed
	if strings.HasPrefix(pattern, "/api/v1/artifacts/") {
		if strings.HasSuffix(pattern, "/content") {
			return "/api/v1/artifacts/{id}/content"
		}
		return "/api/v1/artifacts/{id}"
	}
	if strings.HasPrefix(pattern, "/api/v1/contracts/") && strings.HasSuffix(pattern, "/versions") {
		return "/api/v1/contracts/{id}/versions"
	}
	if strings.HasPrefix(pattern, "/api/v1/incidents/") {
		if strings.HasSuffix(pattern, "/triage") {
			return "/api/v1/incidents/{id}/triage"
		}
		if strings.HasSuffix(pattern, "/approve") {
			return "/api/v1/incidents/{id}/approve"
		}
	}
	if strings.HasPrefix(pattern, "/api/v1/reviews/") && strings.HasSuffix(pattern, "/approve") {
		return "/api/v1/reviews/{id}/approve"
	}
	if strings.HasPrefix(pattern, "/api/v1/connections/") {
		if strings.Contains(pattern, "/secrets/") {
			return "/api/v1/connections/{id}/secrets/{field}"
		}
		if strings.HasSuffix(pattern, "/test") {
			return "/api/v1/connections/{id}/test"
		}
		return "/api/v1/connections/{id}"
	}
	return "UNKNOWN_ROUTE"
}

// NormalizeStatus ensures status labels are strictly bounded.
func NormalizeStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "VALID", "RELEASED", "SUCCESS":
		return "valid"
	case "QUARANTINED", "QUARANTINE":
		return "quarantined"
	case "FAILED", "FAILURE", "ERROR":
		return "failed"
	case "DEAD":
		return "dead"
	case "RETRYABLE":
		return "retryable"
	case "RUNNING":
		return "running"
	case "LEASED":
		return "leased"
	case "QUEUED":
		return "queued"
	default:
		return "unknown"
	}
}
