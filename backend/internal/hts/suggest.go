package hts

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
)

// PromptVersion is recorded with every suggestion.
const PromptVersion = "hts-suggest-v2" // v2 (D92): ranks the candidates by fit, and they come from CROSS

// Note goes with every suggestion.
const Note = "suggestion only — confirm the classification with a licensed broker"

// Retrieval sizes: K rulings are retrieved, grouped by subheading, and the
// MaxCandidates best subheadings are returned.
const (
	K             = 10
	MaxCandidates = 3
	excerptRunes  = 800
)

const system = `You help an importer find the U.S. tariff (HTSUS) subheading of a product. You are given the product and up to six candidate 8-digit subheadings. Each candidate lists the CBP rulings that classified goods under it, with each ruling's subject and, when available, an excerpt of its facts.

Return the candidates ordered from best to worst fit for THIS product: what it is and does (its function, its principal component and material), not words it shares with a ruling's subject. A ruling about a different kind of goods that only shares a word (a toy named after the product, a part, a set that merely includes it) is a poor fit. For each candidate write one or two plain sentences on why it might or might not fit, comparing the product with the goods in its rulings. Cite in "cited" only rulings listed under that candidate. If a candidate's rulings describe different goods, say so. Do not propose subheadings that are not listed and do not state duty rates. This is a suggestion for a licensed broker to confirm, not a classification.`

// Ref is a ruling that supports a candidate.
type Ref struct {
	ID      string `json:"id"`
	Date    string `json:"date"`
	Subject string `json:"subject"`
	URL     string `json:"url"`
}

// Candidate is one suggested subheading.
type Candidate struct {
	HTS        string `json:"hts"`
	Supporting []Ref  `json:"supporting_rulings"`
	// Reason is the model's short explanation ("" when the model call failed).
	Reason string `json:"reason"`
	// Cited are the supporting rulings the reason relies on (a subset of Supporting).
	Cited []string `json:"cited"`
	// DutyRates is true when the sourced duty table (D52) has rates for it.
	DutyRates bool `json:"duty_rates_available"`
}

// Result is a suggestion.
type Result struct {
	Candidates []Candidate `json:"candidates"`
	Note       string      `json:"note"`
	// ReasonsUnavailable is true when the candidates are shown without
	// reasons because the model call failed.
	ReasonsUnavailable bool          `json:"reasons_unavailable"`
	Trace              *domain.Trace `json:"trace"`
	// Source is "cross" (a live search of CBP's rulings) or "index" (the
	// local index of rulings that state a subheading).
	Source string `json:"source"`
}

// RateLookup reports whether the duty table has rates for a subheading
// (duty.Table.Line).
type RateLookup func(hts string) bool

// Suggester retrieves rulings that state a subheading and explains the best
// candidates.
type Suggester struct {
	// live and current, when set, take candidates from a CROSS search
	// (D92); the local index answers when CROSS fails or finds nothing.
	live    Live
	current Current
	model   llm.Model
	modelID string
	index   *retrieval.BM25
	entries map[string]Entry
	facts   map[string]string
	rates   RateLookup
	count   int
	subs    int
}

// New builds a suggester over the index entries that have a facts file in
// factsDir (<id>.txt). rates may be nil.
func New(m llm.Model, modelID string, x *Index, factsDir string, rates RateLookup) (*Suggester, error) {
	s := &Suggester{model: m, modelID: modelID, entries: map[string]Entry{}, facts: map[string]string{}, rates: rates}
	var pool []domain.Ruling
	seen := map[string]bool{}
	for _, e := range x.Rulings {
		b, err := os.ReadFile(filepath.Join(factsDir, e.Ruling+".txt"))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("hts: read facts %s: %w", e.Ruling, err)
		}
		date, _ := time.Parse(time.DateOnly, e.Date)
		pool = append(pool, domain.Ruling{ID: e.Ruling, Date: date, Subject: e.Subject, Facts: string(b)})
		s.entries[e.Ruling], s.facts[e.Ruling] = e, string(b)
		for _, h := range e.HTS {
			seen[h] = true
		}
	}
	if len(pool) == 0 {
		return nil, fmt.Errorf("hts: no indexed ruling has facts in %s", factsDir)
	}
	s.count, s.subs = len(pool), len(seen)
	s.index = retrieval.NewBM25(pool, indexText)
	return s, nil
}

