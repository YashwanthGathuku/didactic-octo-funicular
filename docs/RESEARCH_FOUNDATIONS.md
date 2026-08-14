# Research Foundations for Sentinel Flow

**Prepared for:** Yashwanth (Ash)
**Date:** 14 August 2026
**Purpose:** Replace the asserted numbers in the current codebase with results that carry
actual proofs, and state honestly where no proof exists.

---

## How to read this

Each section states: the **claim the product needs**, the **theorem that supplies it**, the
**assumption the theorem requires**, and — the part usually omitted — **what breaks when
that assumption fails in this specific domain**.

A guarantee whose precondition you cannot verify is a decoration. Every section below
names its precondition.

---

## 1. The core claim: "we will tell you before the file is late"

This is the product. Everything else is supporting cast. It is also the one claim the
current codebase supports least — `BreachRiskPct` is computed nowhere and the drift
engine returned literals.

### 1.1 What you actually need

A statement of the form: *"the file will arrive by time T with probability ≥ 1 − α,"*
where that probability is **calibrated** — i.e. when you say 95%, you are right 95% of
the time. An uncalibrated risk score is worse than none, because operations teams learn
its true hit rate within two weeks and then ignore it.

### 1.2 The theorem: split conformal prediction

**Vovk, Gammerman & Shafer (2005),** *Algorithmic Learning in a Random World*, Springer.
**Papadopoulos, Proedrou, Vovk & Gammerman (2002),** "Inductive Confidence Machines for
Regression," ECML.
**Lei, G'Sell, Rinaldo, Tibshirani & Wasserman (2018),** "Distribution-Free Predictive
Inference for Regression," *JASA* 113(523).

**Construction.** Fit any arrival-time predictor `μ̂` on a training split. On a held-out
calibration set of size *n*, compute nonconformity scores `sᵢ = |yᵢ − μ̂(xᵢ)|`. Let
`q̂` be the `⌈(n+1)(1−α)⌉`-th smallest score. Predict the interval
`C(x) = [μ̂(x) − q̂, μ̂(x) + q̂]`.

**Theorem.** If the calibration scores and the test score are exchangeable, then

```
P( Y_test ∈ C(X_test) ) ≥ 1 − α
```

**Proof sketch (worth internalising — it is three lines).** Under exchangeability, the
test score `s_{n+1}` is equally likely to occupy any of the `n+1` ranks among
`{s₁,…,s_{n+1}}`. So `P(s_{n+1} > q̂)` is just the fraction of ranks above the
`⌈(n+1)(1−α)⌉`-th, which is at most α. Coverage failure is a rank event; no distributional
assumption enters anywhere.

**Why this is the right instrument here.** The guarantee is
*finite-sample* (holds at n=50 calibration days, not "asymptotically"), *distribution-free*
(makes no Gaussian claim about arrival times, which are right-skewed and bounded below),
and *model-agnostic* (wraps whatever forecaster you already have — LightGBM, Prophet,
a seasonal median). <cite index="107-1">Notably, conformal methods provide a finite-sample coverage guarantee, not an asymptotic or approximate guarantee; conformal prediction does not require any assumptions about the distribution P — only an exchangeable sampling design; and the guarantee holds even if the predictive model is not a good fit</cite>.

### 1.3 The precondition that fails, and the fix

<cite index="104-1">These assumptions cease to hold for time series due to their temporally correlated nature.</cite>
File arrival times are emphatically not exchangeable: month-end differs from mid-month,
a counterparty's ERP migration shifts the whole distribution, holidays reshape the week.

Three papers address this directly:

- **Gibbs & Candès (2021),** "Adaptive Conformal Inference Under Distribution Shift,"
  NeurIPS. Updates α online via `α_{t+1} = α_t + γ(α − err_t)`. Guarantees long-run
  coverage converges to 1−α *regardless of how the distribution shifts*, including
  adversarially. This is the single most applicable paper in this document — it is
  designed for exactly your setting, where a counterparty silently changes behaviour.
