package main

import (
	"bytes"
	"sort"
	"strings"
	"testing"

	"sentinel-gateway/internal/nacha"
)

// The generator's presets must demonstrate what they claim.
//
// The previous implementation padded short records but never truncated long
// ones, so five of its six presets emitted records of the wrong width while
// their descriptions claimed otherwise -- and the preset advertising a
// corrupted entry hash was quarantined for malformed record widths instead.
// Nothing caught it because the validator it fed did not check record length.
//
// These tests are the check that was missing.

func allPresets() []GeneratorPreset {
	return []GeneratorPreset{
		PresetBalancedPayroll, PresetUnbalancedCCD, PresetCorruptedEntryHash,
		PresetInvalidAbaRouting, PresetRecordAlignmentError, PresetMissingHeaderSequence,
	}
}

// Every record is 94 characters, except where a preset's stated defect is its
// width.
func TestGeneratedRecordsAreExactlyNinetyFourCharacters(t *testing.T) {
	for _, preset := range allPresets() {
		if preset == PresetRecordAlignmentError {
			continue // its defect is the width itself
		}
		scenario := GenerateNachaScenario(preset)
		for i, line := range strings.Split(strings.TrimRight(scenario.Content, "\n"), "\n") {
			if len(line) != nacha.RecordLength {
				t.Errorf("%s record %d is %d characters, want %d",
					preset, i+1, len(line), nacha.RecordLength)
			}
		}
	}
}

// A preset that claims to be valid must validate, and a preset that claims a
// defect must produce exactly that finding and no other blocking one.
func TestEachPresetProducesExactlyTheFindingsItClaims(t *testing.T) {
	for _, preset := range allPresets() {
		t.Run(string(preset), func(t *testing.T) {
			scenario := GenerateNachaScenario(preset)
			res, err := nacha.Validate(bytes.NewReader([]byte(scenario.Content)))
			if err != nil {
				t.Fatalf("validate: %v", err)
			}

			got := map[string]bool{}
			for _, f := range res.Findings {
				if f.Blocking() {
					got[f.RuleID] = true
				}
			}
			want := map[string]bool{}
			for _, id := range scenario.ExpectedRuleIDs {
				want[id] = true
			}

			for id := range want {
				if !got[id] {
					t.Errorf("%s claims to demonstrate %s but the validator did not raise it. raised: %v",
						preset, id, sortedKeys(got))
				}
			}
			// A preset naming no defect must raise none.
			if len(want) == 0 && len(got) > 0 {
				t.Errorf("%s claims to be valid but raised %v", preset, sortedKeys(got))
			}
			// A preset naming a defect must not raise unrelated blocking rules,
			// except where a malformed record makes downstream records
			// undecodable -- which RECORD_ALIGNMENT_ERROR's description says.
			if len(want) > 0 && preset != PresetRecordAlignmentError {
				for id := range got {
					if !want[id] {
						t.Errorf("%s raised the unrelated blocking rule %s; the fixture demonstrates more than it claims",
							preset, id)
					}
				}
			}
		})
	}
}

// The presets that claim validity must reach VALIDATED under a contract that
// does not require balance.
func TestValidPresetsAreValidated(t *testing.T) {
	for _, preset := range []GeneratorPreset{PresetBalancedPayroll, PresetUnbalancedCCD} {
		scenario := GenerateNachaScenario(preset)
		res, err := nacha.Validate(bytes.NewReader([]byte(scenario.Content)))
		if err != nil {
			t.Fatal(err)
		}
		if d := nacha.Decide(res, nacha.DefaultContract); d.Outcome != nacha.OutcomeValidated {
			t.Errorf("%s: outcome = %s, want VALIDATED. reasons: %v", preset, d.Outcome, d.Reasons)
		}
	}

	// And the unbalanced one is quarantined under a contract that requires
	// balance, which is the point of that fixture.
	scenario := GenerateNachaScenario(PresetUnbalancedCCD)
	res, _ := nacha.Validate(bytes.NewReader([]byte(scenario.Content)))
	strict := nacha.FeedContract{ID: "OFFSET", Version: "1.0", RequireBalanced: true}
	if d := nacha.Decide(res, strict); d.Outcome != nacha.OutcomeQuarantined {
		t.Error("the unbalanced fixture was validated under a contract requiring balance")
	}
}

// Every preset must be reproducible: two calls differ only in the timestamp
// fields, so a fixture used in a demo is the same fixture next time.
func TestGeneratedFixturesAreStructurallyStable(t *testing.T) {
	for _, preset := range allPresets() {
		a := GenerateNachaScenario(preset)
		b := GenerateNachaScenario(preset)
		if len(a.Content) != len(b.Content) {
			t.Errorf("%s produced fixtures of %d and %d bytes", preset, len(a.Content), len(b.Content))
		}
		if a.Description != b.Description {
			t.Errorf("%s changed its description between calls", preset)
		}
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
