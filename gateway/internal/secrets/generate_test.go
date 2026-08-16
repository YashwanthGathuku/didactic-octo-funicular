package secrets

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Generated secrets must not collide. A birthday collision at 256 bits is not
// reachable, so any repeat here means the generator is not random at all --
// which is precisely the failure mode of the time-based generator this replaces.
func TestGenerateProducesDistinctValues(t *testing.T) {
	const n = 4096
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		v, err := Generate()
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		raw := v.Expose()
		if _, dup := seen[raw]; dup {
			t.Fatalf("Generate returned a duplicate after %d values; the source is not a CSPRNG", i)
		}
		seen[raw] = struct{}{}
	}
}

// The archived generator derived secrets from the wall clock, so two secrets
// created in the same moment shared a prefix and the whole value was a function
// of a guessable number. Consecutive values here must share no meaningful
// prefix.
func TestGenerateIsNotTimeDerived(t *testing.T) {
	var prev string
	for i := 0; i < 64; i++ {
		v, err := Generate()
		if err != nil {
			t.Fatal(err)
		}
		raw := v.Expose()
		if prev != "" {
			shared := 0
			for shared < len(prev) && shared < len(raw) && prev[shared] == raw[shared] {
				shared++
			}
			if shared > 4 {
				t.Errorf("consecutive secrets share a %d-character prefix; the generator looks like a counter or a clock", shared)
			}
		}
		prev = raw
	}
}

func TestGenerateMeetsTheMinimumLength(t *testing.T) {
	v, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Expose()) < MinSecretLength {
		t.Errorf("a generated secret is %d characters, below MinSecretLength=%d", len(v.Expose()), MinSecretLength)
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(v.Expose()) {
		t.Errorf("a generated secret is not URL-safe: %q", v.Expose())
	}
}

// A generated secret redacts like any other. This exists because it would be
// easy for a future Generate to return a bare string for convenience.
func TestGeneratedSecretsRedact(t *testing.T) {
	v := MustGenerate()
	if got := v.String(); got != Placeholder {
		t.Errorf("a generated secret renders as %q, not the placeholder", got)
	}
}

// The generator must not reach a non-cryptographic source. Reviewing this by
// eye works once; asserting it keeps working.
//
// The check parses each file rather than searching its text, because the
// comments in this package name the weak sources deliberately in order to
// explain why they are wrong. A textual scan flags its own documentation.
func TestGeneratorDoesNotUseWeakEntropySources(t *testing.T) {
	weakPackages := map[string]bool{"math/rand": true, "math/rand/v2": true}
	weakCalls := map[string]bool{"UnixNano": true, "UnixMilli": true, "UnixMicro": true}

	for _, name := range []string{"generate.go", "value.go", "store.go", "memory.go", "sqlstore.go", "sealer.go"} {
		path := filepath.Join(".", name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s is missing; this list must track the package's files", name)
			continue
		}

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0) // comments discarded
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}

		for _, imp := range file.Imports {
			pkg := strings.Trim(imp.Path.Value, `"`)
			if weakPackages[pkg] {
				t.Errorf("%s imports %s; credential material must come only from crypto/rand", name, pkg)
			}
		}

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if weakCalls[sel.Sel.Name] {
				t.Errorf("%s calls %s at %s; a clock is not an entropy source",
					name, sel.Sel.Name, fset.Position(call.Pos()))
			}
			return true
		})
	}
}
