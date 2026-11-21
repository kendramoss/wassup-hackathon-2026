// Package adjudicate is a second, focused model call for the engine's largest
// error group (D163): CBP holding China, where the components or
// sub-assemblies were made, against the country where they were assembled,
// or the reverse. It shows contrastive precedents (earlier rulings where CBP
// held China despite later work elsewhere, and earlier rulings where the
// later work conferred origin) and asks one question: is the processing
// outside China a substantial transformation of the Chinese components?
//
// It never sees the engine's answer, only the two candidate countries.
package adjudicate

import (
	"context"
	"fmt"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
)

// Version is recorded with every adjudicated row.
const Version = "adjudicate-v1"

// Pivot is the component country of the error cluster.
const Pivot = "China"

// Trigger modes (D163).
const (
	// Narrow: the engine named a country other than China and the facts
	// name China.
	Narrow = "narrow"
	// Symmetric: the facts name China and another country, whatever the
	// engine named.
	Symmetric = "symmetric"
)

// Options configures the adjudicator.
type Options struct {
	Model           string
	MaxOutputTokens int
	Mode            string  // Narrow or Symmetric
	Below           float64 // trigger only below this stated confidence
	PerSide         int     // precedents per group (default 3)
}

// Adjudicator holds the precedent index and the model.
type Adjudicator struct {
	model    llm.Model
	bm       *retrieval.BM25
	mentions map[string]map[string]int
	opts     Options
}

// New indexes pool (the same precedent pool as the engine).
func New(model llm.Model, pool []domain.Ruling, opts Options) *Adjudicator {
	if opts.PerSide <= 0 {
		opts.PerSide = 3
	}
	m := make(map[string]map[string]int, len(pool))
	for _, r := range pool {
		c, _ := corpus.CountryMentions(r.Facts)
		m[r.ID] = c
	}
	return &Adjudicator{model: model, opts: opts, mentions: m,
		bm: retrieval.NewBM25(pool, func(r domain.Ruling) string { return corpus.Truncate(r.Facts, retrieval.DocRunes) })}
}

// Candidates returns the two countries to choose between and whether the
// case is triggered. The other candidate is the engine's answer when it is
// not China, else the most-named country other than China (first named on a
// tie).
func Candidates(facts, pred string, conf float64, opts Options) (other string, ok bool) {
	if conf >= opts.Below {
		return "", false
	}
	counts, order := corpus.CountryMentions(facts)
	if counts[Pivot] == 0 {
		return "", false
	}
	switch opts.Mode {
	case Narrow:
		if pred == Pivot || pred == "" {
			return "", false
		}
		return pred, true
	case Symmetric:
		if pred != Pivot && pred != "" {
			return pred, true
		}
		best := ""
		for _, c := range order {
			if c != Pivot && (best == "" || counts[c] > counts[best]) {
				best = c
			}
		}
		return best, best != ""
	}
	return "", false
}

// Decision is the adjudicator's answer for one case.
type Decision struct {
	Origin     string  `json:"origin_country"`
	Transforms bool    `json:"substantial_transformation_outside_china"`
	Confidence float64 `json:"confidence"`
	Closest    string  `json:"closest_precedent"`
	Reasoning  string  `json:"reasoning"`
	// Held and Conferred are the precedent IDs shown in each group.
	Held      []string  `json:"held_china"`
	Conferred []string  `json:"conferred_elsewhere"`
	Usage     llm.Usage `json:"-"`
	ModelID   string    `json:"-"`
}

// System is the adjudicator prompt (adjudicate-v1).
const System = `You are a US customs origin analyst. One question decides this case: CBP must choose between China, where the main components or sub-assemblies were made, and another country where later assembly or processing took place.

Decide the country of origin CBP would assign for non-preferential purposes under the substantial-transformation test.

You are shown earlier CBP rulings in two groups:
GROUP A: CBP held that origin stayed with China although assembly or processing took place in another country.
GROUP B: CBP held that the assembly or processing in another country was a substantial transformation of Chinese components.

Compare the case with both groups. Look at what the Chinese parts are (finished functional sub-assemblies, or materials and simple parts), whether a Chinese component alone gives the article its essential character, and how complex, skilled and value-adding the later work is (many parts joined, programmed, tested, calibrated) or how simple (a few parts screwed or plugged together, packed). Then answer the question: is the processing outside China a substantial transformation of the Chinese components?

Return ONLY a JSON object:
{"substantial_transformation_outside_china": true or false,
 "origin_country": "<China or the other candidate, as named>",
 "confidence": <0.0-1.0>,
 "closest_precedent": "<ruling number from either group that is closest to the case>",
 "reasoning": "<=80 words"}`

