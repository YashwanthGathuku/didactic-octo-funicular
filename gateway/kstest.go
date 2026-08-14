package main

import (
	"errors"
	"math"
	"sort"
)

// ---------------------------------------------------------------------------
// Two-sample Kolmogorov-Smirnov test.
//
// The previous drift.go advertised "Normalized Kolmogorov-Smirnov or Chi-Sq
// D-statistic" and returned hardcoded literals. No test was performed. This
// file implements the real thing.
//
// Definition. For samples X_1..X_n ~ F and Y_1..Y_m ~ G with empirical CDFs
// F_n and G_m, the two-sample KS statistic is
//
//	D_{n,m} = sup_x |F_n(x) - G_m(x)|
//
// Because both ECDFs are step functions that only change at sample points, the
// supremum is attained at one of the n+m observations, so D is computed exactly
// by a single merge pass -- no discretisation, no binning.
//
// Null distribution. Under H0: F = G, Smirnov (1948) showed that with
// n_e = nm/(n+m),
//
//	lim_{n,m->inf} P( sqrt(n_e) D_{n,m} <= t ) = K(t) = 1 - 2 sum_{k=1}^inf (-1)^{k-1} e^{-2k^2 t^2}
//
// so the asymptotic p-value is
//
//	p = Q_KS(t) = 2 sum_{k=1}^inf (-1)^{k-1} e^{-2 k^2 t^2},  t = sqrt(n_e) D
//
// The series is alternating with rapidly decreasing terms, so truncation error
// is bounded by the first omitted term. We use the standard Stephens (1970)
// small-sample correction t = (sqrt(n_e) + 0.12 + 0.11/sqrt(n_e)) * D, which
// makes the asymptotic form usable down to roughly n_e >= 4.
//
// WHY THIS MATTERS FOR FILE MONITORING. KS is distribution-free: the null
// distribution of D does not depend on F. That is exactly the property you want
// for counterparty file metrics (record counts, byte sizes, amount totals),
// which are right-skewed and heavy-tailed and badly violate the Gaussian
// assumption behind a 3-sigma rule.
//
// KNOWN LIMITATION, stated rather than hidden: KS is most sensitive near the
// centre of the distribution and comparatively insensitive in the tails. For
// tail-focused drift (rare large batches) an Anderson-Darling or a weighted
// KS variant is the better instrument. See the accompanying research notes.
// ---------------------------------------------------------------------------

// KSResult carries the statistic, its p-value, and the location of the maximum
// divergence -- the last of which is the operationally useful part, because it
// tells an analyst *where* in the distribution the counterparty changed.
type KSResult struct {
	D             float64 `json:"d"`
	PValue        float64 `json:"pValue"`
	EffectiveN    float64 `json:"effectiveN"`
	NBaseline     int     `json:"nBaseline"`
	NCurrent      int     `json:"nCurrent"`
	ArgMaxX       float64 `json:"argMaxX"`
	IsSignificant bool    `json:"isSignificant"`
	Alpha         float64 `json:"alpha"`
}

// KolmogorovSmirnovQ evaluates the complementary Kolmogorov distribution
//
//	Q(t) = P(sup_x |B(x)| > t) = 2 sum_{k=1}^inf (-1)^{k-1} exp(-2 k^2 t^2)
//
// NUMERICAL NOTE. The alternating series above converges super-exponentially for
// large t but arbitrarily SLOWLY as t -> 0: every term exp(-2k^2 t^2) -> 1, so a
// naive truncation returns a badly wrong value in the small-t regime. (This was
// caught by TestKSQIsMonotoneAndBounded, which found Q non-monotone near t=0.1.)
//
// Jacobi's theta-function transformation gives an equivalent series that
// converges fastest exactly where the first one fails:
//
//	Q(t) = 1 - (sqrt(2 pi) / t) * sum_{k=1}^inf exp( -(2k-1)^2 pi^2 / (8 t^2) )
//
// We switch at t = 1, where both forms converge in ~3 terms and agree to ~1e-15.
// This is the standard treatment (Marsaglia, Tsang & Wang 2003; Press et al.,
// Numerical Recipes s14.3).
func KolmogorovSmirnovQ(t float64) float64 {
	if t <= 0 {
		return 1.0
	}
	const eps = 1e-15

	var p float64
	if t < 1.0 {
		// Theta-transformed series: accurate for small t.
		sum := 0.0
		for k := 1; k <= 100; k++ {
			m := float64(2*k - 1)
			term := math.Exp(-(m * m * math.Pi * math.Pi) / (8.0 * t * t))
			sum += term
			if term < eps {
				break
			}
		}
		p = 1.0 - (math.Sqrt(2.0*math.Pi)/t)*sum
	} else {
		// Alternating series: accurate for large t, error bounded by the first
		// omitted term.
		sum, sign := 0.0, 1.0
		for k := 1; k <= 100; k++ {
			term := math.Exp(-2.0 * float64(k) * float64(k) * t * t)
			sum += sign * term
			sign = -sign
			if term < eps {
				break
			}
		}
		p = 2.0 * sum
	}

	return math.Min(1.0, math.Max(0.0, p))
}