- **Barber, Candès, Ramdas & Tibshirani (2023),** "Conformal Prediction Beyond
  Exchangeability," *Annals of Statistics* 51(2). Weighted conformal with an explicit
  coverage gap bounded by the total-variation distance between the weighted calibration
  distribution and the test distribution. You get a guarantee *plus a number quantifying
  how much the non-exchangeability cost you*.
- **Romano, Patterson & Candès (2019),** "Conformalized Quantile Regression," NeurIPS.
  Produces intervals of *varying* width. This matters operationally: a counterparty with
  a tight 10-minute arrival window and one with a 4-hour window should not get the same
  band.

### 1.4 Concrete recommendation

Implement **CQR + adaptive conformal (ACI)** per counterparty-feed:
1. Base quantile model on features: day-of-week, business-day-of-month, holiday proximity
   (Federal Reserve calendar), upstream dependency status, trailing 20-day arrival median.
2. Conformalize with the last 60 same-weekday observations.
3. Wrap in ACI with γ ≈ 0.005 so calibration self-corrects after a partner changes systems.
4. Report `P(breach) = 1 − (conformal CDF evaluated at the cutoff)` and **publish your
   realised coverage** on the dashboard. A monitoring product that publishes its own
   calibration curve is a genuinely differentiated claim, and it is one you can prove.

---

## 2. "Detect the change as fast as possible" — quickest change detection

Your drift profiler asks the wrong question. A KS test on a 30-day window answers *"are
these two batches different?"* The operational question is *"a change started at some
unknown time; flag it with minimum delay subject to a false-alarm budget."* That is a
different problem with its own optimality theory.

### 2.1 CUSUM and its exact optimality

**Page (1954),** "Continuous Inspection Schemes," *Biometrika* 41.
**Lorden (1971),** "Procedures for Reacting to a Change in Distribution," *Ann. Math.
Statist.* 42(6) — asymptotic minimax optimality.
**Moustakides (1986),** "Optimal Stopping Times for Detecting Changes in Distributions,"
*Annals of Statistics* 15(2), 749–779 — **exact** optimality.

**Statistic.** With pre-change density `f₀` and post-change `f₁`:

```
W_n = max(0, W_{n-1} + log( f₁(X_n) / f₀(X_n) )),   W₀ = 0
τ   = min{ n : W_n ≥ B }
```

**Why it works.** The log-likelihood-ratio increment has negative drift `−D(f₀‖f₁)` before
the change and positive drift `+D(f₁‖f₀)` after, where `D` is KL divergence. The `max(0,·)`
reflects the random walk at zero so pre-change evidence does not accumulate.

**Theorem (Moustakides 1986).** CUSUM **exactly** minimises Lorden's worst-case expected
detection delay subject to a constraint on the average run length to false alarm — not
merely asymptotically. <cite index="123-1">Lorden showed that the CuSum algorithm is asymptotically optimal according to his minimax criterion for delay, as the mean time to false alarm goes to infinity. This result was improved upon by Moustakides who showed that the CuSum algorithm is exactly optimal under Lorden's criterion.</cite> <cite index="128-1">Moustakides established the exact optimality of CUSUM for any ARL to false alarm γ ≥ 1.</cite>

This is a genuine, citable optimality result — the strongest available for "detect a
distribution shift with minimum delay." It is the correct engine for *"this counterparty's
arrival times have permanently shifted 40 minutes later."*

**Also relevant:** Pollak (1985), *Ann. Statist.* 13(1), 206–227 — Shiryaev-Roberts under a
less pessimistic delay criterion; Lai (1998) for the composite/unknown-post-change case,
which is your real situation since you never know the post-change distribution in advance.

### 2.2 Practical note

Use a **window-limited CUSUM** (Xie, Moustakides & Xie, 2022, arXiv:2206.06777) which
estimates post-change parameters in a sliding window and retains first-order asymptotic
optimality under Lorden's criterion. It handles the "unknown `f₁`" problem without
assuming an exponential family.

---

## 3. Continuous monitoring breaks classical p-values

