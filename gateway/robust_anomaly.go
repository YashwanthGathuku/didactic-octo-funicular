package main

import (
	"fmt"
	"math"
	"sort"
)

// ---------------------------------------------------------------------------
// Robust volume anomaly detection.
//
// PROBLEM WITH THE EXISTING 3-SIGMA RULE (anomaly.go):
//
//  1. Mean and standard deviation are not robust. Their breakdown point is 0:
//     a single contaminating observation can move them arbitrarily far. In this
//     domain that contaminant is guaranteed -- the first genuinely anomalous
//     file enters the rolling window and inflates sigma, which raises the
//     threshold, which masks the next anomaly. This is classic masking.
//
//  2. "3 sigma = 99.7%" only holds for a Gaussian. Daily file record counts are
//     right-skewed, bounded below at zero, and have month-end/quarter-end
//     spikes. Under skew, the true tail mass beyond mu+3sigma is far above
//     0.3%, so the observed false-alarm rate exceeds the advertised one.
//     Distribution-free, Chebyshev only guarantees P(|X-mu| >= 3 sigma) <= 1/9
//     = 11.1% -- two orders of magnitude weaker than the claimed 0.3%.
//
//  3. The baseline was a hardcoded constant (mean 10000, sd 1500, n=30), so
//     nothing actually adapted per counterparty.
//
// FIX: median + MAD (median absolute deviation).
//
//	MAD = median_i( |x_i - median(x)| )
//
// MAD has the highest possible breakdown point (50%): up to half the window can
// be arbitrarily corrupted before the estimate is destroyed. Scaling by
// 1/Phi^{-1}(3/4) = 1.4826 makes MAD a consistent estimator of sigma for
// Gaussian data, so thresholds remain interpretable.
//
// The modified z-score (Iglewicz & Hoaglin, 1993) is
//
//	M_i = 0.6745 (x_i - median) / MAD
//
// with a recommended flag threshold of |M_i| > 3.5.
//
// EDGE CASE that a naive implementation gets wrong: if more than half the
// window holds an identical value (common for feeds with a constant daily
// record count), MAD = 0 and M_i explodes to +/-Inf. We fall back to the
// average absolute deviation, per Iglewicz & Hoaglin's own recommendation.
// ---------------------------------------------------------------------------

type RobustAnomalyFinding struct {
	IsAnomaly        bool    `json:"isAnomaly"`
	ModifiedZ        float64 `json:"modifiedZ"`
	Median           float64 `json:"median"`
	MAD              float64 `json:"mad"`
	ScaledMAD        float64 `json:"scaledMad"`
	ActualValue      float64 `json:"actualValue"`
	DeviationPct     float64 `json:"deviationPct"`
	Threshold        float64 `json:"threshold"`
	SampleCount      int     `json:"sampleCount"`
	Severity         string  `json:"severity"`
	FallbackUsed     bool    `json:"fallbackUsed"`
	Explanation      string  `json:"explanation"`
	InsufficientData bool    `json:"insufficientData"`
}

const madToSigma = 1.4826 // 1 / Phi^{-1}(0.75)

func medianOf(sorted []float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2.0
}

// MedianAndMAD returns the median and the raw (unscaled) MAD of a sample.
func MedianAndMAD(sample []float64) (float64, float64) {
	if len(sample) == 0 {
		return 0, 0
	}
	s := append([]float64(nil), sample...)
	sort.Float64s(s)
	med := medianOf(s)

	dev := make([]float64, len(s))
	for i, v := range s {
		dev[i] = math.Abs(v - med)
	}
	sort.Float64s(dev)
	return med, medianOf(dev)
}

// EvaluateRobustVolumeAnomaly scores an observation against a rolling history
// using the modified z-score. history should be the counterparty's own recent
// same-weekday observations; 20+ points is a sensible minimum.
func EvaluateRobustVolumeAnomaly(actual float64, history []float64, threshold float64) RobustAnomalyFinding {
	if threshold <= 0 {
		threshold = 3.5
	}
	if len(history) < 8 {
		return RobustAnomalyFinding{
			InsufficientData: true,
			ActualValue:      actual,
			SampleCount:      len(history),
			Threshold:        threshold,
			Severity:         "INFO",
			Explanation: fmt.Sprintf(
				"Only %d historical observations; refusing to assert an anomaly. A baseline needs >=8 points (>=20 recommended) before any threshold is meaningful.",
				len(history)),
		}
	}

	med, mad := MedianAndMAD(history)
	scaled := mad * madToSigma
	fallback := false

	if mad == 0 {
		// More than half the window is identical. Use mean absolute deviation.
		sum := 0.0
		for _, v := range history {
			sum += math.Abs(v - med)
		}
		meanAbsDev := sum / float64(len(history))
		if meanAbsDev == 0 {
			// Genuinely constant history: any deviation at all is notable.
			isAnom := actual != med
			return RobustAnomalyFinding{
				IsAnomaly:    isAnom,
				Median:       med,
				MAD:          0,
				ActualValue:  actual,
				SampleCount:  len(history),
				Threshold:    threshold,
				FallbackUsed: true,
				Severity:     severityFor(isAnom, math.Inf(1)),
				Explanation:  "History is perfectly constant; any deviation is flagged. Verify the feed is not a stuck fixture.",
			}
		}
		scaled = meanAbsDev * 1.253314 // consistency factor for MeanAD under normality
		fallback = true
	}

	mz := 0.6745 * (actual - med) / (scaled / madToSigma)
	if fallback {
		mz = (actual - med) / scaled
	}

	devPct := 0.0
	if med != 0 {
		devPct = (actual - med) / med * 100.0
	}
	isAnom := math.Abs(mz) > threshold

	direction := "spike"
	if mz < 0 {
		direction = "drop"
	}
	expl := fmt.Sprintf(
		"Within robust baseline (|M|=%.2f <= %.1f). median=%.0f, scaled MAD=%.0f over %d observations.",
		math.Abs(mz), threshold, med, scaled, len(history))
	if isAnom {
		expl = fmt.Sprintf(
			"Robust %s: %.0f vs median %.0f (%.1f%%), modified z=%.2f exceeds %.1f. Estimator is median/MAD (50%% breakdown), so this is not an artefact of an earlier outlier inflating the baseline.",
			direction, actual, med, math.Abs(devPct), mz, threshold)
	}

	return RobustAnomalyFinding{
		IsAnomaly:    isAnom,
		ModifiedZ:    math.Round(mz*100) / 100,
		Median:       med,
		MAD:          mad,
		ScaledMAD:    scaled,
		ActualValue:  actual,
		DeviationPct: math.Round(devPct*10) / 10,
		Threshold:    threshold,
		SampleCount:  len(history),
		Severity:     severityFor(isAnom, math.Abs(mz)),
		FallbackUsed: fallback,
		Explanation:  expl,
	}
}

func severityFor(isAnom bool, absMZ float64) string {
	if !isAnom {
		return "INFO"
	}
	if absMZ >= 7.0 {
		return "CRITICAL"
	}
	return "WARNING"
}
