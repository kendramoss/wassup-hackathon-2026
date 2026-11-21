package hts

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
)

// Per-part suggestions (D380). The 19 CFR 102.20 rule check (D360, D363)
// needs an HTS code for the good and for every part, and users rarely have
// the parts' codes. For each part the part index (PartIndex) gives the codes
// earlier CBP rulings stated for similar parts; one model call for the whole
// product ranks them and, when no ruling fits, gives its own code, labelled
// "model". Nothing is filled in: the user accepts a code per part.

// PartsPromptVersion is recorded with every per-part suggestion.
const PartsPromptVersion = "parts-hts-suggest-v1"

// UnverifiedNote goes with every per-part suggestion.
const UnverifiedNote = "unverified suggestions — check every code with your customs broker before you rely on it"

// Limits of one request.
const (
	// MaxParts is the most parts suggested in one request.
	MaxParts = 20
	// partHits is how many part-index sentences are read per part, and
	// partPool how many distinct codes from them go to the model.
	partHits = 12
	partPool = 4
	// partRefs is how many rulings are listed per code.
	partRefs = 3
	// descRunes is how much of the product description goes to the model.
	descRunes = 1500
)

// Where a per-part candidate comes from.
const (
	// SourceRulings: an earlier CBP ruling stated this code for a similar
	// part (Supporting lists them).
	SourceRulings = "rulings"
	// SourceModel: the model's own reading of the HTSUS; no ruling was
	// found that states it for a similar part.
	SourceModel = "model"
)

// PartRef is a ruling sentence that classified a similar part.
type PartRef struct {
	ID       string `json:"id"`
	Date     string `json:"date"`
	Part     string `json:"part,omitempty"`
	Sentence string `json:"sentence,omitempty"`
	Subject  string `json:"subject,omitempty"`
	URL      string `json:"url"`
}

// PartCandidate is one suggested code for a part (or the good).
type PartCandidate struct {
	HTS        string    `json:"hts"`
	Source     string    `json:"source"`
	Reason     string    `json:"reason"`
	Supporting []PartRef `json:"supporting_rulings"`
	Cited      []string  `json:"cited"`
}

// PartSuggestion is the suggestion for the part at Index in the request.
type PartSuggestion struct {
	Index      int             `json:"index"`
	Name       string          `json:"name"`
	Candidates []PartCandidate `json:"candidates"`
	// Cached is true when the answer came from the cache (no model call).
	Cached bool `json:"cached"`
}

// GoodSuggestion is the suggestion for the finished good, made only when the
// product has no HTS code.
type GoodSuggestion struct {
	Candidates []PartCandidate `json:"candidates"`
	// Source is where the good's ruling candidates came from (cross, index).
	Source string `json:"source"`
}

// PartsResult is a per-part suggestion.
type PartsResult struct {
	Parts []PartSuggestion `json:"parts"`
	Good  *GoodSuggestion  `json:"good"`
	Note  string           `json:"note"`
	// ReasonsUnavailable is true when the model call failed: the ruling
	// candidates are returned without reasons and no model codes are given.
	ReasonsUnavailable bool          `json:"reasons_unavailable"`
	Trace              *domain.Trace `json:"trace"`
}

// PartsOptions bound a suggestion. Before and Exclude are for the
// evaluation (D381): only rulings dated before Before, never Exclude.
type PartsOptions struct {
	Good    bool // also suggest the good's code
	Before  time.Time
	Exclude string
	NoCache bool
}

// PartsSuggester suggests codes for a product's parts.
type PartsSuggester struct {
	model    llm.Model
	modelID  string
	entries  []PartEntry
	index    *retrieval.BM25
	products *Suggester // the good's candidates; nil: no good suggestion
	current  Current

	mu    sync.Mutex
	cache map[string]cachedPart
	now   func() time.Time
}

type cachedPart struct {
	cands []PartCandidate
	at    time.Time
}

// Cache bounds: answers are kept a day, at most cacheMax of them.
const (
	cacheTTL = 24 * time.Hour
	cacheMax = 2000
)

// NewParts builds a per-part suggester over the part index. products, when
// set, suggests the good's code; current, when set, drops model codes of 8
// digits that are not in today's HTS.
func NewParts(m llm.Model, modelID string, x *PartIndex, products *Suggester, current Current) *PartsSuggester {
	p := &PartsSuggester{model: m, modelID: modelID, entries: x.Entries, products: products, current: current,
		cache: map[string]cachedPart{}, now: time.Now}
	docs := make([]domain.Ruling, len(x.Entries))
	for i, e := range x.Entries {
		d, _ := time.Parse(time.DateOnly, e.Date)
		docs[i] = domain.Ruling{ID: fmt.Sprint(i), Date: d, Subject: e.Ruling, Facts: e.Part + " " + e.Part + " " + e.Sentence}
	}
	p.index = retrieval.NewBM25(docs, func(r domain.Ruling) string { return r.Facts })
	return p
}