This is the subtlest and most important statistical point in the whole design, and the
current architecture walks straight into it.

**Problem.** The implementation plan specifies `P(Breach)` updating **every 60 seconds**.
Under classical testing, if you evaluate a p-value repeatedly and alert the first time it
drops below α, your actual Type I error tends to **1** by the law of the iterated
logarithm. Your 5% false-alarm rate is not 5%. It is eventually 100%.

**Solution: anytime-valid inference via test martingales.**

**Ramdas, Grünwald, Vovk & Shafer (2023),** "Game-Theoretic Statistics and Safe
Anytime-Valid Inference," *Statistical Science* 38(4), 576–601. DOI 10.1214/23-STS894.
**Ville (1939),** *Étude critique de la notion de collectif.*
**Howard, Ramdas, McAuliffe & Sekhon (2021),** "Time-Uniform Chernoff Bounds via
Nonnegative Supermartingales," *Probability Surveys*.

**Ville's inequality.** For a nonnegative supermartingale `(M_t)` with `M₀ = 1`:

```
P( ∃ t ≥ 1 : M_t ≥ 1/α ) ≤ α
```

Note the `∃t`. The bound holds **simultaneously over all times**, so you may look at the
statistic as often as you like, stop whenever you like, and the error guarantee survives.

<cite index="132-1">Safe anytime-valid inference provides measures of statistical evidence and certainty — e-processes for testing and confidence sequences for estimation — that remain valid at all stopping times, accommodating continuous monitoring and analysis of accumulating data and optional stopping or continuation for any reason. These measures crucially rely on test martingales, which are nonnegative martingales starting at one.</cite>

**Why it matters commercially, not just mathematically.** Every competitor's SLA dashboard
that refreshes a p-value or a "risk score" on a timer has this bug and does not know it.
Shipping confidence sequences instead of confidence intervals is a defensible technical
differentiator that a quant at a custody bank will immediately recognise.

**Also see:** Shekhar & Ramdas (2023), "Reducing Sequential Change Detection to Sequential
Estimation," arXiv:2309.09111 — connects §2 and §3 into one framework.

---

## 4. Alert volume: the arithmetic that decides whether anyone keeps the product

**Benjamini & Hochberg (1995),** "Controlling the False Discovery Rate," *JRSS-B* 57(1).
**Benjamini & Yekutieli (2001),** *Annals of Statistics* 29(4) — validity under positive
regression dependence.

**The arithmetic you must put in the pitch.** 2,000 feeds × 4 metrics = 8,000 tests/day.
At a nominal α = 0.05 that is **400 false alerts every day**. At the "3σ = 0.3%" the code
assumes, still ~24/day. Either number destroys the "85–95% alert noise reduction" claim
in the README before a single line of ML runs.

**BH procedure.** Sort `p₍₁₎ ≤ … ≤ p₍ₙ₎`. Find the largest *k* with `p₍ₖ₎ ≤ (k/N)q`.
Reject `H₍₁₎…H₍ₖ₎`. Then `E[FDR] ≤ q`.

Feeds from the same counterparty are positively correlated (shared ERP, shared network
path), which is exactly the PRDS regime where Benjamini-Yekutieli shows plain BH remains
valid — you do **not** need the conservative log-factor correction. Worth knowing, because
the naive move is to apply Bonferroni and destroy your power.

**Implemented:** `gateway/kstest.go → BenjaminiHochberg()`, tested in
`integrity_test.go → TestBenjaminiHochbergControlsFDR`.

---

## 5. Distribution comparison — what the KS test does and does not give you

**Smirnov (1948),** "Table for Estimating the Goodness of Fit of Empirical Distributions,"
*Ann. Math. Statist.* 19(2).
**Massart (1990),** "The Tight Constant in the DKW Inequality," *Annals of Probability*
18(3) — this is the one that gives you a **finite-sample** bound.
**Marsaglia, Tsang & Wang (2003),** "Evaluating Kolmogorov's Distribution," *J. Statistical
Software* 8(18) — numerically stable evaluation.

