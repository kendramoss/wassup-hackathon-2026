// Package jpengine assesses preferential origin for goods imported into Japan.
package jpengine

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jpcorpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jprules"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

const (
	PromptVersion = "jp-engine-v1"
	DefaultModel  = "gemini-4-argon"
	DefaultK      = 5

	Qualifies      = "qualifies"
	DoesNotQualify = "does_not_qualify"
	Undetermined   = "undetermined"
)

type Engine struct {
	model   llm.Model
	modelID string
	pool    *jpcorpus.Pool
	rules   *jprules.Table
	k       int
}

func New(model llm.Model, modelID string, pool *jpcorpus.Pool, rules *jprules.Table, k int) *Engine {
	return &Engine{model: model, modelID: modelID, pool: pool, rules: rules, k: k}
}

func NewDefault(model llm.Model, modelID string, k int) (*Engine, error) {
	pool, err := jpcorpus.EmbeddedPool()
	if err != nil {
		return nil, err
	}
	rules, err := jprules.Default()
	if err != nil {
		return nil, err
	}
	return New(model, modelID, pool, rules, k), nil
}

func (e *Engine) Get(id string) (domain.Ruling, string, bool) {
	return e.pool.Get(id)
}

var hsSyntax = regexp.MustCompile(`^[0-9]{4}(?:[. ]?[0-9]{2}(?:[-. ]?[0-9]{3})?)?$`)
var rulingNumber = regexp.MustCompile(`\b(?i:JP[0-9]{5,10})\b`)

func hsNumber(raw string) string {
	s := strings.TrimSpace(raw)
	if !hsSyntax.MatchString(s) {
		return ""
	}
	return strings.NewReplacer(".", "", "-", "", " ", "").Replace(s)
}

func agreementCode(raw string) string {
	raw = strings.TrimSpace(raw)
	for code, name := range jpcorpus.AgreementNames {
		if strings.EqualFold(raw, code) || strings.EqualFold(raw, name) {
			return code
		}
	}
	return ""
}

// Engine-level preflight reasons. Rule lookup reasons come from jprules.
const (
	ReasonNoFacts         jprules.Reason = "no_facts"
	ReasonNoDate          jprules.Reason = "no_date"
	ReasonNoAgreement     jprules.Reason = "no_agreement"
	ReasonNonPreferential jprules.Reason = "non_preferential"
	ReasonNoHS            jprules.Reason = "no_hs"
)

const reasonNotInEdition = "This tariff code is not in the HS 2017 edition that this agreement's schedule uses. If it is a newer code, check the agreement's schedule for that edition or ask a specialist."

const reasonUnverifiedRule = "We could not verify a rule for this tariff code and date. Check the agreement's tariff schedule or ask a specialist."

// PreflightResult says whether a case can reach the model and why not. It
// uses no model. Agreement, HS and Date are normalized as Predict uses them;
// Agreement and HS are empty when the input does not name one. Evidence is the
// rule text Predict would send: product specific rule rows, or for an
// unlisted AJCEP HS 2017 code the general originating criteria, or for a code
// of a later HS edition with a reviewed correlation the parent codes' rule row
// behind a mapping note.
type PreflightResult struct {
	Eligible     bool
	Reason       jprules.Reason
	ReviewReason string
	Agreement    string
	HS           string
	Date         time.Time
	Evidence     []jprules.Evidence
	Err          error
}

// Preflight runs every check that precedes the model call. Reasons no_facts
// and no_date are the errors Predict returns.
func Preflight(rules *jprules.Table, c domain.Case) PreflightResult {
	if strings.TrimSpace(c.Facts) == "" && c.Product == nil {
		return PreflightResult{Reason: ReasonNoFacts}
	}
	if c.Date.IsZero() {
		return PreflightResult{Reason: ReasonNoDate}
	}
	var pf PreflightResult
	if c.Japan != nil {
		pf.Agreement, pf.HS = agreementCode(c.Japan.Agreement), hsNumber(c.Japan.HS)
	}
	y, m, d := c.Date.Date()
	pf.Date = time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	switch {
	case pf.Agreement == "":
		pf.Reason = ReasonNoAgreement
		pf.ReviewReason = "Choose the trade agreement you want to use for this import."
		return pf
	case pf.Agreement == "Non-preferential":
		pf.Reason = ReasonNonPreferential
		pf.ReviewReason = "This check covers preferences under trade agreements. Ask a specialist about Japan's other origin rules."
		return pf
	case !rules.HasAgreement(pf.Agreement):
		pf.Reason = jprules.ReasonUnsupportedAgreement
		pf.ReviewReason = "We do not have a verified product rule for this trade agreement."
		return pf
	case pf.HS == "":
		pf.Reason = ReasonNoHS
		pf.ReviewReason = "Enter the finished product's tariff code. Use at least four digits."
		return pf
	}
	res, err := rules.LookupDetailed(pf.Agreement, pf.HS, pf.Date)
	pf.Reason, pf.Err = res.Reason, err
	if err != nil {
		return pf
	}
	if res.Reason == jprules.ReasonEditionHint && res.Hint != "" {
		pf.ReviewReason = res.Hint
		return pf
	}
	if res.Reason == jprules.ReasonNotInEdition {
		pf.ReviewReason = reasonNotInEdition
		return pf
	}
	if res.Reason != jprules.ReasonPSR && res.Reason != jprules.ReasonGeneral && res.Reason != jprules.ReasonMapped || len(res.Evidence) == 0 {
		pf.ReviewReason = reasonUnverifiedRule
		return pf
	}
	pf.Eligible, pf.Evidence = true, res.Evidence
	return pf
}