// Coverage is the number of classifying sentences and rulings indexed.
func (p *PartsSuggester) Coverage() (sentences, rulings int) {
	seen := map[string]bool{}
	for _, e := range p.entries {
		seen[e.Ruling] = true
	}
	return len(p.entries), len(seen)
}

// CacheKey is how a part's answer is cached: the part's words and the
// product's name, lower case, punctuation dropped. The product name is in
// the key because a part's code depends on what it is part of ("housing").
func CacheKey(part, product string) string {
	return normWords(part) + "\x00" + normWords(product)
}

func normWords(s string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }), " ")
}

// partCandidates groups the part-index sentences most like the part by code
// (summed BM25 score) and keeps the best partPool.
func (p *PartsSuggester) partCandidates(name string, before time.Time, exclude string) []PartCandidate {
	hits := p.index.Top(name, partHits, func(r domain.Ruling) bool {
		return r.Subject != exclude && (before.IsZero() || r.Date.Before(before))
	})
	score := map[string]float64{}
	refs := map[string][]PartRef{}
	var order []string
	for _, h := range hits {
		if h.Score <= 0 {
			continue
		}
		var i int
		fmt.Sscan(h.Ruling.ID, &i)
		e := p.entries[i]
		if _, ok := score[e.HTS]; !ok {
			order = append(order, e.HTS)
		}
		score[e.HTS] += h.Score
		if len(refs[e.HTS]) < partRefs && !slices.ContainsFunc(refs[e.HTS], func(r PartRef) bool { return r.ID == e.Ruling }) {
			refs[e.HTS] = append(refs[e.HTS], PartRef{ID: e.Ruling, Date: e.Date, Part: e.Part, Sentence: e.Sentence, URL: domain.RulingURL(e.Ruling)})
		}
	}
	sort.SliceStable(order, func(i, j int) bool { return score[order[i]] > score[order[j]] })
	if len(order) > partPool {
		order = order[:partPool]
	}
	out := make([]PartCandidate, 0, len(order))
	for _, code := range order {
		out = append(out, PartCandidate{HTS: code, Source: SourceRulings, Supporting: refs[code], Cited: []string{}})
	}
	return out
}

// item is one thing the model is asked about: a part or the good.
type item struct {
	key   string // "p<i>" or "good"
	name  string
	made  string
	pool  []PartCandidate
	parts []int // request indexes that share this answer
	cache string
}

const partsSystem = `You help an importer find the U.S. tariff (HTSUS) codes of the parts of a product, so that a 19 CFR 102.20 tariff-shift rule can be checked. You are given the product and a list of items: its parts and, when key is "good", the finished product itself. A part item may list sentences of earlier CBP rulings that classified a similar thing, with the code CBP stated. The good may list candidate subheadings from CBP rulings on similar products.

For each item return up to 3 codes, best first. Classify a part as imported on its own, not as the finished product. Prefer a code that one of the item's listed rulings states for the same kind of thing: copy that code exactly as listed and cite those rulings. A ruling about a different kind of thing that only shares a word is not support. When no listed ruling fits, give the code you judge most likely from the HTSUS headings and the section and chapter notes (for example parts of machines of chapter 84 or 85), as NNNN.NN or NNNN.NN.NN, with cited left empty. Never give chapter 98 or 99 codes. For each code write one short plain sentence on why. If you cannot tell what an item is, return no codes for it. Do not state duty rates. This is a suggestion for a licensed broker to confirm, not a classification.`

type partsAnswer struct {
	Items []struct {
		Key        string `json:"key"`
		Candidates []struct {
			HTS    string   `json:"hts"`
			Reason string   `json:"reason"`
			Cited  []string `json:"cited"`
		} `json:"candidates"`
	} `json:"items"`
}

func partsSchema(items []*item) map[string]any {
	var keys, ids []string
	for _, it := range items {
		keys = append(keys, it.key)
		for _, c := range it.pool {
			for _, r := range c.Supporting {
				if !slices.Contains(ids, r.ID) {
					ids = append(ids, r.ID)
				}
			}
		}
	}
	cited := map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
	if len(ids) > 0 {
		cited["items"] = map[string]any{"type": "string", "enum": ids}
	} else {
		cited["maxItems"] = 0
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"items": map[string]any{"type": "array", "items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"key": map[string]any{"type": "string", "enum": keys},
					"candidates": map[string]any{"type": "array", "maxItems": MaxCandidates, "items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"hts":    map[string]any{"type": "string"},
							"reason": map[string]any{"type": "string"},
							"cited":  cited,
						},
						"required": []string{"hts", "reason", "cited"},
					}},
				},
				"required": []string{"key", "candidates"},
			}},
		},
		"required": []string{"items"},
	}
}