**Dvoretzky-Kiefer-Wolfowitz with Massart's constant:**

```
P( sup_x |F_n(x) − F(x)| > ε ) ≤ 2 e^{−2nε²}
```

Unlike the asymptotic KS p-value, this holds **at every n**. Inverting it gives a
distribution-free confidence band around the empirical CDF, which is the honest way to
show a counterparty "here is your arrival distribution and here is its uncertainty."

**Stated limitation.** KS is most sensitive near the distribution's centre and weak in the
tails — precisely where late-file risk lives. For tail-sensitive drift use
**Anderson & Darling (1954),** *JASA* 49(268), which weights by `1/(F(x)(1−F(x)))`, or the
**Cramér-von Mises** family. Do not claim KS detects tail drift well; it does not.

**Implemented:** `gateway/kstest.go`. Uses the alternating series for t ≥ 1 and the
Jacobi theta-transformed series for t < 1 (the naive single-series implementation is
numerically wrong near t → 0 — this was caught by
`TestKSQIsMonotoneAndBounded` after I wrote the bug myself).

---

## 6. Robust baselines — why 3σ fails and what replaces it

**Iglewicz & Hoaglin (1993),** *How to Detect and Handle Outliers*, ASQC Quality Press —
the modified z-score `M_i = 0.6745(x_i − median)/MAD`, threshold 3.5.
**Hampel (1974),** "The Influence Curve and Its Role in Robust Estimation," *JASA* 69(346)
— breakdown point and influence function formalism.
**Rousseeuw & Croux (1993),** "Alternatives to the Median Absolute Deviation," *JASA*
88(424) — **the upgrade you should make next.**

**The masking argument, formally.** The breakdown point of the sample mean and standard
deviation is `1/n → 0`: one arbitrarily large observation moves them without bound. So the
first genuine anomaly to enter a rolling window inflates σ, raising the threshold, hiding
the next. `TestRobustDetectorResistsMasking` demonstrates this concretely: one
contaminating point drives the classic z-score of a genuine 2× spike down to **−0.04**.

MAD has breakdown point 1/2 — the maximum attainable. Scaling by
`1/Φ⁻¹(0.75) = 1.4826` makes it consistent for σ under normality.

**Why 3σ ≠ 99.7% here.** That figure requires Gaussianity. Distribution-free, Chebyshev
gives only `P(|X−μ| ≥ 3σ) ≤ 1/9 = 11.1%` — a factor of ~37 weaker than the number in the
code comment. Daily record counts are right-skewed and bounded below at zero, so the true
tail mass exceeds 0.3% substantially.

**Next upgrade.** Rousseeuw-Croux `Sn` and `Qn` estimators retain the 50% breakdown point
but achieve ~58% and ~82% Gaussian efficiency respectively, versus MAD's 37%. `Qn` is
strictly better than MAD for this use case and computable in O(n log n). Worth doing.

**Also worth reading:** **Liu, Ting & Zhou (2008),** "Isolation Forest," ICDM — the method
the implementation plan names. Note its actual property: it isolates anomalies in
*expected path length* O(log n) and does not estimate a density. It is a fine multivariate
screen but it gives you **no calibrated probability**, so it cannot produce `P(breach)`.
Pair it with conformal calibration (§1) if you want both.

**Implemented:** `gateway/robust_anomaly.go`.

---

## 7. The audit ledger — you claimed Merkle, you built a hash chain

**Merkle (1988),** "A Digital Signature Based on a Conventional Encryption Function,"
CRYPTO '87.
**Crosby & Wallach (2009),** "Efficient Data Structures for Tamper-Evident Logging,"
*18th USENIX Security Symposium*, 317–334. — **this is your paper.**
**Laurie, Langley & Kasper, RFC 6962** (Certificate Transparency); superseded by
**RFC 9162** (CT v2).

