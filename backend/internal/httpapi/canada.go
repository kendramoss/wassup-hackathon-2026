package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jurisdiction"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
)

// CanadaLabel is shown with every Canadian answer: the check has no
// accuracy figure, because Canada publishes too few origin rulings to test
// it against (D725).
const CanadaLabel = "Rules check only — no accuracy figure yet (no published rulings to test against)"

const canadaNote = "Canada's marking rules for goods imported from the United States or Mexico (SOR/94-23), applied to the codes given. Not a ruling."

// maxCanadaMaterials caps the materials of one check.
const maxCanadaMaterials = 50

// CanadaMarkingView is the answer of POST /v1/rules/canada-marking.
type CanadaMarkingView struct {
	Jurisdiction string `json:"jurisdiction"`
	Rules        string `json:"rules"`
	TableVersion string `json:"table_version"`
	Label        string `json:"label"`
	// Country and CountryFrom say where production was read from, when the
	// request sent a product.
	Country     string             `json:"country"`
	CountryFrom string             `json:"country_from,omitempty"`
	Result      tariffshift.Result `json:"result"`
	// Guidance says what would resolve an undetermined result; absent
	// otherwise. It is not part of Result, so the case rule check is
	// unchanged.
	Guidance *tariffshift.Guidance `json:"guidance,omitempty"`
	Note     string                `json:"note"`
}

type canadaMarkingRequest struct {
	// Product, when sent, is read like the case's U.S. rule check: its HTS
	// code, its parts' codes and origins, and the production country from
	// its plants (D441). Otherwise the fields of tariffshift.Input are used.
	Product *domain.Product `json:"product,omitempty"`
	tariffshift.Input
}

// canadaMarking answers POST /v1/rules/canada-marking: Canada's marking
// origin for a good from its HTS code and its materials' codes and origins,
// under SOR/94-23 (D720-D725). No model call; nothing is stored.
func (a *api) canadaMarking(w http.ResponseWriter, r *http.Request) {
	var in canadaMarkingRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	v := CanadaMarkingView{Jurisdiction: "ca", Rules: "SOR/94-23 Schedule III", TableVersion: a.d.CanadaMarking.Table().Version,
		Label: CanadaLabel, Note: canadaNote}
	q := in.Input
	if in.Product != nil {
		p := *in.Product
		v.Country, v.CountryFrom = productionCountry(p)
		q = tariffshift.Input{GoodHTS: p.HTS, Country: v.Country, SimpleAssembly: in.SimpleAssembly, GRI2a: in.GRI2a, BendingToShape: in.BendingToShape}
		for _, c := range p.Components {
			q.Materials = append(q.Materials, tariffshift.Material{Name: c.Name, HTS: c.HTS, Origin: c.MadeIn})
		}
	} else {
		v.Country = domain.NormalizeCountry(q.Country)
	}
	if strings.TrimSpace(q.GoodHTS) == "" {
		a.fail(w, r, fmt.Errorf("%w: good_hts (or product.hts) is required", domain.ErrInvalid))
		return
	}
	if len(q.Materials) > maxCanadaMaterials {
		a.fail(w, r, fmt.Errorf("%w: at most %d materials", domain.ErrInvalid, maxCanadaMaterials))
		return
	}
	if err := tariffshift.ValidateShares(q.Materials); err != nil {
		a.fail(w, r, err)
		return
	}
	v.Result = a.d.CanadaMarking.Evaluate(q)
	if v.Result.Status == tariffshift.Undetermined {
		v.Guidance = tariffshift.Guide(a.d.CanadaMarking, q, v.Result)
	}
	writeJSON(w, http.StatusOK, v)
}

// CanadaAuthority registers Canada as a rules-only market (D725): its
// "engine" is the SOR/94-23 check on the case's product codes. No model is
// called and no ruling is retrieved, so its fast path is 0: every answer
// goes to review, and an answer without codes has no origin.
func CanadaAuthority(rules *tariffshift.Rules) jurisdiction.Authority {
	return jurisdiction.Authority{
		Market:     jurisdiction.CA,
		Agency:     "Canada Border Services Agency",
		Rules:      "SOR/94-23 (marking rules for goods from CUSMA countries), rules check only",
		Precedents: "none (rules only; no published rulings to test against)",
		Engine:     canadaEngine{rules: rules},
	}
}

type canadaEngine struct{ rules *tariffshift.Rules }

// Predict reads the case's product like the U.S. rule check and answers
// with the SOR/94-23 result. It never fails: a case without the codes the
// rules need gets no origin and a reason saying what to add.
func (e canadaEngine) Predict(_ context.Context, c domain.Case) (engine.Result, error) {
	p := domain.Prediction{
		Trace: domain.Trace{Provider: "rules", ModelID: "none", PromptVersion: e.rules.Table().Version},
	}
	if c.Product == nil || !productHasCodes(*c.Product) {
		p.Reasoning = CanadaLabel + ". Add HS codes for the product and its parts to run Canada's marking rules."
		return engine.Result{Prediction: p}, nil
	}
	country, _ := productionCountry(*c.Product)
	in := tariffshift.Input{GoodHTS: c.Product.HTS, Country: country}
	for _, comp := range c.Product.Components {
		in.Materials = append(in.Materials, tariffshift.Material{Name: comp.Name, HTS: comp.HTS, Origin: comp.MadeIn})
	}
	res := e.rules.Evaluate(in)
	if res.Status == tariffshift.Decided {
		p.Origin, p.DecisiveStep = res.Origin, res.Basis
	}
	p.Reasoning = CanadaLabel + ". " + strings.ToUpper(res.Reason[:1]) + res.Reason[1:] + "."
	p.KeyFactors = res.Assumes
	return engine.Result{Prediction: p}, nil
}

// productHasCodes: a code for the good and at least one part.
func productHasCodes(p domain.Product) bool {
	if strings.TrimSpace(p.HTS) == "" {
		return false
	}
	for _, c := range p.Components {
		if strings.TrimSpace(c.HTS) != "" {
			return true
		}
	}
	return false
}
