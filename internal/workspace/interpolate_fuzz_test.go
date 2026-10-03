package workspace

import (
	"strings"
	"testing"
)

// FuzzInterpolateString verifies the interpolator never panics or returns
// unexpected extra braces on arbitrary (key, value, input) combinations.
func FuzzInterpolateString(f *testing.F) {
	interp := NewInterpolator()

	// Seed corpus: representative real-world patterns
	f.Add("TOKEN", "abc123", "Bearer {{TOKEN}}")
	f.Add("HOST", "localhost", "http://{{HOST}}/api")
	f.Add("A", "{{B}}", "start {{A}} end")  // circular via B undefined
	f.Add("", "", "{{MISSING}}")
	f.Add("X", "{{X}}", "{{X}}")            // self-referential
	f.Add("K", "v", "{{K}}{{K}}{{K}}")
	f.Add("PORT", "8080", "{{HOST}}:{{PORT}}")
	f.Add("$uuid", "", "id={{$uuid}}")

	f.Fuzz(func(t *testing.T, key, value, input string) {
		vars := map[string]string{}
		if key != "" {
			vars[key] = value
		}
		// Must not panic
		result := interp.InterpolateString(input, vars)

		// Invariant: unresolved placeholders must remain as-is (still contain {{)
		// Resolved placeholders must not leave dangling {{...}} with known keys
		if key != "" && strings.Contains(input, "{{"+key+"}}") {
			// If fully resolvable (value has no nested vars) result should not
			// contain the original placeholder unless value itself reintroduced it
			if !strings.Contains(value, "{{") {
				if strings.Contains(result, "{{"+key+"}}") {
					t.Errorf("placeholder {{%s}} not resolved in %q → %q", key, input, result)
				}
			}
		}
	})
}

// FuzzInterpolateBroken verifies malformed {{-sequences never panic.
func FuzzInterpolateBroken(f *testing.F) {
	interp := NewInterpolator()

	f.Add("{{")
	f.Add("}}")
	f.Add("{{}}")
	f.Add("{{ }}")
	f.Add("{{.invalid!}}")
	f.Add("{ { VAR } }")
	f.Add("{{VAR")
	f.Add("VAR}}")

	f.Fuzz(func(t *testing.T, input string) {
		// Must not panic regardless of input
		_ = interp.InterpolateString(input, nil)
	})
}

// FuzzInterpolateAtoB verifies A→B→A circular chains terminate and never panic.
func FuzzInterpolateAtoB(f *testing.F) {
	interp := NewInterpolator()

	f.Add("X", "Y", "Z")
	f.Add("api", "dev", "prod")

	f.Fuzz(func(t *testing.T, a, b, c string) {
		// Construct A→B→C→A circular chain
		vars := map[string]string{
			"VA": "{{VB}}",
			"VB": "{{VC}}",
			"VC": "{{VA}}", // cycle
		}
		_ = a
		_ = b
		_ = c
		result := interp.InterpolateString("{{VA}}", vars)
		// Must still contain {{ (unresolvable circular), must not hang
		if !strings.Contains(result, "{{") {
			t.Errorf("expected unresolved circular to keep braces, got: %q", result)
		}
	})
}