// Suggest returns up to MaxCandidates codes for each named part of the
// product (and for the good when opt.Good), from one model call. Parts with
// an empty name are skipped. If the model call fails, the ruling candidates
// are returned without reasons (ReasonsUnavailable) and the error is returned
// too, for logging.
func (p *PartsSuggester) Suggest(ctx context.Context, prod domain.Product, opt PartsOptions) (PartsResult, error) {
	res := PartsResult{Note: UnverifiedNote, Parts: []PartSuggestion{}}
	useCache := !opt.NoCache && opt.Before.IsZero()
	byKey := map[string]*item{}
	var items []*item
	for i, c := range prod.Components {
		name := strings.TrimSpace(c.Name)
		if name == "" {
			continue
		}
		res.Parts = append(res.Parts, PartSuggestion{Index: i, Name: name, Candidates: []PartCandidate{}})
		ck := CacheKey(name, prod.Name)
		if useCache {
			if cs, ok := p.cached(ck); ok {
				res.Parts[len(res.Parts)-1].Candidates, res.Parts[len(res.Parts)-1].Cached = cs, true
				continue
			}
		}
		if it, ok := byKey[ck]; ok {
			it.parts = append(it.parts, i)
			continue
		}
		it := &item{key: fmt.Sprintf("p%d", i), name: name, made: strings.TrimSpace(c.MadeIn), parts: []int{i}, cache: ck,
			pool: p.partCandidates(name, opt.Before, opt.Exclude)}
		byKey[ck] = it
		items = append(items, it)
	}
	if opt.Good && p.products != nil {
		facts := ProductText(prod)
		cs, src := p.products.pool(ctx, facts, prod.Name, opt.Before, opt.Exclude)
		good := &item{key: "good", name: strings.TrimSpace(prod.Name)}
		for _, c := range cs {
			pc := PartCandidate{HTS: c.HTS, Source: SourceRulings, Cited: []string{}}
			for _, r := range c.Supporting {
				pc.Supporting = append(pc.Supporting, PartRef{ID: r.ID, Date: r.Date, Subject: r.Subject, URL: r.URL})
			}
			good.pool = append(good.pool, pc)
		}
		res.Good = &GoodSuggestion{Candidates: []PartCandidate{}, Source: src}
		items = append(items, good)
	}
	if len(items) == 0 {
		return res, nil
	}
	resp, err := p.model.Generate(ctx, llm.Request{Model: p.modelID, System: partsSystem,
		Messages:   []llm.Message{{Role: llm.RoleUser, Content: partsPrompt(prod, items)}},
		JSONSchema: partsSchema(items), SchemaName: "parts_hts_suggestion", MaxOutputTokens: min(8192, 1024+400*len(items))})
	var a partsAnswer
	if err == nil {
		err = resp.DecodeJSON(&a)
	}
	if err != nil {
		res.ReasonsUnavailable = true
		for _, it := range items {
			p.place(&res, it, trimCands(slices.Clone(it.pool)), false)
		}
		return res, fmt.Errorf("hts: parts: %w", err)
	}
	res.Trace = &domain.Trace{Provider: resp.Provider, ModelID: resp.ModelID, PromptVersion: PartsPromptVersion,
		InputTokens: resp.Usage.InputTokens, OutputTokens: resp.Usage.OutputTokens, LatencyMS: resp.Latency.Milliseconds()}
	valid := p.checkCurrent(ctx, items, a)
	for _, it := range items {
		var cs []PartCandidate
		for _, ai := range a.Items {
			if ai.Key != it.key {
				continue
			}
			for _, ac := range ai.Candidates {
				cs = append(cs, PartCandidate{HTS: ac.HTS, Reason: ac.Reason, Cited: ac.Cited})
			}
			break
		}
		cs = settle(it, cs, valid)
		p.place(&res, it, cs, useCache && it.key != "good")
	}
	return res, nil
}

// place puts an item's candidates on its parts (or the good) and caches them.
func (p *PartsSuggester) place(res *PartsResult, it *item, cs []PartCandidate, cache bool) {
	if cs == nil {
		cs = []PartCandidate{}
	}
	if it.key == "good" {
		res.Good.Candidates = cs
		return
	}
	for i := range res.Parts {
		if slices.Contains(it.parts, res.Parts[i].Index) && !res.Parts[i].Cached {
			res.Parts[i].Candidates = cs
		}
	}
	if cache {
		p.store(it.cache, cs)
	}
}

