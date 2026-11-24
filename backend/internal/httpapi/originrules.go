package httpapi

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jurisdiction"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/nporigin"
)

// OriginRulesLabel is shown with every EU and UK answer (D766): the checks
// apply published rules, and no origin decisions with their facts are
// published to test them against.
const OriginRulesLabel = "Rules check only — no accuracy figure (no published origin rulings to test against)"

// maxOriginMaterials caps the materials of one EU or UK check, and
// maxOriginAnswers its answers.
const (
	maxOriginMaterials = 50
	maxOriginAnswers   = 100
)

// originMarket is what differs between the EU and UK routes.
type originMarket struct {
	code  string
	rules string
	note  string
}

var (
	euMarket = originMarket{code: jurisdiction.EU, rules: "EU non-preferential origin: UCC Art. 60, Delegated Regulation (EU) 2015/2446 Art. 31-36 and Annex 22-01",
		note: "The EU's non-preferential origin rules, applied to the codes and answers given. Not a ruling."}
	ukMarket = originMarket{code: jurisdiction.UK, rules: "UK non-preferential origin: TCTA 2018 s. 17, SI 2020/1433 and the HMRC reference document v1.2 (Part Four)",
		note: "The UK's non-preferential origin rules, applied to the codes and answers given. Not a ruling."}
)

// OriginSourceView is the pinned source of a market's rules.
type OriginSourceView struct {
	Title      string `json:"title"`
	Instrument string `json:"instrument"`
	URL        string `json:"url"`
	Version    string `json:"version"`
	AsOf       string `json:"as_of"`
	Retrieved  string `json:"retrieved"`
	SHA256     string `json:"sha256"`
	Licence    string `json:"licence"`
}

// OriginRulesView is the answer of POST /v1/rules/eu-origin and
// POST /v1/rules/uk-origin.
type OriginRulesView struct {
	Jurisdiction string `json:"jurisdiction"`
	Rules        string `json:"rules"`
	TableVersion string `json:"table_version"`
	Label        string `json:"label"`
	// Chapters are the HS chapters the list has rules for; a good in any
	// other chapter gets a judgement outcome.
	Chapters []string `json:"chapters"`
	// Country and CountryFrom say where production was read from, when the
	// request sent a product.
	Country        string           `json:"country"`
	CountryFrom    string           `json:"country_from,omitempty"`
	Result         nporigin.Result  `json:"result"`
	Source         OriginSourceView `json:"source"`
	Administration string           `json:"administration"`
	Note           string           `json:"note"`
}

type originRulesRequest struct {
	// Product, when sent, is read like the case's U.S. rule check: its HS
	// code, its parts' codes and origins, and the production country from
	// its plants. Otherwise the fields of nporigin.Input are used.
	Product *domain.Product `json:"product,omitempty"`
	nporigin.Input
}

func (a *api) euOrigin(w http.ResponseWriter, r *http.Request) {
	a.originRules(w, r, euMarket, a.d.EUOrigin)
}

func (a *api) ukOrigin(w http.ResponseWriter, r *http.Request) {
	a.originRules(w, r, ukMarket, a.d.UKOrigin)
}

// originRules answers an EU or UK non-preferential origin check (D760-D766).
// No model call; nothing is stored.
func (a *api) originRules(w http.ResponseWriter, r *http.Request, m originMarket, rules *nporigin.Rules) {
	var in originRulesRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	q := in.Input
	v := OriginRulesView{Jurisdiction: m.code, Rules: m.rules, Label: OriginRulesLabel, Note: m.note}
	if in.Product != nil {
		v.Country, v.CountryFrom = productionCountry(*in.Product)
		q.GoodHTS, q.Country, q.Materials = in.Product.HTS, v.Country, nil
		for _, c := range in.Product.Components {
			q.Materials = append(q.Materials, nporigin.Material{Name: c.Name, HTS: c.HTS, Origin: c.MadeIn})
		}
	} else {
		v.Country = domain.NormalizeCountry(q.Country)
	}
	if err := validateOriginInput(rules, q); err != nil {
		a.fail(w, r, err)
		return
	}
	t := rules.Table()
	v.TableVersion, v.Chapters, v.Administration = t.Version, t.ChapterCodes(), rules.General().Administration
	s := t.Source
	v.Source = OriginSourceView{Title: s.Title, Instrument: s.Instrument, URL: s.URL, Version: s.Version, AsOf: s.AsOf, Retrieved: s.Retrieved, SHA256: s.SHA256, Licence: s.Licence}
	v.Result = rules.Evaluate(q)
	writeJSON(w, http.StatusOK, v)
}

