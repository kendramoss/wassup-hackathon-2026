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

func (e *Engine) Predict(ctx context.Context, c domain.Case) (engine.Result, error) {
	if strings.TrimSpace(c.Facts) == "" && c.Product == nil {
		return engine.Result{}, engine.ErrNoFacts
	}
	if c.Date.IsZero() {
		return engine.Result{}, fmt.Errorf("japan preference: case date is required")
	}
	p := &domain.JapanResult{Outcome: Undetermined}
	result := engine.Result{Prediction: domain.Prediction{
		Japan: p, Precedents: []string{}, CitedRulings: []string{}, KeyFactors: []string{},
	}}
	if c.Japan != nil {
		p.Agreement, p.HS = agreementCode(c.Japan.Agreement), hsNumber(c.Japan.HS)
	}
	switch {
	case p.Agreement == "":
		p.ReviewReason = "Choose the trade agreement you want to use for this import."
		return result, nil
	case p.Agreement == "Non-preferential":
		p.ReviewReason = "This check covers preferences under trade agreements. Ask a specialist about Japan's other origin rules."
		return result, nil
	case !e.rules.HasAgreement(p.Agreement):
		p.ReviewReason = "We do not have a verified product rule for this trade agreement."
		return result, nil
	case p.HS == "":
		p.ReviewReason = "Enter the finished product's tariff code. Use at least four digits."
		return result, nil
	}
	y, m, d := c.Date.Date()
	c.Date = time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	rules, err := e.rules.Lookup(p.Agreement, p.HS, c.Date)
	if err != nil {
		return result, err
	}
	if len(rules) == 0 {
		p.ReviewReason = "We could not verify a rule for this tariff code and date. Check the agreement's tariff schedule or ask a specialist."
		return result, nil
	}

	facts := caseFacts(c)
	query := p.Agreement + " " + facts
	precedents := e.pool.Top(c, p.Agreement, query, e.k)
	for _, r := range rules {
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
		p.ReviewReason = "A specialist needs to check the rule and the missing production or materials facts."
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
