package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
)

// traceEngine answers with a chosen prompt version and origin.
type traceEngine struct {
	version, origin string
	err             error
}

func (e traceEngine) Predict(context.Context, domain.Case) (engine.Result, error) {
	if e.err != nil {
		return engine.Result{}, e.err
	}
	return engine.Result{Prediction: domain.Prediction{Origin: e.origin, Confidence: 0.97, KeyFactors: []string{"final production"}, CitedRulings: []string{"N350001"},
		Trace: domain.Trace{ModelID: "gemini-4-argon", PromptVersion: e.version}}}, nil
}

const exportFacts = `{"date":"2026-03-01","facts":"Brackets are stamped in Mexico from steel coil made in China."}`

// regimeHarness routes every case to routerRegime. The general engine and the
// selected specialist answer with the given traces.
func regimeHarness(t *testing.T, routerRegime string, general, special traceEngine) harness {
	t.Helper()
	return newHarnessWith(t, routerRegime, func(d *Deps) {
		d.ExportCFR177 = true
		d.Engine = general
		switch routerRegime {
		case router.RegimeTextile:
			d.Textile = special
		case router.RegimeUSMCA:
			d.USMCA = special
		case router.RegimePharma:
			d.Pharma = special
		}
	})
}

type regimeWant struct {
	has, not []string
	note     string // substring of a note beside the letter ("" = no regime note)
}

var (
	wantGeneral = regimeWant{has: []string{"substantial"}, not: []string{"102.21", "102.11", "Part 102 rules", "active pharmaceutical ingredient"}}
	wantTextile = regimeWant{has: []string{"19 C.F.R. § 102.21"}, not: []string{"134.1(b)", "substantial transformation", "substantial-transformation", "102.11"}, note: "Asked under 19 C.F.R. § 102.21"}
	wantUSMCA   = regimeWant{has: []string{"19 C.F.R. § 102.11", "§ 102.20", "does not ask CBP to decide the country of origin for Section 301"}, not: []string{"134.1(b)", "substantial transformation", "substantial-transformation", "102.21", "preference"}, note: "Asked under the Part 102 rules"}
	wantPharma  = regimeWant{has: []string{"active pharmaceutical ingredient (API)"}, not: []string{"102.21", "102.11", "Part 102"}, note: "Pharmaceutical (active ingredient) regime"}
)

// The handler's letter follows the regime that produced the stored
// prediction, in both formats; preview and recorded export return the same
// raw content.
func TestExportRegimeMatrixThroughHandler(t *testing.T) {
	gen := traceEngine{version: engine.PromptVersion, origin: "China"}
	for _, tc := range []struct {
		name         string
		routerRegime string
		special      traceEngine
		want         regimeWant
		wantNotes    []string // substrings, each must appear in some note
		notWantNotes []string
	}{
		{"general", "substantial_transformation", traceEngine{}, wantGeneral, nil, []string{"routed to the", "not recorded"}},
		{"textile", router.RegimeTextile, traceEngine{version: engine.TextileVersion, origin: "Vietnam"}, wantTextile, nil, []string{"routed to the", "not recorded"}},
		{"textile v2", router.RegimeTextile, traceEngine{version: engine.TextileVersion2 + "+rules", origin: "Vietnam"}, wantTextile, nil, []string{"not recorded"}},
		{"usmca", router.RegimeUSMCA, traceEngine{version: engine.USMCAVersion, origin: "Mexico"}, wantUSMCA, nil, []string{"routed to the", "not recorded"}},
		{"pharma", router.RegimePharma, traceEngine{version: engine.PharmaVersion, origin: "India"}, wantPharma, nil, []string{"routed to the", "not recorded"}},
		// Specialist labelled, specialist failed, general answered: general draft, flagged.
		{"textile failed, provisional general", router.RegimeTextile, traceEngine{err: errors.New("down")}, wantGeneral, []string{"routed to the textile_102_21 regime", "not a textile_102_21 determination"}, nil},
		{"usmca failed, provisional general", router.RegimeUSMCA, traceEngine{err: errors.New("down")}, wantGeneral, []string{"routed to the usmca_part102 regime", "not a usmca_part102 determination"}, nil},
		{"pharma failed, provisional general", router.RegimePharma, traceEngine{err: errors.New("down")}, wantGeneral, []string{"routed to the pharma_api regime"}, nil},
		// Unknown traces never confirm a specialist or general, whatever the router said.
		{"unknown version under a textile label", router.RegimeTextile, traceEngine{version: "mystery-v9", origin: "Vietnam"}, wantGeneral, []string{"not recorded"}, []string{"routed to the"}},
		{"empty version under a usmca label", router.RegimeUSMCA, traceEngine{version: "", origin: "Mexico"}, wantGeneral, []string{"not recorded"}, nil},
		{"unknown version, general label", "substantial_transformation", traceEngine{}, wantGeneral, nil, nil}, // placeholder replaced below
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := gen
			if tc.name == "unknown version, general label" {
				g = traceEngine{version: "mystery-v9", origin: "China"}
				tc.wantNotes = []string{"not recorded"}
			}
			hs := regimeHarness(t, tc.routerRegime, g, tc.special)
			code, c := hs.do(t, "POST", "/v1/cases/predict", exportFacts)
			if code != http.StatusCreated {
				t.Fatalf("predict = %d %v", code, c)
			}
			id := c["id"].(string)
			for _, format := range []string{"markdown", "cfr177"} {
				pcode, prev := hs.do(t, "POST", "/v1/cases/"+id+"/export", `{"format":"`+format+`","preview":true}`)
				if pcode != 200 {
					t.Fatalf("%s preview = %d %v", format, pcode, prev)
				}
				letter := prev["content"].(string)
				for _, w := range tc.want.has {
					if !strings.Contains(letter, w) {
						t.Errorf("%s letter missing %q", format, w)
					}
				}
				for _, a := range tc.want.not {
					if strings.Contains(letter, a) {
						t.Errorf("%s letter has %q", format, a)
					}
				}
				notes := joinNotes(prev)
				if tc.want.note != "" && !strings.Contains(notes, tc.want.note) {
					t.Errorf("%s notes missing %q: %s", format, tc.want.note, notes)
				}
				for _, w := range tc.wantNotes {
					if !strings.Contains(notes, w) {
						t.Errorf("%s notes missing %q: %s", format, w, notes)
					}
				}
				for _, a := range tc.notWantNotes {
					if strings.Contains(notes, a) {
						t.Errorf("%s notes have %q: %s", format, a, notes)
					}
				}
				// Reviewer text never enters the printable letter (#265/#266).
				for _, n := range prev["notes"].([]any) {
					if strings.Contains(letter, n.(string)) {
						t.Errorf("%s: a note is in the letter: %q", format, n)
					}
				}
				// D97: the recorded export is the previewed letter, raw.
				rcode, rec := hs.do(t, "POST", "/v1/cases/"+id+"/export", `{"format":"`+format+`"}`)
				if rcode != 200 || rec["content"] != prev["content"] || joinNotes(rec) != notes {
					t.Errorf("%s recorded export differs from preview (%d)", format, rcode)
				}
				// Deterministic.
				_, again := hs.do(t, "POST", "/v1/cases/"+id+"/export", `{"format":"`+format+`","preview":true}`)
				if again["content"] != prev["content"] {
					t.Errorf("%s preview not deterministic", format)
				}
			}
		})
	}
}