// indexText is what BM25 sees: the subject (it usually names the product)
// and the start of the facts.
func indexText(r domain.Ruling) string {
	return r.Subject + "\n" + corpus.Truncate(r.Facts, retrieval.DocRunes)
}

// WithCROSS makes the suggester take candidates from a live CROSS search,
// keeping only subheadings current reports in today's HTS (nil keeps all).
func (s *Suggester) WithCROSS(live Live, current Current) *Suggester {
	s.live, s.current = live, current
	return s
}

// Source says where the last candidates came from, for the response.
const (
	SourceCROSS = "cross"
	SourceIndex = "index"
)

// Coverage is the number of rulings and distinct subheadings searched.
func (s *Suggester) Coverage() (rulings, subheadings int) { return s.count, s.subs }

// Candidates groups the K rulings most like facts by subheading, scoring each
// subheading by the sum of its rulings' BM25 scores, and returns the best
// MaxCandidates (ties: the subheading retrieved first). No model call.
func (s *Suggester) Candidates(facts string) []Candidate {
	hits := s.index.Top(facts, K, func(domain.Ruling) bool { return true })
	score := map[string]float64{}
	var order []string
	refs := map[string][]Ref{}
	for _, h := range hits {
		if h.Score <= 0 {
			continue // no shared term: not evidence for anything
		}
		e := s.entries[h.Ruling.ID]
		for _, code := range e.HTS {
			if _, ok := score[code]; !ok {
				order = append(order, code)
			}
			score[code] += h.Score
			refs[code] = append(refs[code], Ref{ID: e.Ruling, Date: e.Date, Subject: e.Subject, URL: domain.RulingURL(e.Ruling)})
		}
	}
	sort.SliceStable(order, func(i, j int) bool { return score[order[i]] > score[order[j]] })
	if len(order) > MaxCandidates {
		order = order[:MaxCandidates]
	}
	out := make([]Candidate, 0, len(order))
	for _, code := range order {
		c := Candidate{HTS: code, Supporting: refs[code], Cited: []string{}}
		if s.rates != nil {
			c.DutyRates = s.rates(code)
		}
		out = append(out, c)
	}
	return out
}

type answer struct {
	Candidates []struct {
		HTS    string   `json:"hts"`
		Reason string   `json:"reason"`
		Cited  []string `json:"cited"`
	} `json:"candidates"`
}

// schema constrains the answer to the candidates' subheadings and rulings.
func schema(cs []Candidate) map[string]any {
	var codes, ids []string
	for _, c := range cs {
		codes = append(codes, c.HTS)
		for _, r := range c.Supporting {
			if !slices.Contains(ids, r.ID) {
				ids = append(ids, r.ID)
			}
		}
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"candidates": map[string]any{"type": "array", "items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"hts":    map[string]any{"type": "string", "enum": codes},
					"reason": map[string]any{"type": "string"},
					"cited":  map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": ids}},
				},
				"required": []string{"hts", "reason", "cited"},
			}},
		},
		"required": []string{"candidates"},
	}
}

// Suggest returns up to MaxCandidates subheadings for facts, each explained
// by one model call. If the model call fails the candidates are returned
// without reasons (ReasonsUnavailable) and the error is returned too, for
// logging; the caller may show the result.
func (s *Suggester) Suggest(ctx context.Context, facts string) (Result, error) {
	return s.SuggestNamed(ctx, facts, "")
}

