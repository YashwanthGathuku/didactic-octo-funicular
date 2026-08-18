package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"sentinel-gateway/internal/nacha"
)

type BenchmarkMetrics struct {
	TotalRecordsParsed  int     `json:"totalRecordsParsed"`
	DurationMs          float64 `json:"durationMs"`
	RecordsPerSecond    float64 `json:"recordsPerSecond"`
	ThroughputMBPerSec  float64 `json:"throughputMBPerSec"`
	TotalBytesStreamed  int64   `json:"totalBytesStreamed"`
	AllocatedMemoryKB   uint64  `json:"allocatedMemoryKB"`
	TotalAllocations    uint64  `json:"totalAllocations"`
	Sha256ThroughputMBs float64 `json:"sha256ThroughputMBs"`
	ValidationPassRate  float64 `json:"validationPassRate"`
	EngineIdentifier    string  `json:"engineIdentifier"`
	EntryHashSum        int64   `json:"entryHashSum"`
	ValidRoutingRecords int     `json:"validRoutingRecords"`
}

// BenchmarkResult records detailed percentile, memory, and environment metadata for a benchmark run.
type BenchmarkResult struct {
	PresetName       string    `json:"presetName"`
	DatasetVersion   string    `json:"datasetVersion"`
	RecordCount      int       `json:"recordCount"`
	Concurrency      int       `json:"concurrency"`
	Iterations       int       `json:"iterations"`
	TotalRecords     int64     `json:"totalRecords"`
	TotalBytes       int64     `json:"totalBytes"`
	DurationMs       float64   `json:"durationMs"`
	ThroughputMBs    float64   `json:"throughputMBs"`
	RecordsPerSec    float64   `json:"recordsPerSec"`
	P50LatencyMs     float64   `json:"p50LatencyMs"`
	P95LatencyMs     float64   `json:"p95LatencyMs"`
	P99LatencyMs     float64   `json:"p99LatencyMs"`
	MinLatencyMs     float64   `json:"minLatencyMs"`
	MaxLatencyMs     float64   `json:"maxLatencyMs"`
	MeanLatencyMs    float64   `json:"meanLatencyMs"`
	PeakAllocBytes   uint64    `json:"peakAllocBytes"`
	TotalAllocBytes  uint64    `json:"totalAllocBytes"`
	TotalMallocs     uint64    `json:"totalMallocs"`
	Errors           int       `json:"errors"`
	QuarantinedCount int       `json:"quarantinedCount"`
	ValidCount       int       `json:"validCount"`
	GoVersion        string    `json:"goVersion"`
	NumCPU           int       `json:"numCPU"`
	OS               string    `json:"os"`
	Arch             string    `json:"arch"`
	EngineIdentifier string    `json:"engineIdentifier"`
	Timestamp        time.Time `json:"timestamp"`
}

const nachaRecordWidth = 94

const (
	routingOrigin = "021000021"
	routingRDFI   = "121000358"
)

func pad94(s string) string {
	if len(s) > nachaRecordWidth {
		return s[:nachaRecordWidth]
	}
	return s + strings.Repeat(" ", nachaRecordWidth-len(s))
}

func padNum(n int64, width int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) > width {
		return s[len(s)-width:]
	}
	return strings.Repeat("0", width-len(s)) + s
}