// General drafts are byte-for-byte the pre-regime text: the unchanged phrases
// of the textile CFR177 control and the general question.
func TestExportGeneralAndTextileControls(t *testing.T) {
	hs := regimeHarness(t, "substantial_transformation", traceEngine{version: engine.PromptVersion, origin: "China"}, traceEngine{})
	_, c := hs.do(t, "POST", "/v1/cases/predict", exportFacts)
	_, g := hs.do(t, "POST", "/v1/cases/"+c["id"].(string)+"/export", `{"preview":true}`)
	if !strings.Contains(g["content"].(string), "under the substantial-transformation test?") || len(g["notes"].([]any)) != 1 {
		t.Errorf("general export changed: %v", g["notes"])
	}
	ht := regimeHarness(t, router.RegimeTextile, traceEngine{version: engine.PromptVersion, origin: "China"}, traceEngine{version: engine.TextileVersion, origin: "Vietnam"})
	_, c = ht.do(t, "POST", "/v1/cases/predict", exportFacts)
	_, tx := ht.do(t, "POST", "/v1/cases/"+c["id"].(string)+"/export", `{"format":"cfr177","preview":true}`)
	if !strings.Contains(tx["content"].(string), "determined under 19 C.F.R. § 102.21") {
		t.Error("textile CFR177 control lost 102.21")
	}
}

// With no requested country the position wording keeps the regime and
// invents no country.
func TestExportRegimeWithoutRequestedCountry(t *testing.T) {
	for name, tc := range map[string]struct {
		regime string
		eng    traceEngine
		has    string
	}{
		"general": {"substantial_transformation", traceEngine{version: engine.PromptVersion}, "substantial"},
		"textile": {router.RegimeTextile, traceEngine{version: engine.TextileVersion}, "19 C.F.R. § 102.21"},
		"usmca":   {router.RegimeUSMCA, traceEngine{version: engine.USMCAVersion}, "19 C.F.R. § 102.11"},
		"pharma":  {router.RegimePharma, traceEngine{version: engine.PharmaVersion}, "active pharmaceutical ingredient (API)"},
	} {
		t.Run(name, func(t *testing.T) {
			general := traceEngine{version: engine.PromptVersion}
			hs := regimeHarness(t, tc.regime, general, tc.eng)
			code, c := hs.do(t, "POST", "/v1/cases/predict", exportFacts)
			if code != http.StatusCreated {
				t.Fatalf("predict = %d %v", code, c)
			}
			for format, want := range map[string]string{"markdown": "The requester submits that origin should be determined as follows", "cfr177": "a product of [FILL IN: country]"} {
				_, out := hs.do(t, "POST", "/v1/cases/"+c["id"].(string)+"/export", `{"format":"`+format+`","preview":true}`)
				letter, _ := out["content"].(string)
				if !strings.Contains(letter, want) || !strings.Contains(letter, tc.has) || strings.Contains(letter, "product of Mexico") {
					t.Errorf("%s letter:\n%s", format, letter)
				}
			}
		})
	}
}

// A drafted plan's own prediction decides the regime, not the case's: the
// planner's trace here is empty, so the draft is general with the unknown note.
func TestExportPlanDraftUsesPlanPredictionRegime(t *testing.T) {
	hs := regimeHarness(t, router.RegimeTextile, traceEngine{version: engine.PromptVersion, origin: "China"}, traceEngine{version: engine.TextileVersion, origin: "Vietnam"})
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	hs.do(t, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand"}`)
	code, out := hs.do(t, "POST", "/v1/cases/"+id+"/export", `{"plan_index":0,"format":"cfr177","preview":true}`)
	if code != 200 {
		t.Fatalf("plan draft = %d %v", code, out)
	}
	letter, _ := out["content"].(string)
	if strings.Contains(letter, "102.21") || !strings.Contains(joinNotes(out), "not recorded") {
		t.Errorf("plan draft notes %s", joinNotes(out))
	}
}
