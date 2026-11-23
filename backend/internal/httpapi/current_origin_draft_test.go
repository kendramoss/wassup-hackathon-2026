package httpapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

func currentOriginBody(t *testing.T, co any) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(heatGunBody(t)), &m); err != nil {
		t.Fatal(err)
	}
	m["current_origin"] = co
	b, _ := json.Marshal(m)
	return string(b)
}

// #720: a country-only current_origin reads "origin China", not "origin China ()",
// in the Markdown draft and in sections 3 and 8 of the 177.2 draft. A ruling keeps
// the existing wording; no review note enters the letter.
func TestCurrentOriginCountryOnlyDraftHasNoEmptyParentheses(t *testing.T) {
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.ExportCFR177 = true })
	for _, tc := range []struct {
		name string
		co   map[string]any
		want map[string][]string
		not  []string
	}{
		{"country only", map[string]any{"country": "China"}, map[string][]string{
			"markdown": {"origin China"},
			"cfr177": {"CBP has ruled on this production setup: origin China.",
				"CBP's ruling on the current setup, origin China, is described in section 3."},
		}, []string{"China ()", "()"}},
		{"country and empty ruling", map[string]any{"country": "China", "ruling": ""}, map[string][]string{
			"markdown": {"origin China"},
			"cfr177":   {"CBP has ruled on this production setup: origin China."},
		}, []string{"China ()", "()"}},
		{"control: country and ruling", map[string]any{"country": "China", "ruling": "N328954"}, map[string][]string{
			"markdown": {"origin China (CBP ruling N328954, "},
			"cfr177": {"CBP has ruled on this production setup: origin China (CBP ruling N328954, ",
				"CBP's ruling on the current setup, origin China (CBP ruling N328954, "},
		}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, c := hs.do(t, "POST", "/v1/cases/predict", currentOriginBody(t, tc.co))
			if code != 201 {
				t.Fatalf("predict %d %v", code, c)
			}
			id := c["id"].(string)
			for format, wants := range tc.want {
				code, out := hs.do(t, "POST", "/v1/cases/"+id+"/export", `{"format":"`+format+`","preview":true}`)
				if code != 200 {
					t.Fatalf("%s export %d %v", format, code, out)
				}
				md, _ := out["content"].(string)
				for _, w := range wants {
					if !strings.Contains(md, w) {
						t.Errorf("%s missing %q in:\n%s", format, w, md)
					}
				}
				for _, n := range tc.not {
					if strings.Contains(md, n) {
						t.Errorf("%s contains %q", format, n)
					}
				}
				if strings.Contains(md, "Draft for review") {
					t.Errorf("%s: review note in the letter", format)
				}
			}
		})
	}
}

// The Ask agent's case text drops the empty parentheses the same way.
func TestAgentCaseTextCurrentOriginCountryOnly(t *testing.T) {
	for _, tc := range []struct {
		o    planner.KnownOrigin
		want string
	}{
		{planner.KnownOrigin{Country: "China"}, "CBP's ruling on the current setup: China.\n"},
		{planner.KnownOrigin{Country: "China", Ruling: "N328954"}, "CBP's ruling on the current setup: China (N328954).\n"},
	} {
		o := tc.o
		if got := agentCaseText(store.Case{ID: "c1", Date: "2026-01-14", CurrentOrigin: &o}); !strings.Contains(got, tc.want) {
			t.Errorf("%+v: %q", tc.o, got)
		}
	}
}

func TestCurrentOriginFallback(t *testing.T) {
	for _, tc := range []struct {
		o    planner.KnownOrigin
		want string
	}{
		{planner.KnownOrigin{Country: "China"}, "China"},
		{planner.KnownOrigin{Country: " China "}, "China"},
		{planner.KnownOrigin{Country: "China", Ruling: "N328954"}, "China (N328954)"},
		{planner.KnownOrigin{Ruling: "N328954"}, "(N328954)"},
	} {
		if got := currentOriginFallback(tc.o); got != tc.want {
			t.Errorf("%+v = %q, want %q", tc.o, got, tc.want)
		}
	}
}