func padText(s string, width int) string {
	if len(s) > width {
		return s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}

// GenerateLargeNachaCorpus creates a high-volume synthetic NACHA stream in memory with conformant 94-char records.
func GenerateLargeNachaCorpus(recordCount int) []byte {
	var builder strings.Builder
	builder.Grow((recordCount+10)*(nachaRecordWidth+2) + 512)

	write := func(line string) {
		builder.WriteString(pad94(line))
		builder.WriteString("\n")
	}

	// 1. File Header
	header := "1" + "01" + " " + routingRDFI + " " + routingOrigin +
		time.Now().Format("060102") + "1200" + "A" + "094" + "10" + "1" +
		padText("DESTINATION BANK", 23) + padText("ORIGIN COMPANY", 23) + padText("", 8)
	write(header)

	// 2. Batch Header (200 mixed, 220 credit-only, PPD)
	bh := "5" + "220" + padText("ORIGIN COMPANY", 16) + padText("", 20) +
		padText("1234567890", 10) + "PPD" + padText("PAYROLL", 10) +
		time.Now().Format("060102") + time.Now().Format("060102") + "   1" + routingOrigin[:8] + "0000001"
	write(bh)

	var entryHashSum int64 = 0
	var totalCredits int64 = 0
	var totalDebits int64 = 0

	for i := 0; i < recordCount; i++ {
		// Entry detail: 6 + 22 (credit) + routingRDFI (9 digits) + account (17) + amount (10) + id (15) + name (22) + discr (2) + addenda (1) + trace (15)
		ed := "6" + "22" + routingRDFI + padText(fmt.Sprintf("ACC%010d", i+1), 17) +
			padNum(10000, 10) + padText(fmt.Sprintf("EMP-%06d", i+1), 15) +
			padText(fmt.Sprintf("PAYROLL ENTRY %06d", i+1), 22) + "  0" +
			routingOrigin[:8] + padNum(int64(i+1), 7)
		write(ed)
		entryHashSum += 12100035 // prefix 8 digits of routingRDFI
		totalCredits += 10000
	}

	// 3. Batch Control
	bc := "8" + "220" + padNum(int64(recordCount), 6) + padNum(entryHashSum%10000000000, 10) +
		padNum(totalDebits, 12) + padNum(totalCredits, 12) +
		padText("1234567890", 10) + padText("", 19) + padText("", 6) +
		routingOrigin[:8] + "0000001"
	write(bc)

	// 4. File Control
	totalRecords := recordCount + 4 // header + batch header + entries + batch control + file control
	_ = totalRecords                // block count not needed since we don't add padding
	fc := "9" + padNum(1, 6) + padNum(int64((recordCount+4+9)/10), 6) + padNum(int64(recordCount), 8) +
		padNum(entryHashSum%10000000000, 10) + padNum(totalDebits, 12) + padNum(totalCredits, 12) +
		padText("", 39)
	write(fc)

	return []byte(builder.String())
}

// RunStreamingBenchmark executes a single-pass streaming parsing and SHA-256 calculation benchmark.
func RunStreamingBenchmark(recordCount int) BenchmarkMetrics {
	if recordCount <= 0 {
		recordCount = 25000
	}

	corpus := GenerateLargeNachaCorpus(recordCount)
	totalBytes := int64(len(corpus))

	var memStart runtime.MemStats
	runtime.ReadMemStats(&memStart)

	start := time.Now()

	// 1. SHA-256 calculation
	hasher := sha256.New()
	hasher.Write(corpus)
	_ = hasher.Sum(nil)
	shaDuration := time.Since(start)

	// 2. Fixed-width NACHA scan using internal/nacha validator
	result, err := nacha.Validate(bytes.NewReader(corpus))
	totalDuration := time.Since(start)

	var memEnd runtime.MemStats
	runtime.ReadMemStats(&memEnd)

	durationSec := totalDuration.Seconds()
	if durationSec <= 0 {
		durationSec = 0.0001
	}

	parsedRecords := 0
	validRoutings := 0
	validationPassRate := 0.0

	if err == nil && result != nil {
		parsedRecords = result.RecordsParsed
		validRoutings = result.EntriesParsed
		if result.EntriesParsed > 0 && len(result.Findings) == 0 {
			validationPassRate = 100.0
		}
	}

	recordsPerSec := float64(parsedRecords) / durationSec
	GlobalMetrics.RecordMeasuredParseRate(recordsPerSec)
	mbPerSec := (float64(totalBytes) / (1024 * 1024)) / durationSec
	shaMBs := (float64(totalBytes) / (1024 * 1024)) / (shaDuration.Seconds() + 0.00001)

	return BenchmarkMetrics{
		TotalRecordsParsed:  parsedRecords,
		DurationMs:          float64(totalDuration.Milliseconds()),
		RecordsPerSecond:    recordsPerSec,
		ThroughputMBPerSec:  mbPerSec,
		TotalBytesStreamed:  totalBytes,
		AllocatedMemoryKB:   (memEnd.TotalAlloc - memStart.TotalAlloc) / 1024,
		TotalAllocations:    memEnd.Mallocs - memStart.Mallocs,
		Sha256ThroughputMBs: shaMBs,
		ValidationPassRate:  validationPassRate,
		EngineIdentifier:    "Sentinel-Go-Nacha-Validator-v1.1",
		EntryHashSum:        0,
		ValidRoutingRecords: validRoutings,
	}
}

// RunHarness runs a reproducible multi-iteration, concurrent benchmark suite across representative dataset sizes.
func RunHarness(preset string, recordCount int, concurrency int, iterations int) BenchmarkResult {
	if recordCount <= 0 {
		switch strings.ToLower(preset) {
		case "small":
			recordCount = 100
		case "medium":
			recordCount = 10_000
		case "large":
			recordCount = 100_000
		default:
			preset = "custom"
			recordCount = 10_000
		}
	}
	if concurrency <= 0 {
		concurrency = 1
	}
	if iterations <= 0 {
		iterations = 5
	}

	corpus := GenerateLargeNachaCorpus(recordCount)
	fileBytes := int64(len(corpus))

	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	latencies := make([]float64, 0, iterations*concurrency)
	var latMu sync.Mutex
	var errCount, quarantinedCount, validCount int
	var countMu sync.Mutex

	totalStart := time.Now()

	var wg sync.WaitGroup
	for c := 0; c < concurrency; c++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				iterStart := time.Now()
				res, err := nacha.Validate(bytes.NewReader(corpus))
				elapsed := time.Since(iterStart)
				elapsedMs := elapsed.Seconds() * 1000.0

				latMu.Lock()
				latencies = append(latencies, elapsedMs)
				latMu.Unlock()

				countMu.Lock()
				if err != nil {
					errCount++
				} else if res != nil {
					decision := nacha.Decide(res, nacha.DefaultContract)
					if decision.Quarantined() {
						quarantinedCount++
					} else {
						validCount++
					}
				}
				countMu.Unlock()
			}
		}()
	}
	wg.Wait()
	totalDuration := time.Since(totalStart)

	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	sort.Float64s(latencies)
	totalSamples := len(latencies)

	var p50, p95, p99, minLat, maxLat, sumLat float64
	if totalSamples > 0 {
		minLat = latencies[0]
		maxLat = latencies[totalSamples-1]
		p50 = latencies[int(float64(totalSamples)*0.50)]
		p95 = latencies[int(float64(totalSamples)*0.95)]
		p99 = latencies[int(float64(totalSamples)*0.99)]
		for _, l := range latencies {
			sumLat += l
		}
	}
	meanLat := sumLat / float64(totalSamples)

	totalProcessedBytes := fileBytes * int64(concurrency*iterations)
	totalRecords := int64(recordCount+4) * int64(concurrency*iterations)
	durationSec := totalDuration.Seconds()
	if durationSec <= 0 {
		durationSec = 0.0001
	}

	throughputMBs := (float64(totalProcessedBytes) / (1024 * 1024)) / durationSec
	recordsPerSec := float64(totalRecords) / durationSec

	GlobalMetrics.RecordMeasuredParseRate(recordsPerSec)

	return BenchmarkResult{
		PresetName:       preset,
		DatasetVersion:   "NACHA-SYNTHETIC-v1.1",
		RecordCount:      recordCount,
		Concurrency:      concurrency,
		Iterations:       iterations,
		TotalRecords:     totalRecords,
		TotalBytes:       totalProcessedBytes,
		DurationMs:       float64(totalDuration.Milliseconds()),
		ThroughputMBs:    throughputMBs,
		RecordsPerSec:    recordsPerSec,
		P50LatencyMs:     p50,
		P95LatencyMs:     p95,
		P99LatencyMs:     p99,
		MinLatencyMs:     minLat,
		MaxLatencyMs:     maxLat,
		MeanLatencyMs:    meanLat,
		PeakAllocBytes:   memAfter.Sys,
		TotalAllocBytes:  memAfter.TotalAlloc - memBefore.TotalAlloc,
		TotalMallocs:     memAfter.Mallocs - memBefore.Mallocs,
		Errors:           errCount,
		QuarantinedCount: quarantinedCount,
		ValidCount:       validCount,
		GoVersion:        runtime.Version(),
		NumCPU:           runtime.NumCPU(),
		OS:               runtime.GOOS,
		Arch:             runtime.GOARCH,
		EngineIdentifier: "Sentinel-Go-Nacha-StreamingValidator-v1.1",
		Timestamp:        time.Now().UTC(),
	}
}
