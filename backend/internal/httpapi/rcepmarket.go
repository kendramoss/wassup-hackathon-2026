package httpapi

import (
	"context"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jurisdiction"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/rcep"
)

// RCEPMarketLabel is shown with every South Korea and Australia answer.
const RCEPMarketLabel = "RCEP rules check"

// RCEPAuthority registers a market whose answer is the RCEP preference
// check (Chapter 3, Annex 3A) on the case's product codes: South Korea and
// Australia, both RCEP Parties. No model is called and no ruling is
// retrieved; every answer goes to review. A good that qualifies takes the
// origin of the Party where it was produced (Art. 3.2).
func RCEPAuthority(market, agency string) jurisdiction.Authority {
	return jurisdiction.Authority{
		Market:     market,
		Agency:     agency,
		Rules:      "RCEP Agreement, Chapter 3 and Annex 3A, rules check only",
		Precedents: "none (rules only)",
		Engine:     rcepEngine{t: rcep.Default()},
	}
}

type rcepEngine struct{ t *rcep.Table }

// Predict never fails: a case without the codes the rules need gets no
// origin and a reason saying what to add. Parts are read as not known to be
// RCEP originating unless they come from outside RCEP, which makes them
// non-originating (Art. 3.5.4).
func (e rcepEngine) Predict(_ context.Context, c domain.Case) (engine.Result, error) {
	p := domain.Prediction{
		Trace: domain.Trace{Provider: "rules", ModelID: "none", PromptVersion: e.t.Version},
	}
	if c.Product == nil || !productHasCodes(*c.Product) {
		p.Reasoning = RCEPMarketLabel + ". Add HS codes for the product and its parts to run the RCEP rules."
		return engine.Result{Prediction: p}, nil
	}
	country, _ := productionCountry(*c.Product)
	in := rcep.Input{HS: c.Product.HTS, ProducedIn: country}
	for _, comp := range c.Product.Components {
		st := rcep.StatusUnknown
		if rcep.Party(comp.MadeIn) == "" && strings.TrimSpace(comp.MadeIn) != "" {
			st = rcep.StatusNonOriginating
		}
		in.Materials = append(in.Materials, rcep.Material{Name: comp.Name, HS: comp.HTS, Country: comp.MadeIn, Status: st})
	}
	res, err := e.t.Check(in)
	if err != nil {
		p.Reasoning = RCEPMarketLabel + ". The product's codes could not be read: " + err.Error()
		return engine.Result{Prediction: p}, nil
	}
	if res.Outcome == "yes" {
		p.Origin = country
		p.DecisiveStep = "RCEP Art. 3.2"
		if res.Rule != nil {
			p.DecisiveStep += ", Annex 3A " + res.Rule.Code
		}
		if res.Applied != "" {
			p.DecisiveStep += " (" + res.Applied + ")"
		}
	}
	p.Reasoning = RCEPMarketLabel + ". " + strings.Join(res.Reasons, " ")
	return engine.Result{Prediction: p}, nil
}
