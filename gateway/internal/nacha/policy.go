package nacha

import (
	"fmt"
	"sort"
	"strings"
)

// PolicyVersion identifies the rule-to-decision mapping that produced a
// decision.
//
// It is recorded on every decision so a release approved last quarter can be
// explained by the policy in force then, not by the policy in force now. A
// decision without a policy version is an opinion.
const PolicyVersion = "release-policy/1.0.0"

// Outcome is what the policy decided.
type Outcome string

const (
	// OutcomeValidated means the artifact may proceed toward approval. It is
	// not a release: release requires approval and dual control, which live in
	// internal/domain.
	OutcomeValidated Outcome = "VALIDATED"

	// OutcomeQuarantined means the artifact fails closed and cannot proceed.
	OutcomeQuarantined Outcome = "QUARANTINED"
)

// FeedContract carries the counterparty-specific expectations a file must meet.
//
// The balance requirement lives here rather than in the validator because it is
// a property of the arrangement, not of the format. A file where debits equal
// credits is *balanced*; whether balanced is *required* depends on what the
// originator agreed to send. The previous implementation returned
// `isBalanced: debits == credits` from every validation and treated it as a
// correctness signal, which is wrong in both directions: an offsetting file
// that must balance and does not is a defect, and a credit-only payroll file
// that never balances is entirely correct.
type FeedContract struct {
	// ID and Version identify the contract that produced a decision.
	ID      string
	Version string

	// RequireBalanced means debits must equal credits across the file.
	RequireBalanced bool

	// MaxFileAmountMinor bounds the total value of a single file, in minor
	// units. Zero means the contract sets no limit.
	//
	// This is a contract term, not a Nacha rule: it is what this counterparty
	// agreed it would send, which is checkable without a licensed rule source.
	MaxFileAmountMinor int64

	// AllowedSECCodes, when non-empty, restricts the batch SEC codes this feed
	// may carry. Also a contract term rather than a rule-set requirement --
	// whether a code is *valid* needs the Operating Rules; whether it is
	// *expected on this feed* is an agreement.
	AllowedSECCodes []string
}

// DefaultContract is used when no feed contract has been configured.
//
// It requires nothing beyond format validity and is deliberately permissive
// about balance, because assuming a balance requirement that was never agreed
// would quarantine legitimate payroll files. The absence of a contract is
// reported by Decision.ContractID being empty, so "no contract was applied" is
// visible rather than inferred.
var DefaultContract = FeedContract{
	ID:      "",
	Version: "",
}

// Decision is the versioned outcome of applying a policy to a result.
//
// It is the record that justifies a release, so it carries everything needed to
// reconstruct the reasoning: which policy, which contract, which findings, and
// what was not checked.
type Decision struct {
	Outcome       Outcome `json:"outcome"`
	PolicyVersion string  `json:"policyVersion"`

	ContractID      string `json:"contractId,omitempty"`
	ContractVersion string `json:"contractVersion,omitempty"`

	// Reasons are the human-readable causes of a quarantine, most significant
	// first. Empty for a validated artifact.
	Reasons []string `json:"reasons,omitempty"`

	// BlockingRuleIDs lists the rules that caused the quarantine, so a decision
	// can be queried without parsing prose.
	BlockingRuleIDs []string `json:"blockingRuleIds,omitempty"`

	// NotCheckedRuleIDs lists what the validator could not evaluate. A
	// VALIDATED outcome alongside a non-empty list means "valid as far as this
	// system can tell", and the list is what makes that qualification legible.
	NotCheckedRuleIDs []string `json:"notCheckedRuleIds,omitempty"`
}

// Quarantined reports whether the artifact fails closed.
func (d Decision) Quarantined() bool { return d.Outcome == OutcomeQuarantined }

// Decide applies the release policy to a validation result.
//
// The structure is deliberate: it starts from quarantine and only reaches
// VALIDATED by falling through every check. The defect this replaces did the
// reverse -- it initialised the status to RELEASED and downgraded on a positive
// finding, so any condition nobody thought to check released the file.
func Decide(result *Result, contract FeedContract) Decision {
	decision := Decision{
		Outcome:         OutcomeQuarantined,
		PolicyVersion:   PolicyVersion,
		ContractID:      contract.ID,
		ContractVersion: contract.Version,
	}
	for _, r := range result.NotChecked {
		decision.NotCheckedRuleIDs = append(decision.NotCheckedRuleIDs, r.ID)
	}

	var reasons []string
	var blocking []string

	// 1. The parser must have succeeded. An unreadable file is quarantined
	// whatever else is true of it.
	if !result.ParserOK {
		reasons = append(reasons, "the artifact could not be parsed as an ACH file")
	}

	// 2. Zero records. This is the empty-file case stated explicitly rather
	// than left to emerge from the absence of findings.
	if result.RecordsParsed == 0 {
		reasons = append(reasons, "the artifact contains no records")
	}
	if result.EntriesParsed == 0 {
		reasons = append(reasons, "the artifact contains no entry detail records, so it instructs no payment")
	}

	// 3. Any blocking finding.
	seen := map[string]bool{}
	for _, f := range result.Findings {
		if !f.Blocking() {
			continue
		}
		if !seen[f.RuleID] {
			seen[f.RuleID] = true
			blocking = append(blocking, f.RuleID)
			reasons = append(reasons, fmt.Sprintf("%s (record %d): %s", f.RuleID, f.RecordNumber, f.Description))
		}
	}

	// 4. Contract terms.
	if contract.RequireBalanced && result.TotalDebitsMinor != result.TotalCreditsMinor {
		reasons = append(reasons, fmt.Sprintf(
			"the feed contract requires a balanced file; debits %s and credits %s differ",
			Amount(result.TotalDebitsMinor), Amount(result.TotalCreditsMinor)))
		blocking = append(blocking, "CONTRACT.BALANCE_REQUIRED")
	}
	if contract.MaxFileAmountMinor > 0 {
		total := result.TotalDebitsMinor + result.TotalCreditsMinor
		if total > contract.MaxFileAmountMinor {
			reasons = append(reasons, fmt.Sprintf(
				"the file total %s exceeds the contract limit %s",
				Amount(total), Amount(contract.MaxFileAmountMinor)))
			blocking = append(blocking, "CONTRACT.FILE_AMOUNT_LIMIT")
		}
	}

	sort.Strings(blocking)
	decision.BlockingRuleIDs = blocking
	decision.Reasons = reasons

	if len(reasons) == 0 {
		decision.Outcome = OutcomeValidated
	}
	return decision
}

// Summary renders a decision for an operator in one line, with no raw content.
func (d Decision) Summary() string {
	if d.Outcome == OutcomeValidated {
		if len(d.NotCheckedRuleIDs) > 0 {
			return fmt.Sprintf("VALIDATED under %s; %d rule(s) not checked for lack of an authoritative source",
				d.PolicyVersion, len(d.NotCheckedRuleIDs))
		}
		return fmt.Sprintf("VALIDATED under %s", d.PolicyVersion)
	}
	return fmt.Sprintf("QUARANTINED under %s: %s", d.PolicyVersion, strings.Join(d.Reasons, "; "))
}
