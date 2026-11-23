package tariffshift

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

const goldenPath = "testdata/canada_evaluate.golden"

// goldenInputs is a fixed sample of questions for one row: foreign and
// domestic materials, missing and short codes, every optional fact unknown,
// true and false.
func goldenInputs(code string) []Input {
	tr, fa := true, false
	var out []Input
	mats := [][]Material{
		{{Name: "a", HTS: "5205.11", Origin: "China"}},
		{{Name: "a", HTS: "", Origin: "China"}},
		{{Name: "a", HTS: "8473", Origin: "China"}, {Name: "b", HTS: "8708.99", Origin: "Mexico"}},
		{{Name: "a", HTS: code, Origin: "China"}},
		{{Name: "a", HTS: "7318.15", Origin: "China", ValueShare: ptrf(0.05)}, {Name: "b", HTS: "3926.90", Origin: "Mexico"}},
		{{Name: "a", NotIn: "8473", Origin: "China"}},
		{{Name: "a", HTS: "8501.10", Origin: "Mexico"}},
		{{Name: "a", HTS: "7318.15", Origin: ""}, {Name: "b", HTS: "7318", Origin: "Korea"}},
	}
	for _, m := range mats {
		for _, f := range [][3]*bool{{nil, nil, nil}, {&tr, &fa, &tr}, {&fa, &tr, &fa}} {
			out = append(out, Input{GoodHTS: code, Country: "Mexico", Materials: m, SimpleAssembly: f[0], GRI2a: f[1], BendingToShape: f[2]})
		}
	}
	return out
}

func ptrf(f float64) *float64 { return &f }

// TestCanadaEvaluateGolden pins Evaluate's bytes (Result JSON) for a sample
// over every Canadian row, so the guidance layer cannot change a decided
// result, a legacy reason or a material's needs. To refresh after a
// deliberate change to the rules: UPDATE_GOLDEN=1 go test ./internal/tariffshift -run Golden.
func TestCanadaEvaluateGolden(t *testing.T) {
	r := mustCanada(t)
	var lines []string
	for _, rule := range r.Table().Rules {
		h := sha256.New()
		for _, code := range []string{rule.Applies.Lo, rule.Applies.Hi} {
			for _, in := range goldenInputs(code) {
				b, err := json.Marshal(r.Evaluate(in))
				if err != nil {
					t.Fatal(err)
				}
				h.Write(b)
				h.Write([]byte{'\n'})
			}
		}
		lines = append(lines, fmt.Sprintf("%s %s", hex.EncodeToString(h.Sum(nil))[:16], rule.ID))
	}
	got := strings.Join(lines, "\n") + "\n"
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		gl, wl := strings.Split(got, "\n"), strings.Split(string(want), "\n")
		for i := range gl {
			if i >= len(wl) || gl[i] != wl[i] {
				t.Fatalf("Evaluate output changed for row %q (golden %q)", gl[i], wl[min(i, len(wl)-1)])
			}
		}
		t.Fatalf("golden length differs")
	}
}