**The gap.** A linear hash chain requires **O(n)** work to prove anything and offers no
proof of consistency between two published states. Crosby & Wallach's *history tree* gives:
<cite index="118-1">O(log n) membership and consistency proofs</cite>. <cite index="112-1">A tree-based data structure that can generate tamper-evident proofs with logarithmic size and space, improving over previous linear constructions, and allowing large-scale log servers to selectively delete old events, in an agreed-upon fashion, while generating efficient proofs that no inappropriate events were deleted.</cite>

**Two operations you need and currently cannot perform:**
1. **Membership proof** — "prove event #4,812,003 is in the log" using ~23 hashes for a
   log of 10M events, not 4.8M.
2. **Consistency proof** — "prove the root you published to the auditor last quarter is a
   prefix of the root you are publishing now." A hash chain cannot do this at all. For
   SEC 17a-4(f) non-rewriteable/non-erasable evidence, this is the operation that matters.

**Threat model, stated properly.** <cite index="115-1">Forward integrity: events prior to Byzantine failure are tamper-evident. You don't know when the logger becomes evil. Strong insider attacks: malicious administrator.</cite> That is precisely your threat model — the concern is not an external attacker, it is a privileged operator rewriting a release decision after a loss. Note the paper's own caveat, which you must reproduce honestly: **tamper-evidence requires auditing.** A log nobody challenges proves nothing. So the product needs a scheduled auditor that fetches and verifies published roots, or the whole construction is theatre.

**Reference implementation:** github.com/scrosby/fastsig (`edu.rice.historytree`), and
Google's Trillian is a production-grade CT-style verifiable log you could adopt outright
rather than build.

**Partially implemented:** verification now recomputes each hash
(`gateway/ledger.go`), which closes the content-tampering hole. The tree structure and
consistency proofs are **not** implemented — do not claim "Merkle" until they are.

---

## 8. Tokenisation and the FPE label

The policy struct advertised `FPE_AES256` while the code did substring masking plus HMAC.
Beyond the mislabelling, there is a substantive result you need before implementing real
FPE:

**NIST SP 800-38G** (2016) specifies FF1 and FF3.
**Durak & Vaudenay (2017),** "Breaking the FF3 Format-Preserving Encryption Standard Over
Small Domains," CRYPTO '17, eprint 2017/521.
**Hoang, Tessaro & Trieu (2018),** "The Curse of Small Domains," eprint 2018/556.
**Hoang, Miller & Trieu (2019),** "Attacks Only Get Better: How to Break FF3 on Large
Domains," eprint 2019/244.

<cite index="146-1">NIST has concluded that FF3 is no longer suitable as a general-purpose FPE method.</cite>
<cite index="147-1">In SP 800-38G Revision 1, the tweak parameter is reduced to 56 bits; the revised FF3 is named FF3-1. The domain size for both FF1 and FF3 in SP 800-38G was required to be at least one hundred and recommended to be at least one million. In response to the analysis of Hoang, Tessaro, and Trieu, the recommendation was strengthened to a requirement: the minimum domain size for FF1 and FF3-1 in Draft SP 800-38G Revision 1 is one million.</cite>

**The direct consequence for this product.** <cite index="149-1">DV's attack can recover the entire codebook of FF3 using O(N⁵) expected time for domain Z_N × Z_N; the improved attack recovers encrypted 6-digit PINs in about 2³⁰ operations and encrypted SSNs in about 2⁵⁰.</cite>
Financial identifiers are exactly the small domains under attack: ABA routing numbers have
~10⁴–10⁵ *assigned* values, well under NIST's one-million floor. **FPE is the wrong tool
for routing numbers.** Use a random-token vault (token ↔ ciphertext lookup, which is what
the remediated `vault.go` now does) and reserve FPE for genuinely large-domain fields where
legacy schema width constraints force it.

**And on "FIPS 140":** it is a validation of a cryptographic *module* by an accredited
CMVP laboratory, with a certificate number. It cannot be asserted by naming a constant
`SENTINEL_FIPS140_HMAC_SECRET`. Claiming it without a certificate in a bank sales cycle is
a serious problem. Removed.

---

