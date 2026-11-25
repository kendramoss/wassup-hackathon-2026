package engine

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

func TestChinaCase(t *testing.T) {
	for _, tc := range []struct {
		name, facts string
		want        bool
	}{
		{"china and vietnam", "The motor is made in China and the fan is assembled in Vietnam.", true},
		{"chinese parts, thailand", "Chinese steel parts are welded in Thailand.", true},
		{"china only", "The lamp is made in China.", false},
		{"china and the united states", "The lamp is made in China and sold in the United States.", false},
		{"no china", "The pump is cast in India and machined in Germany.", false},
		{"chinaware is not china", "Chinaware is glazed in Japan.", false},
		{"empty", "", false},
	} {
		if got := ChinaCase(domain.Case{Facts: tc.facts}); got != tc.want {
			t.Errorf("%s: ChinaCase = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestChinaCaseRulesOnlyForGatedCases(t *testing.T) {
	if ChinaCaseRules(domain.Case{Facts: "Chinese parts are assembled in Cambodia."}) != ChinaRules {
		t.Error("a gated case did not get ChinaRules")
	}
	if ChinaCaseRules(domain.Case{Facts: "A lamp made in China."}) != "" {
		t.Error("an ungated case got ChinaRules")
	}
}

// The unseen check (eval/splits/splits_chinacheck.json, D860) is exactly the
// training rulings ChinaCase names.
func TestChinaCheckSplitMatchesGate(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	var tune, check struct {
		Train     []string `json:"train"`
		TuneCheck []string `json:"tune_check"`
	}
	for p, v := range map[string]any{"splits_tune.json": &tune, "splits_chinacheck.json": &check} {
		b, err := os.ReadFile(filepath.Join(root, "eval", "splits", p))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(b, v); err != nil {
			t.Fatal(err)
		}
	}
	var want []string
	for _, id := range append(tune.Train, tune.TuneCheck...) {
		b, err := os.ReadFile(filepath.Join(root, "backend", "data", "facts", id+".txt"))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		if ChinaCase(domain.Case{Facts: string(b)}) {
			want = append(want, id)
		}
	}
	sort.Strings(want)
	if strings.Join(want, ",") != strings.Join(check.Train, ",") {
		t.Errorf("splits_chinacheck.json has %d rulings, ChinaCase names %d", len(check.Train), len(want))
	}
}
