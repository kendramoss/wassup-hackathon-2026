package httpapi

import (
	"context"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jurisdiction"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
)

// MexicoLabel is shown with every Mexican answer.
const MexicoLabel = "Rules check only"

// MexicoAuthority registers Mexico as a rules-only market (D914): its answer
// is the non-preferential origin check of the Acuerdo DOF 30-08-1994
// (Anexo I and its Apéndice) on the case's product codes. No model is called
// and no ruling is retrieved; every answer goes to review.
func MexicoAuthority(rules *tariffshift.Rules) jurisdiction.Authority {
	return jurisdiction.Authority{
		Market:     jurisdiction.MX,
		Agency:     "Agencia Nacional de Aduanas de México",
		Rules:      "Acuerdo DOF 30-08-1994, Anexo I (non-preferential country of origin), rules check only",
		Precedents: "none (rules only)",
		Engine:     mexicoEngine{rules: rules},
	}
}

type mexicoEngine struct{ rules *tariffshift.Rules }

// Predict never fails: a case without the codes the rules need gets no
// origin and a reason saying what to add.
func (e mexicoEngine) Predict(_ context.Context, c domain.Case) (engine.Result, error) {
	p := domain.Prediction{
		Trace: domain.Trace{Provider: "rules", ModelID: "none", PromptVersion: e.rules.Table().Version},
	}
	if c.Product == nil || !productHasCodes(*c.Product) {
		p.Reasoning = MexicoLabel + ". Add HS codes for the product and its parts to run Mexico's origin rules."
		return engine.Result{Prediction: p}, nil
	}
	country, _ := productionCountry(*c.Product)
	in := tariffshift.Input{GoodHTS: c.Product.HTS, Country: country}
	for _, comp := range c.Product.Components {
		in.Materials = append(in.Materials, tariffshift.Material{Name: comp.Name, HTS: comp.HTS, Origin: comp.MadeIn})
	}
	res := e.rules.Evaluate(in)
	reason := res.Reason
	if res.Status == tariffshift.OutOfScope {
		reason = "this chapter has no specific rule, so origin is where the last substantial transformation with a change of tariff classification took place (Anexo I, art. 3 II); a person has to judge it"
	}
	if res.Status == tariffshift.Decided {
		p.Origin, p.DecisiveStep = res.Origin, res.Basis
	}
	p.Reasoning = MexicoLabel + ". " + strings.ToUpper(reason[:1]) + reason[1:] + "."
	p.KeyFactors = res.Assumes
	return engine.Result{Prediction: p}, nil
}