// Schema constrains the answer.
var Schema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"substantial_transformation_outside_china": map[string]any{"type": "boolean"},
		"origin_country":    map[string]any{"type": "string"},
		"confidence":        map[string]any{"type": "number"},
		"closest_precedent": map[string]any{"type": "string"},
		"reasoning":         map[string]any{"type": "string"},
	},
	"required":             []string{"substantial_transformation_outside_china", "origin_country", "confidence", "closest_precedent", "reasoning"},
	"additionalProperties": false,
}

// Precedents returns the contrastive groups for a case: A, rulings where
// CBP held China and the facts name another country too; B, rulings whose
// facts name China where CBP held another country named in the facts. Both
// are dated strictly before the case.
func (a *Adjudicator) Precedents(c domain.Case) (held, conferred []domain.Ruling) {
	base := retrieval.PrecedentFilter(c)
	heldChina := func(r domain.Ruling) bool {
		m := a.mentions[r.ID]
		return base(r) && r.Origin == Pivot && m[Pivot] > 0 && len(m) >= 2
	}
	conferredElsewhere := func(r domain.Ruling) bool {
		m := a.mentions[r.ID]
		return base(r) && r.Origin != "" && r.Origin != Pivot && m[Pivot] > 0 && m[r.Origin] > 0
	}
	for _, h := range a.bm.Top(c.Facts, a.opts.PerSide, heldChina) {
		held = append(held, h.Ruling)
	}
	for _, h := range a.bm.Top(c.Facts, a.opts.PerSide, conferredElsewhere) {
		conferred = append(conferred, h.Ruling)
	}
	return held, conferred
}

// RenderUser builds the user message: both precedent groups, the two
// candidates and the case facts (same truncation as the engine).
func RenderUser(facts, other string, held, conferred []domain.Ruling) string {
	var b strings.Builder
	group := func(title string, rs []domain.Ruling) {
		b.WriteString(title + "\n")
		if len(rs) == 0 {
			b.WriteString("(no earlier ruling found)\n")
		}
		for _, p := range rs {
			fmt.Fprintf(&b, "[%s | %s] FACTS: %s\nCBP HOLDING: %s\n\n", p.ID, p.Date.Format("2006-01-02"),
				corpus.Truncate(p.Facts, 1500), corpus.Truncate(p.Holding, corpus.PoolHoldingRunes))
		}
	}
	group("GROUP A (CBP held China despite later work elsewhere):", held)
	group("GROUP B (CBP held the later processing conferred origin outside China):", conferred)
	fmt.Fprintf(&b, "CANDIDATES: China or %s\n\n", other)
	b.WriteString("FACTS (ruling request, analysis removed):\n" + corpus.Truncate(facts, 12000))
	return b.String()
}

// Decide makes the adjudicator call for a triggered case.
func (a *Adjudicator) Decide(ctx context.Context, c domain.Case, other string) (Decision, error) {
	held, conferred := a.Precedents(c)
	resp, err := a.model.Generate(ctx, llm.Request{
		Model:           a.opts.Model,
		System:          System,
		Messages:        []llm.Message{{Role: llm.RoleUser, Content: RenderUser(c.Facts, other, held, conferred)}},
		JSONSchema:      Schema,
		SchemaName:      "origin_adjudication",
		MaxOutputTokens: a.opts.MaxOutputTokens,
	})
	if err != nil {
		return Decision{}, fmt.Errorf("adjudicate %s: %w", c.ID, err)
	}
	var d Decision
	if err := resp.DecodeJSON(&d); err != nil {
		return Decision{}, fmt.Errorf("adjudicate %s: %w", c.ID, err)
	}
	d.Origin = domain.NormalizeCountry(d.Origin)
	d.Usage, d.ModelID = resp.Usage, resp.ModelID
	for _, r := range held {
		d.Held = append(d.Held, r.ID)
	}
	for _, r := range conferred {
		d.Conferred = append(d.Conferred, r.ID)
	}
	return d, nil
}

// Apply returns the final origin and confidence: the adjudicator's answer
// when it names one of the two candidates (the lower confidence when it
// changes the engine's answer), else the engine's.
func Apply(pred string, conf float64, other string, d Decision) (string, float64, bool) {
	if d.Origin != Pivot && d.Origin != other {
		return pred, conf, false
	}
	if d.Origin == pred {
		return pred, conf, false
	}
	return d.Origin, min(conf, d.Confidence), true
}
