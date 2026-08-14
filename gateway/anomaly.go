package main

import (
	"fmt"
	"math"
)

type BaselineStats struct {
	MeanRecords float64 `json:"meanRecords"`
	StdDevRecords float64 `json:"stdDevRecords"`
	MeanBytes float64 `json:"meanBytes"`
	StdDevBytes float64 `json:"stdDevBytes"`
	SampleCount int `json:"sampleCount"`
}

type VolumeAnomalyFinding struct {
	IsAnomaly bool `json:"isAnomaly"`
	ZScore float64 `json:"zScore"`
	MetricName string `json:"metricName"`
	ActualValue float64 `json:"actualValue"`
	ExpectedMean float64 `json:"expectedMean"`
	DeviationPct float64 `json:"deviationPct"`
	Severity string `json:"severity"`
	Explanation string `json:"explanation"`
}

// Default baseline for Meridian Commercial ACH (e.g. ~10,000 records ± 1,500)
var DefaultBaseline = BaselineStats{
	MeanRecords:   10000.0,
	StdDevRecords: 1500.0,
	MeanBytes:     940000.0,
	StdDevBytes:   141000.0,
	SampleCount:   30,
}

// EvaluateVolumeAnomaly compares actual file metrics against rolling statistical baselines.
func EvaluateVolumeAnomaly(actualRecords int, actualBytes int64, baseline BaselineStats) *VolumeAnomalyFinding {
	if baseline.StdDevRecords <= 0 {
		baseline.StdDevRecords = 1.0
	}

	zScore := (float64(actualRecords) - baseline.MeanRecords) / baseline.StdDevRecords
	absZ := math.Abs(zScore)
	deviationPct := ((float64(actualRecords) - baseline.MeanRecords) / baseline.MeanRecords) * 100.0

	// 3-Sigma threshold (99.7% confidence interval)
	if absZ >= 3.0 {
		severity := "WARNING"
		if absZ >= 5.0 {
			severity = "CRITICAL"
		}

		direction := "spike"
		if zScore < 0 {
			direction = "drop"
		}

		return &VolumeAnomalyFinding{
			IsAnomaly:    true,
			ZScore:       math.Round(zScore*100) / 100,
			MetricName:   "Record Count",
			ActualValue:  float64(actualRecords),
			ExpectedMean: baseline.MeanRecords,
			DeviationPct: math.Round(deviationPct*10) / 10,
			Severity:     severity,
			Explanation:  fmt.Sprintf("Statistical %s detected: %d records deviates by %.1f%% (|Z|=%.2f > 3.0σ baseline).", direction, actualRecords, math.Abs(deviationPct), absZ),
		}
	}

	return &VolumeAnomalyFinding{
		IsAnomaly:    false,
		ZScore:       math.Round(zScore*100) / 100,
		MetricName:   "Record Count",
		ActualValue:  float64(actualRecords),
		ExpectedMean: baseline.MeanRecords,
		DeviationPct: math.Round(deviationPct*10) / 10,
		Severity:     "INFO",
		Explanation:  "File volume conforms within expected 3-Sigma statistical baseline.",
	}
}