// validateOriginInput checks the request's shape: a code, at most 50
// materials with shares in [0, 1], answers and a row the table knows.
func validateOriginInput(rules *nporigin.Rules, q nporigin.Input) error {
	if strings.TrimSpace(q.GoodHTS) == "" {
		return fmt.Errorf("%w: good_hts (or product.hts) is required", domain.ErrInvalid)
	}
	if len(q.Materials) > maxOriginMaterials {
		return fmt.Errorf("%w: at most %d materials", domain.ErrInvalid, maxOriginMaterials)
	}
	share := func(i int, field string, v *float64) error {
		if v != nil && (math.IsNaN(*v) || math.IsInf(*v, 0) || *v < 0 || *v > 1) {
			return fmt.Errorf("%w: materials[%d].%s must be a number from 0 to 1", domain.ErrInvalid, i, field)
		}
		return nil
	}
	for i, m := range q.Materials {
		if err := share(i, "value_share", m.ValueShare); err != nil {
			return err
		}
		if err := share(i, "weight_share", m.WeightShare); err != nil {
			return err
		}
	}
	if len(q.Answers) > maxOriginAnswers {
		return fmt.Errorf("%w: at most %d answers", domain.ErrInvalid, maxOriginAnswers)
	}
	for k := range q.Answers {
		if !rules.ValidAnswer(k) {
			return fmt.Errorf("%w: answers: %q is not a question these rules ask", domain.ErrInvalid, k)
		}
	}
	if q.Row != "" && !rules.ValidRow(q.Row) {
		return fmt.Errorf("%w: row %q is not a row of these rules", domain.ErrInvalid, q.Row)
	}
	return nil
}

// OriginRulesAuthority registers the EU or the UK as a rules-only market
// (D766): its "engine" is the non-preferential origin check on the case's
// product codes. No model is called and no ruling is retrieved, so its fast
// path is 0: every answer goes to review.
func OriginRulesAuthority(market string, rules *nporigin.Rules) jurisdiction.Authority {
	a := jurisdiction.Authority{Market: market, Engine: originEngine{rules: rules},
		Precedents: "none (rules only; no published origin rulings to test against)"}
	switch market {
	case jurisdiction.EU:
		a.Agency = "Customs authorities of the EU Member States"
		a.Rules = "UCC Art. 60, Delegated Regulation (EU) 2015/2446 Art. 31-36 and Annex 22-01 (non-preferential origin), rules check only"
	case jurisdiction.UK:
		a.Agency = "HM Revenue & Customs"
		a.Rules = "TCTA 2018 s. 17, SI 2020/1433 and the HMRC reference document v1.2 (non-preferential origin), rules check only"
	}
	return a
}

type originEngine struct{ rules *nporigin.Rules }

// Predict reads the case's product like the U.S. rule check and answers
// with the list-rule result. It never fails: a case without codes, or with
// a good the list does not decide, gets no origin and a reason.
func (e originEngine) Predict(_ context.Context, c domain.Case) (engine.Result, error) {
	p := domain.Prediction{
		Trace: domain.Trace{Provider: "rules", ModelID: "none", PromptVersion: e.rules.Table().Version},
	}
	if c.Product == nil || !productHasCodes(*c.Product) {
		p.Reasoning = OriginRulesLabel + ". Add HS codes for the product and its parts to run the origin rules."
		return engine.Result{Prediction: p}, nil
	}
	country, _ := productionCountry(*c.Product)
	in := nporigin.Input{GoodHTS: c.Product.HTS, Country: country}
	for _, comp := range c.Product.Components {
		in.Materials = append(in.Materials, nporigin.Material{Name: comp.Name, HTS: comp.HTS, Origin: comp.MadeIn})
	}
	res := e.rules.Evaluate(in)
	if res.Status == nporigin.Decided {
		p.Origin, p.DecisiveStep = res.Origin, res.Basis
	}
	reason := strings.TrimSpace(res.Reason)
	if reason != "" {
		reason = strings.ToUpper(reason[:1]) + reason[1:]
	}
	p.Reasoning = OriginRulesLabel + ". " + strings.TrimSuffix(reason, ".") + "."
	p.KeyFactors = res.Assumes
	return engine.Result{Prediction: p}, nil
}