## 9. Dependency cascades — the multi-hop failure the pitch describes

The research report's strongest narrative ("one late custodian file → NAV cannot be struck
→ regulatory cutoff missed") is a **cascade**, and nothing in the codebase models
dependency structure.

**Hawkes (1971),** "Spectra of Some Self-Exciting and Mutually Exciting Point Processes,"
*Biometrika* 58(1) — conditional intensity
`λ(t) = μ + Σ_{tᵢ<t} α e^{−β(t−tᵢ)}`, where each event raises the probability of
subsequent events. This is the natural formalism for "late file at hop 1 raises hazard of
lateness at hops 2..k."
**Ogata (1978, 1988)** — MLE estimation and residual-based goodness of fit for Hawkes.
**Cox (1972),** "Regression Models and Life-Tables," *JRSS-B* 34(2) — proportional hazards,
`λ(t|x) = λ₀(t)exp(βᵀx)`, for time-to-arrival with censoring. Files that have not arrived
yet are **right-censored observations**, not missing data; treating them as missing biases
every estimate you produce. This is the single most common modelling error in this domain.

**Practical note.** Start with Cox. It is well-understood, has stable software, handles
censoring correctly, and gives interpretable hazard ratios per counterparty that an
operations manager can read. Add Hawkes only when you can actually observe the
multi-hop graph.

---

## 10. What has no proof, and should not be claimed

Stated plainly so it does not creep back into the README:

| Claim | Status |
|---|---|
| RTO / RPO figures | No measurement exists. Requires killing a real primary and timing a real promotion. |
| Throughput multiples vs "industry MFT" | No like-for-like baseline exists. An in-memory scan is not comparable to an end-to-end MFT pipeline doing disk I/O, decryption, and DB writes. |
| "85–95% alert noise reduction" | Unfalsifiable without a labelled incident corpus and a measured baseline. See §4 — the arithmetic currently runs the other way. |
| "MTTD < 30 seconds" | Meaningless without specifying detection *of what*, at what false-alarm rate. Detection delay and false-alarm rate trade off against each other (§2); quoting one without the other is not a result. |
| LLM triage accuracy | No eval existed. The harness could not fail. Now it can (`ai-tier/evals/runner.py`), but you have no labelled dataset yet, so there is still no number to quote. |
| "SIMD-accelerated" | No SIMD in the codebase. Go's stdlib SHA-256 uses SHA-NI on amd64; that is the stdlib's doing, not yours. |

---

## 11. Reading order

If you read four things, read these, in this order:

1. **Angelopoulos & Bates (2023),** "Conformal Prediction: A Gentle Introduction,"
   *Foundations and Trends in ML* 16(4). The most practical on-ramp to §1.
2. **Crosby & Wallach (2009),** USENIX Security. Short, concrete, and directly replaces
   what you built in `ledger.go`.
3. **Ramdas, Grünwald, Vovk & Shafer (2023),** *Statistical Science* 38(4). The continuous-
   monitoring problem in §3 is a real bug in the architecture, not a refinement.
4. **Veeravalli & Banerjee (2013),** "Quickest Change Detection," arXiv:1210.5552. A
   readable survey of §2 with the Lorden/Moustakides/Pollak results laid out together.

---

## 12. Where this changes the product argument

The verification memo concluded that "no tool detects the absence of an event" is false —
IBM Sterling Control Center ships exactly that feature. The defensible reframing is not
*"nobody can do this"* but:

> *Incumbent tools require an operator to hand-configure an expected arrival window per
> feed. Nobody does that across thousands of feeds, so the capability exists and sits
> unused. We auto-learn the window per counterparty and attach a calibrated probability
> to the prediction, with published coverage.*

That claim is narrower, true, and — critically — **provable with the results in §1, §2 and
§3**. It is also directly testable against a customer in two weeks: ask an MFT ops team
whether their SLC rules are configured, and if not, why not.

The mathematics in this document is what turns "we have a dashboard" into "we have a
calibrated forecast with a stated error guarantee." That is the difference between a
feature and a product.
