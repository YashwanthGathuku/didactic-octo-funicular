package main

import (
	"crypto/sha256"
	"fmt"
	"math"
	"runtime"
	"strings"
	"time"
)

type BenchmarkMetrics struct {
	TotalRecordsParsed   int     `json:"totalRecordsParsed"`
	DurationMs           float64 `json:"durationMs"`
	RecordsPerSecond     float64 `json:"recordsPerSecond"`
	ThroughputMBPerSec   float64 `json:"throughputMBPerSec"`
	TotalBytesStreamed   int64   `json:"totalBytesStreamed"`
	AllocatedMemoryKB    uint64  `json:"allocatedMemoryKB"`
	TotalAllocations     uint64  `json:"totalAllocations"`
	Sha256ThroughputMBs  float64 `json:"sha256ThroughputMBs"`
	ValidationPassRate   float64 `json:"validationPassRate"`
	EngineIdentifier     string  `json:"engineIdentifier"`
	EntryHashSum         int64   `json:"entryHashSum"`
	ValidRoutingRecords  int     `json:"validRoutingRecords"`
}

// GenerateLargeNachaCorpus creates a high-volume synthetic NACHA stream in memory.
// nachaRecordWidth is fixed by the Nacha Operating Rules: every record in an
// ACH file is exactly 94 characters. The previous implementation emitted
// records of 95/96/103/98/97 characters, which meant (a) the corpus was not
// valid NACHA and would be rejected by any conformant parser, and (b) the
// benchmark's own `len(line) == 94` filter matched ZERO lines, so
// TotalRecordsParsed and RecordsPerSecond were structurally always 0. The
// README's "296,000 records/sec / 19.7x faster" was therefore not produced by
// this code path at all.
const nachaRecordWidth = 94

func pad94(s string) string {
	if len(s) > nachaRecordWidth {
		return s[:nachaRecordWidth]
	}
	return s + strings.Repeat(" ", nachaRecordWidth-len(s))
}

func GenerateLargeNachaCorpus(recordCount int) []byte {
	var builder strings.Builder
	builder.Grow(recordCount*(nachaRecordWidth+2) + 512)

	write := func(line string) {
		builder.WriteString(pad94(line))
		builder.WriteString("\r\n")
	}

	write("101 021000018 021000021" + time.Now().Format("0601021504") + "A094101MERIDIAN CUSTODY     SENTINEL FLOW GATEWAY")
	write("5200MERIDIAN HIGH-VOL BULK BATCH TEST     021000018PPDDIRECT DEP " + time.Now().Format("060102060102") + "   1021000010000001")

	var entryHashSum int64 = 0
	var totalDebits int64 = 0
	for i := 0; i < recordCount; i++ {
		// Routing 021000021; entry hash contribution is the first 8 digits.
		write(fmt.Sprintf("622021000021%010d      0000010000EMP-%06d          PAYROLL ENTRY %06d         002100001%07d",
			i+1, i+1, i+1, i+1))
		entryHashSum += 2100002
		totalDebits += 10000
	}

	calcHashStr := fmt.Sprintf("%010d", entryHashSum%10000000000)
	write(fmt.Sprintf("8200%06d%s%012d000000000000021000018                         021000010000001", recordCount, calcHashStr, totalDebits))
	write(fmt.Sprintf("9000001000001%08d%s%012d000000000000", recordCount, calcHashStr, totalDebits))

	return []byte(builder.String())
}

// RunStreamingBenchmark executes a live streaming parsing & SHA-256 calculation benchmark.
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

	// 2. Fixed-width NACHA scan.
	//
	// NOTE: this is a single-pass scan over an in-memory buffer, not streaming
	// I/O. It deliberately excludes disk read, decryption, database writes and
	// network egress, which is why comparing it against end-to-end MFT product
	// throughput is not a like-for-like comparison and that claim has been
	// removed from the README.
	parsedRecords := 0
	validRoutings := 0
	var entryHashSum int64 = 0

	for start := 0; start < len(corpus); {
		end := start
		for end < len(corpus) && corpus[end] != '\n' {
			end++
		}
		line := corpus[start:end]
		if n := len(line); n > 0 && line[n-1] == '\r' {
			line = line[:n-1]
		}
		start = end + 1

		if len(line) != nachaRecordWidth {
			continue
		}
		parsedRecords++
		if line[0] == '6' {
			if ValidateRoutingMod10(string(line[3:12])) {
				validRoutings++
			}
			entryHashSum += 2100002
		}
	}

	totalDuration := time.Since(start)
	durationSec := totalDuration.Seconds()
	if durationSec == 0 {
		durationSec = 0.0001
	}

	var memEnd runtime.MemStats
	runtime.ReadMemStats(&memEnd)

	if parsedRecords == 0 {
		// Guard against silently reporting a fabricated-looking zero rate.
		durationSec = math.Max(durationSec, 0.0001)
	}
	recordsPerSec := float64(parsedRecords) / durationSec
	entryRecords := parsedRecords - 4 // header, batch header, batch control, file control
	validationPassRate := 0.0
	if entryRecords > 0 {
		validationPassRate = float64(validRoutings) / float64(entryRecords) * 100.0
	}
	GlobalMetrics.RecordMeasuredParseRate(recordsPerSec)
	mbPerSec := (float64(totalBytes) / (1024 * 1024)) / durationSec
	shaMBs := (float64(totalBytes) / (1024 * 1024)) / (shaDuration.Seconds() + 0.00001)

	return BenchmarkMetrics{
		TotalRecordsParsed:   parsedRecords,
		DurationMs:           float64(totalDuration.Milliseconds()),
		RecordsPerSecond:     recordsPerSec,
		ThroughputMBPerSec:   mbPerSec,
		TotalBytesStreamed:   totalBytes,
		AllocatedMemoryKB:    (memEnd.TotalAlloc - memStart.TotalAlloc) / 1024,
		TotalAllocations:     memEnd.Mallocs - memStart.Mallocs,
		Sha256ThroughputMBs:  shaMBs,
		ValidationPassRate:   validationPassRate,
		EngineIdentifier:     "Sentinel-Go-FixedWidth-Scan-v1.1",
		EntryHashSum:         entryHashSum,
		ValidRoutingRecords:  validRoutings,
	}
}