// SuggestNamed is Suggest with the product's name, which the CROSS search
// tries first (CBP's subjects name the product).
func (s *Suggester) SuggestNamed(ctx context.Context, facts, name string) (Result, error) {
	res := Result{Note: Note, Source: SourceIndex}
	local := s.Candidates(facts)
	if s.live != nil {
		if cs, err := crossCandidates(ctx, s.live, s.current, facts, name); err == nil && len(cs) > 0 {
			// Half the pool from each source, so neither crowds the other out:
			// CROSS knows every product CBP classified, the local index has
			// the full facts of recent rulings. The model ranks the pool (D92).
			res.Candidates, res.Source = interleave(cs, local, crossPool), SourceCROSS
		}
	}
	if res.Source == SourceIndex {
		res.Candidates = local
	}
	for i := range res.Candidates {
		if s.rates != nil {
			res.Candidates[i].DutyRates = s.rates(res.Candidates[i].HTS)
		}
	}
	if len(res.Candidates) == 0 {
		return res, nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "PRODUCT:\n%s\n\n", corpus.Truncate(strings.TrimSpace(facts), domain.MaxFactsRunes))
	for i, c := range res.Candidates {
		fmt.Fprintf(&b, "CANDIDATE %d: %s\n", i+1, c.HTS)
		for _, r := range c.Supporting {
			if f, ok := s.facts[r.ID]; ok {
				excerpt := strings.Join(strings.Fields(corpus.Truncate(f, excerptRunes)), " ")
				fmt.Fprintf(&b, "- %s (%s): %s\n  Facts: %s\n", r.ID, r.Date, r.Subject, excerpt)
			} else {
				fmt.Fprintf(&b, "- %s (%s): %s\n", r.ID, r.Date, r.Subject) // a CROSS hit: its subject only
			}
		}
		b.WriteString("\n")
	}
	resp, err := s.model.Generate(ctx, llm.Request{Model: s.modelID, System: system,
		Messages:   []llm.Message{{Role: llm.RoleUser, Content: b.String()}},
		JSONSchema: schema(res.Candidates), SchemaName: "hts_suggestion", MaxOutputTokens: 2000})
	if err == nil {
		var a answer
		if err = resp.DecodeJSON(&a); err == nil {
			res.Trace = &domain.Trace{Provider: resp.Provider, ModelID: resp.ModelID, PromptVersion: PromptVersion,
				InputTokens: resp.Usage.InputTokens, OutputTokens: resp.Usage.OutputTokens, LatencyMS: resp.Latency.Milliseconds()}
			res.Candidates = apply(res.Candidates, a)
			return res, nil
		}
	}
	res.ReasonsUnavailable = true
	if len(res.Candidates) > MaxCandidates {
		res.Candidates = res.Candidates[:MaxCandidates] // retrieval order: the most-supported first
	}
	return res, fmt.Errorf("hts: %w", err)
}

// interleave takes candidates from a and b in turn, skipping duplicates,
// up to n in all.
func interleave(a, b []Candidate, n int) []Candidate {
	var out []Candidate
	add := func(c Candidate) {
		if len(out) < n && !slices.ContainsFunc(out, func(x Candidate) bool { return x.HTS == c.HTS }) {
			out = append(out, c)
		}
	}
	for i := 0; i < max(len(a), len(b)); i++ {
		if i < len(a) {
			add(a[i])
		}
		if i < len(b) {
			add(b[i])
		}
	}
	return out
}

// apply copies the model's reasons onto the candidates, keeping only
// citations of rulings retrieved for that candidate, and orders them as the
// model ranked them (candidates it left out follow, in retrieval order).
// MaxCandidates are returned.
func apply(cs []Candidate, a answer) []Candidate {
	for i := range cs {
		for _, ac := range a.Candidates {
			if ac.HTS != cs[i].HTS || cs[i].Reason != "" {
				continue
			}
			cs[i].Reason = strings.TrimSpace(ac.Reason)
			for _, id := range ac.Cited {
				ok := slices.ContainsFunc(cs[i].Supporting, func(r Ref) bool { return r.ID == id })
				if ok && !slices.Contains(cs[i].Cited, id) {
					cs[i].Cited = append(cs[i].Cited, id)
				}
			}
		}
	}
	var out []Candidate
	used := map[string]bool{}
	for _, ac := range a.Candidates {
		for _, c := range cs {
			if c.HTS == ac.HTS && !used[c.HTS] {
				used[c.HTS] = true
				out = append(out, c)
			}
		}
	}
	for _, c := range cs {
		if !used[c.HTS] {
			out = append(out, c)
		}
	}
	if len(out) > MaxCandidates {
		out = out[:MaxCandidates]
	}
	return out
}

// ProductText describes a structured product for retrieval: its name,
// description, components and operations (no family template needed).
func ProductText(p domain.Product) string {
	var b strings.Builder
	fmt.Fprintf(&b, "The merchandise is a %s.", strings.TrimSpace(p.Name))
	if d := strings.TrimSpace(p.Description); d != "" {
		fmt.Fprintf(&b, " %s", d)
	}
	var parts []string
	for _, c := range p.Components {
		parts = append(parts, c.Name)
	}
	if len(parts) > 0 {
		fmt.Fprintf(&b, " It consists of %s.", strings.Join(parts, ", "))
	}
	var ops []string
	for _, o := range p.Operations {
		ops = append(ops, o.Name)
	}
	if len(ops) > 0 {
		fmt.Fprintf(&b, " Processing: %s.", strings.Join(ops, "; "))
	}
	return b.String()
}