// TwoSampleKS computes D_{n,m}, the asymptotic p-value, and the x at which the
// maximum ECDF gap occurs. Inputs are copied before sorting so caller slices
// are not mutated.
func TwoSampleKS(baseline, current []float64, alpha float64) (KSResult, error) {
	n, m := len(baseline), len(current)
	if n < 2 || m < 2 {
		return KSResult{}, errors.New("two-sample KS requires at least 2 observations in each sample")
	}
	if alpha <= 0 || alpha >= 1 {
		alpha = 0.05
	}

	a := append([]float64(nil), baseline...)
	b := append([]float64(nil), current...)
	sort.Float64s(a)
	sort.Float64s(b)

	i, j := 0, 0
	var cdfA, cdfB, d, argmax float64
	fn, fm := float64(n), float64(m)

	for i < n && j < m {
		// Advance whichever ECDF is behind; ties advance both so the gap is
		// evaluated only after all equal values are consumed (correct handling
		// of discrete/tied data such as integer record counts).
		x := math.Min(a[i], b[j])
		for i < n && a[i] == x {
			i++
			cdfA = float64(i) / fn
		}
		for j < m && b[j] == x {
			j++
			cdfB = float64(j) / fm
		}
		if gap := math.Abs(cdfA - cdfB); gap > d {
			d = gap
			argmax = x
		}
	}

	ne := fn * fm / (fn + fm)
	sqrtNe := math.Sqrt(ne)
	// Stephens (1970) finite-sample correction.
	t := (sqrtNe + 0.12 + 0.11/sqrtNe) * d
	p := KolmogorovSmirnovQ(t)

	return KSResult{
		D:             d,
		PValue:        p,
		EffectiveN:    ne,
		NBaseline:     n,
		NCurrent:      m,
		ArgMaxX:       argmax,
		IsSignificant: p < alpha,
		Alpha:         alpha,
	}, nil
}

// BenjaminiHochberg returns, for a set of p-values, the boolean rejection
// vector controlling the false discovery rate at level q.
//
// This exists because a monitoring platform runs one test per counterparty per
// metric per day. At 2,000 feeds x 4 metrics and a naive alpha = 0.05 you
// generate ~400 false alerts every single day -- which is precisely the alert
// fatigue the product claims to solve. Benjamini-Hochberg (1995) guarantees
// E[FDR] <= q under independence and under positive regression dependence
// (Benjamini-Yekutieli 2001), which is the realistic regime here since feeds
// from the same counterparty are positively correlated.
//
// Ordering: sort p_(1) <= ... <= p_(N); find the largest k with
// p_(k) <= (k/N) q; reject H_(1)..H_(k).
func BenjaminiHochberg(pvalues []float64, q float64) []bool {
	n := len(pvalues)
	out := make([]bool, n)
	if n == 0 {
		return out
	}
	if q <= 0 || q >= 1 {
		q = 0.05
	}
	type pi struct {
		p float64
		i int
	}
	ps := make([]pi, n)
	for i, p := range pvalues {
		ps[i] = pi{p, i}
	}
	sort.Slice(ps, func(x, y int) bool { return ps[x].p < ps[y].p })

	k := -1
	for idx := n - 1; idx >= 0; idx-- {
		if ps[idx].p <= (float64(idx+1)/float64(n))*q {
			k = idx
			break
		}
	}
	for idx := 0; idx <= k; idx++ {
		out[ps[idx].i] = true
	}
	return out
}