// settle checks the model's codes: well formed, not chapter 98/99, not
// repeated; a code equal to one of the item's ruling codes is "rulings"
// with its supporting rulings and only those citations; any other code is
// "model", with no rulings, and (at 8 digits) must be in today's HTS when
// current is set. At most MaxCandidates are kept.
func settle(it *item, in []PartCandidate, valid map[string]bool) []PartCandidate {
	var out []PartCandidate
	for _, c := range in {
		code := FormatCode(c.HTS)
		if code == "" || strings.HasPrefix(code, "98") || strings.HasPrefix(code, "99") ||
			slices.ContainsFunc(out, func(x PartCandidate) bool { return x.HTS == code }) {
			continue
		}
		pc := PartCandidate{HTS: code, Source: SourceModel, Reason: strings.TrimSpace(c.Reason), Cited: []string{}, Supporting: []PartRef{}}
		if i := slices.IndexFunc(it.pool, func(x PartCandidate) bool { return x.HTS == code }); i >= 0 {
			pc.Source, pc.Supporting = SourceRulings, it.pool[i].Supporting
			for _, id := range c.Cited {
				if slices.ContainsFunc(pc.Supporting, func(r PartRef) bool { return r.ID == id }) && !slices.Contains(pc.Cited, id) {
					pc.Cited = append(pc.Cited, id)
				}
			}
		} else if ok, checked := valid[code]; checked && !ok {
			continue // not a subheading in today's HTS
		}
		out = append(out, pc)
		if len(out) == MaxCandidates {
			break
		}
	}
	return out
}

// checkCurrent asks current, in parallel (at most 8 at a time), about every
// 8-digit code the model gave that no ruling listed; the answer maps each
// checked code to whether it is in today's HTS. nil current checks nothing.
func (p *PartsSuggester) checkCurrent(ctx context.Context, items []*item, a partsAnswer) map[string]bool {
	out := map[string]bool{}
	if p.current == nil {
		return out
	}
	pooled := map[string]bool{}
	for _, it := range items {
		for _, c := range it.pool {
			pooled[c.HTS] = true
		}
	}
	var codes []string
	for _, ai := range a.Items {
		for _, c := range ai.Candidates {
			code := FormatCode(c.HTS)
			if len(digitsOf(code)) == 8 && !pooled[code] && !slices.Contains(codes, code) {
				codes = append(codes, code)
			}
		}
	}
	ok := make([]bool, len(codes))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for i, code := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			ok[i] = p.current(ctx, code)
		}()
	}
	wg.Wait()
	for i, code := range codes {
		out[code] = ok[i]
	}
	return out
}

func trimCands(cs []PartCandidate) []PartCandidate {
	if len(cs) > MaxCandidates {
		cs = cs[:MaxCandidates]
	}
	return cs
}

func partsPrompt(prod domain.Product, items []*item) string {
	var b strings.Builder
	fmt.Fprintf(&b, "PRODUCT: %s\n", strings.TrimSpace(prod.Name))
	if d := strings.TrimSpace(prod.Description); d != "" {
		fmt.Fprintf(&b, "DESCRIPTION: %s\n", strings.Join(strings.Fields(corpus.Truncate(d, descRunes)), " "))
	}
	b.WriteString("\n")
	for _, it := range items {
		if it.key == "good" {
			fmt.Fprintf(&b, "ITEM good: the finished product, %q\n", it.name)
			if len(it.pool) == 0 {
				b.WriteString("  No candidate subheadings from rulings.\n")
			}
			for _, c := range it.pool {
				fmt.Fprintf(&b, "  - %s:", c.HTS)
				for _, r := range c.Supporting {
					fmt.Fprintf(&b, " %s (%s) %s;", r.ID, r.Date, r.Subject)
				}
				b.WriteString("\n")
			}
			b.WriteString("\n")
			continue
		}
		fmt.Fprintf(&b, "ITEM %s: part %q", it.key, it.name)
		if it.made != "" {
			fmt.Fprintf(&b, " (made in %s)", it.made)
		}
		b.WriteString("\n")
		if len(it.pool) == 0 {
			b.WriteString("  No earlier ruling in the index classifies a similar thing.\n")
		}
		for _, c := range it.pool {
			for _, r := range c.Supporting {
				fmt.Fprintf(&b, "  - %s (%s) classified %q in %s: %s\n", r.ID, r.Date, r.Part, c.HTS, r.Sentence)
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (p *PartsSuggester) cached(key string) ([]PartCandidate, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	c, ok := p.cache[key]
	if !ok || p.now().Sub(c.at) > cacheTTL {
		return nil, false
	}
	return c.cands, true
}

func (p *PartsSuggester) store(key string, cs []PartCandidate) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	if len(p.cache) >= cacheMax {
		for k, c := range p.cache {
			if now.Sub(c.at) > cacheTTL {
				delete(p.cache, k)
			}
		}
	}
	if len(p.cache) >= cacheMax {
		oldest, at := "", now
		for k, c := range p.cache {
			if !c.at.After(at) {
				oldest, at = k, c.at
			}
		}
		delete(p.cache, oldest)
	}
	p.cache[key] = cachedPart{cands: cs, at: now}
}
