package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
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
	// A Part 102 case whose prediction is the general engine's: asked under
	// Part 102 for marking, the position flagged as not a Part 102
	// determination (D520).
	wantPart102Routed = regimeWant{has: []string{"19 C.F.R. § 102.11", "§ 102.20", "19 C.F.R. § 102.0", "does not ask CBP to decide the country of origin for Section 301"}, not: []string{"134.1(b)", "substantial transformation", "substantial-transformation", "102.21", "Section 301 duties?"}, note: "routed to the Part 102 marking rules"}
	wantPharma        = regimeWant{has: []string{"active pharmaceutical ingredient (API)"}, not: []string{"102.21", "102.11", "Part 102"}, note: "Pharmaceutical (active ingredient) regime"}
)

// The handler's letter follows the regime that produced the stored
// prediction, in both formats; preview and recorded export return the same
// raw content.
func TestExportRegimeMatrixThroughHandler(t *testing.T) {
	gen := traceEngine{version: engine.PromptVersion, origin: "China"}
	for _, tc := range []struct {
		name           string
		routerRegime   string
		special        traceEngine
		want           regimeWant
		wantNotes      []string // substrings, each must appear in some note
		notWantNotes   []string
		generalVersion string // overrides the general engine's trace
	}{
		{"general", "substantial_transformation", traceEngine{}, wantGeneral, nil, []string{"looks like a", "not recorded"}, ""},
		{"textile", router.RegimeTextile, traceEngine{version: engine.TextileVersion, origin: "Vietnam"}, wantTextile, nil, []string{"looks like a", "not recorded"}, ""},
		{"textile v2", router.RegimeTextile, traceEngine{version: engine.TextileVersion2 + "+rules", origin: "Vietnam"}, wantTextile, nil, []string{"not recorded"}, ""},
		{"usmca", router.RegimeUSMCA, traceEngine{version: engine.USMCAVersion, origin: "Mexico"}, wantUSMCA, nil, []string{"looks like a", "not recorded"}, ""},
		{"pharma", router.RegimePharma, traceEngine{version: engine.PharmaVersion, origin: "India"}, wantPharma, nil, []string{"looks like a", "not recorded"}, ""},
		// Specialist labelled, specialist failed, general answered: general draft, flagged.
		{"textile failed, provisional general", router.RegimeTextile, traceEngine{err: errors.New("down")}, wantGeneral, []string{"looks like a textile (19 CFR 102.21) case", "makes no textile (19 CFR 102.21) determination"}, nil, ""},
		// Part 102 failed: still asked under Part 102, as the case is routed there (D520).
		{"usmca failed, provisional general", router.RegimeUSMCA, traceEngine{err: errors.New("down")}, wantPart102Routed, []string{"made by Unde's general engine", "is not a Part 102 determination", "treat it as provisional"}, []string{"looks like a", "asks the general substantial-transformation question"}, ""},
		{"pharma failed, provisional general", router.RegimePharma, traceEngine{err: errors.New("down")}, wantGeneral, []string{"looks like a pharmaceutical (active ingredient) case"}, nil, ""},
		// Unknown traces never confirm a specialist or general, whatever the router said.
		{"unknown version under a textile label", router.RegimeTextile, traceEngine{version: "mystery-v9", origin: "Vietnam"}, wantGeneral, []string{"not recorded"}, []string{"looks like a"}, ""},
		{"empty version under a usmca label", router.RegimeUSMCA, traceEngine{version: "", origin: "Mexico"}, wantGeneral, []string{"not recorded"}, nil, ""},
		{"unknown version, general label", "substantial_transformation", traceEngine{}, wantGeneral, []string{"not recorded"}, nil, "mystery-v9"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := gen
			if tc.generalVersion != "" {
				g = traceEngine{version: tc.generalVersion, origin: "China"}
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
				if len(tc.wantNotes) > 0 {
					for _, w := range []string{"Legal framework not confirmed", "The broker or counsel must select the appropriate framework", "not validated legal advice"} {
						if !strings.Contains(notes, w) {
							t.Errorf("%s notes missing %q: %s", format, w, notes)
						}
					}
					for _, raw := range []string{"textile_102_21", "usmca_part102", "pharma_api"} {
						if strings.Contains(notes, raw) {
							t.Errorf("%s notes show raw identifier %q: %s", format, raw, notes)
						}
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

// enginePlanner is the real planner's behaviour: plan variants are predicted
// by the general engine, so each plan carries an engine-v1 trace, except on
// a request routed to Part 102 when the Part 102 engine is on (part102),
// whose plans carry a usmca-v1 trace (D540).
type enginePlanner struct {
	*fakePlanner
	part102 bool
}

func (p enginePlanner) Plan(ctx context.Context, r planner.Request) (planner.Result, error) {
	res, err := p.fakePlanner.Plan(ctx, r)
	v := engine.PromptVersion
	if p.part102 && r.Regime == router.RegimeUSMCA {
		v = engine.USMCAVersion
	}
	for i := range res.Plans {
		res.Plans[i].Prediction.Trace = domain.Trace{ModelID: "gemini-4-argon", PromptVersion: v}
	}
	return res, err
}

// A plan draft in a specialist-labelled case whose plan came from the general
// engine (textile and pharmaceutical cases; a Part 102 case searched before
// D540): the plan's own prediction decides the regime, and the note describes
// that honestly (general planner, not a claimed failure, nothing confirmed).
func TestExportPlanDraftInSpecialistCase(t *testing.T) {
	for _, regime := range []string{router.RegimeTextile, router.RegimeUSMCA, router.RegimePharma} {
		t.Run(regime, func(t *testing.T) {
			spec := traceEngine{version: map[string]string{router.RegimeTextile: engine.TextileVersion, router.RegimeUSMCA: engine.USMCAVersion, router.RegimePharma: engine.PharmaVersion}[regime], origin: "China"}
			hs := newHarnessWith(t, regime, func(d *Deps) {
				d.ExportCFR177 = true
				d.Engine = traceEngine{version: engine.PromptVersion, origin: "China"}
				d.Planner = enginePlanner{fakePlanner: &fakePlanner{}}
				d.Textile, d.USMCA, d.Pharma = spec, spec, spec
			})
			_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
			id := c["id"].(string)
			hs.do(t, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand"}`)
			for _, format := range []string{"markdown", "cfr177"} {
				code, prev := hs.do(t, "POST", "/v1/cases/"+id+"/export", `{"plan_index":0,"format":"`+format+`","preview":true}`)
				if code != 200 {
					t.Fatalf("%s plan preview = %d %v", format, code, prev)
				}
				letter, notes := prev["content"].(string), joinNotes(prev)
				// The plan's prediction (Thailand, variant facts) is the position.
				if !strings.Contains(letter, "product of Thailand") || !strings.Contains(letter, "variant facts") && format == "cfr177" {
					t.Errorf("%s letter is not the plan's position:\n%s", format, letter)
				}
				if regime == router.RegimeUSMCA {
					// A plan on a Part 102 case asks the Part 102 marking
					// question; the note says the position is the general
					// engine's (D520).
					for _, w := range []string{"under the Part 102 rules of origin", "§ 102.20", "for purposes of country of origin marking (19 C.F.R. § 102.0)?"} {
						if !strings.Contains(letter, w) {
							t.Errorf("%s Part 102 plan letter missing %q", format, w)
						}
					}
					if strings.Contains(letter, "substantial transformation") || strings.Contains(letter, "substantial-transformation") || strings.Contains(letter, "Section 301 duties?") {
						t.Errorf("%s Part 102 plan letter asks the substantial-transformation question:\n%s", format, letter)
					}
				} else if strings.Contains(letter, "102.21") || strings.Contains(letter, "Part 102 rules") || strings.Contains(letter, "active pharmaceutical ingredient") {
					t.Errorf("%s plan letter claims a specialist framework", format)
				}
				planWords := "Plan searches use the general engine except on Part 102 cases"
				if regime == router.RegimeUSMCA {
					planWords = "this is a plan searched before that change"
				}
				for _, w := range []string{"Legal framework not confirmed", "made by Unde's general engine", planWords, "must select the appropriate framework"} {
					if !strings.Contains(notes, w) {
						t.Errorf("%s notes missing %q: %s", format, w, notes)
					}
				}
				for _, bad := range []string{"failed", "not recorded", regime} {
					if strings.Contains(notes, bad) {
						t.Errorf("%s notes wrongly say %q: %s", format, bad, notes)
					}
				}
				rcode, rec := hs.do(t, "POST", "/v1/cases/"+id+"/export", `{"plan_index":0,"format":"`+format+`"}`)
				if rcode != 200 || rec["content"] != prev["content"] || joinNotes(rec) != notes {
					t.Errorf("%s recorded plan export differs from preview (%d)", format, rcode)
				}
			}
		})
	}
}

// A plan whose trace is empty is unknown, not general.
func TestExportPlanDraftWithUnrecordedTrace(t *testing.T) {
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