func (e *Engine) Predict(ctx context.Context, c domain.Case) (engine.Result, error) {
	pf := Preflight(e.rules, c)
	switch pf.Reason {
	case ReasonNoFacts:
		return engine.Result{}, engine.ErrNoFacts
	case ReasonNoDate:
		return engine.Result{}, fmt.Errorf("japan preference: case date is required")
	}
	p := &domain.JapanResult{Outcome: Undetermined, Agreement: pf.Agreement, HS: pf.HS, ReviewReason: pf.ReviewReason}
	result := engine.Result{Prediction: domain.Prediction{
		Japan: p, Precedents: []string{}, CitedRulings: []string{}, KeyFactors: []string{},
	}}
	if pf.Err != nil {
		return result, pf.Err
	}
	if !pf.Eligible {
		return result, nil
	}
	c.Date = pf.Date
	rules := pf.Evidence

	facts := caseFacts(c)
	query := p.Agreement + " " + facts
	precedents := e.pool.Top(c, p.Agreement, query, e.k)
	for _, r := range rules {
		if r.URL == "" { // an Unde Labs caveat note is not a treaty source
			continue
		}
		p.Sources = append(p.Sources, domain.JapanSource{Citation: r.Citation, URL: r.URL})
	}
	for _, r := range precedents {
		result.Prediction.Precedents = append(result.Prediction.Precedents, r.ID)
	}

	result.Votes = 1
	start := time.Now()
	resp, err := e.model.Generate(ctx, llm.Request{
		Model: e.modelID, System: systemPrompt, SchemaName: "jp_preference", JSONSchema: answerSchema, MaxOutputTokens: 8192,
		Messages: []llm.Message{{Role: llm.RoleUser, Content: userPrompt(p.Agreement, p.HS, facts, rules, precedents)}},
	})
	result.Usage = resp.Usage
	result.Prediction.Trace = domain.Trace{
		Provider: resp.Provider, ModelID: resp.ModelID, RequestedModel: e.modelID, PromptVersion: PromptVersion,
		InputTokens: resp.Usage.InputTokens, OutputTokens: resp.Usage.OutputTokens, LatencyMS: time.Since(start).Milliseconds(),
	}
	if err != nil {
		return result, err
	}
	var a modelAnswer
	if err := resp.DecodeJSON(&a); err != nil {
		return result, err
	}
	if err := a.validate(); err != nil {
		return result, err
	}
	p.Outcome, p.Criterion, p.DecidingRule = a.Outcome, a.Criterion, a.DecidingRule
	result.Prediction.Confidence = *a.Confidence
	result.Prediction.Reasoning = a.Reasoning
	if p.Outcome == Qualifies {
		result.Prediction.Origin = normalizeOrigin(a.Origin)
	} else if p.Outcome == Undetermined {
		p.ReviewReason = "A specialist needs to check the rule and the missing production or materials facts." + missingFactsSentence(MissingFacts(c))
	}
	allowed := make(map[string]bool, len(precedents))
	named := make(map[string]bool, len(a.Cited))
	for _, id := range a.Cited {
		named[strings.ToUpper(strings.TrimSpace(id))] = true
	}
	for _, r := range precedents {
		allowed[r.ID] = true
		if named[r.ID] {
			_, u, _ := e.pool.Get(r.ID)
			result.Prediction.CitedRulings = append(result.Prediction.CitedRulings, r.ID)
			p.PrecedentSources = append(p.PrecedentSources, domain.JapanSource{Citation: r.ID, URL: u})
		}
	}
	maskUnknown := func(s string) string {
		if allowed[strings.ToUpper(s)] {
			return s
		}
		return "an unverified reference"
	}
	result.Prediction.Reasoning = rulingNumber.ReplaceAllStringFunc(result.Prediction.Reasoning, maskUnknown)
	p.DecidingRule = rulingNumber.ReplaceAllStringFunc(p.DecidingRule, maskUnknown)
	return result, nil
}

func normalizeOrigin(s string) string {
	if strings.EqualFold(s, "EU") || strings.EqualFold(s, "European Union") {
		return "EU"
	}
	return domain.NormalizeCountry(s)
}

func caseFacts(c domain.Case) string {
	if c.Product == nil {
		return c.Facts
	}
	var out strings.Builder
	out.WriteString(c.Facts)
	fmt.Fprintf(&out, "\nProduct: %s. %s", c.Product.Name, c.Product.Description)
	for _, part := range c.Product.Components {
		fmt.Fprintf(&out, "\nMaterial: %s; made in: %s; tariff code: %s", part.Name, part.MadeIn, part.HTS)
	}
	for _, op := range c.Product.Operations {
		if pl, ok := c.Product.PlantByID(op.Plant); ok {
			fmt.Fprintf(&out, "\nProduction: %s; country: %s", op.Name, pl.Country)
		}
	}
	return out.String()
}
