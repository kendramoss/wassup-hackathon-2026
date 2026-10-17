// Package nearmiss explains a plan move whose precedent check landed just
// under the plan bar: what differs between the move and its supporting
// ruling on the point CBP relied on, and what change to the setup would make
// the two match (D81). It is advice only. It never changes the verdict, the
// bar or whether a plan is shown.
package nearmiss

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/blob"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/verifier"
)

// PromptVersion is recorded with every answer.
const PromptVersion = "near-miss-v1"

// System is the near-miss prompt (new, written before any measurement).
const System = `You help a manufacturer understand why a proposed production change is only close to a U.S. customs precedent, not on it. You get the proposed setup's facts and one earlier CBP ruling (its facts and CBP's analysis and holding) that supports the change, but only with moderate confidence.

1. Find the point CBP relied on in that ruling to decide origin: which component, operation or location was decisive.
2. DIFFERENCE: in one or two sentences, say how the proposed setup differs from the ruling on that point. Name the ruling and the concrete fact on each side (for example "In N325810 the PCBA was also made in Thailand; in your setup it stays in China."). If the setups match on that point, say what other fact in the ruling differs.
3. WOULD_MATCH: in one sentence, the change to the proposed setup that would make it match the ruling on that point. Only changes to where components are made or operations are performed. Say nothing about duty, cost or likelihood.

Use only the two texts given. Do not cite any other ruling. Return ONLY JSON.`

// Schema constrains the answer.
var Schema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"difference":  map[string]any{"type": "string"},
		"would_match": map[string]any{"type": "string"},
	},
	"required":             []string{"difference", "would_match"},
	"additionalProperties": false,
}

// RenderUser is the user message Advise sends.
func RenderUser(facts, ruling, rulingText string) string {
	return fmt.Sprintf("PROPOSED SETUP:\n%s\n\nEARLIER RULING %s (supports the change, moderate confidence):\n%s",
		corpus.Truncate(facts, 8000), ruling, corpus.Truncate(rulingText, 6000))
}

// Advisor writes near-miss advice.
type Advisor struct {
	model     llm.Model
	modelID   string
	search    verifier.Searcher
	maxTokens int
}

// New returns an advisor that reads rulings through search, the verifier's
// own view of the ruling database.
func New(m llm.Model, modelID string, search verifier.Searcher, maxTokens int) *Advisor {
	return &Advisor{model: m, modelID: modelID, search: search, maxTokens: maxTokens}
}

// Task is one near miss: a move's facts and its supporting ruling.
type Task struct {
	Facts   string
	Ruling  string
	Before  time.Time
	Exclude []string
}

// Adviser is anything that advises on a near miss (Advisor, Cache).
type Adviser interface {
	Advise(ctx context.Context, t Task) (domain.NearMiss, error)
}

type answer struct {
	Difference string `json:"difference"`
	WouldMatch string `json:"would_match"`
}

// Advise reads the ruling (only if dated before t.Before and not excluded)
// and makes one model call.
func (a *Advisor) Advise(ctx context.Context, t Task) (domain.NearMiss, error) {
	if !domain.ValidRulingID(t.Ruling) {
		return domain.NearMiss{}, fmt.Errorf("nearmiss: %q is not a ruling number", t.Ruling)
	}
	exclude := map[string]bool{}
	for _, id := range t.Exclude {
		exclude[id] = true
	}
	text, err := a.search.Read(ctx, t.Ruling, t.Before, exclude)
	if err != nil {
		return domain.NearMiss{}, fmt.Errorf("nearmiss: read %s: %w", t.Ruling, err)
	}
	if strings.HasPrefix(text, "NOT FOUND") {
		return domain.NearMiss{}, fmt.Errorf("nearmiss: ruling %s is not available before %s", t.Ruling, t.Before.Format(time.DateOnly))
	}
	resp, err := a.model.Generate(ctx, llm.Request{Model: a.modelID, System: System,
		Messages:   []llm.Message{{Role: llm.RoleUser, Content: RenderUser(t.Facts, t.Ruling, text)}},
		JSONSchema: Schema, SchemaName: "near_miss", MaxOutputTokens: a.maxTokens})
	if err != nil {
		return domain.NearMiss{}, fmt.Errorf("nearmiss: %w", err)
	}
	var ans answer
	if err := resp.DecodeJSON(&ans); err != nil {
		return domain.NearMiss{}, fmt.Errorf("nearmiss: %w", err)
	}
	ans.Difference, ans.WouldMatch = strings.TrimSpace(ans.Difference), strings.TrimSpace(ans.WouldMatch)
	if ans.Difference == "" || ans.WouldMatch == "" {
		return domain.NearMiss{}, fmt.Errorf("nearmiss: %w: empty answer", llm.ErrBadResponse)
	}
	return domain.NearMiss{Difference: ans.Difference, WouldMatch: ans.WouldMatch, Ruling: t.Ruling,
		Trace: domain.Trace{Provider: resp.Provider, ModelID: resp.ModelID, PromptVersion: PromptVersion,
			InputTokens: resp.Usage.InputTokens, OutputTokens: resp.Usage.OutputTokens, LatencyMS: resp.Latency.Milliseconds()}}, nil
}

// MapPromptVersion is the group-mapping prompt (D85).
const MapPromptVersion = "near-miss-map-v1"

// MapSystem asks which operation groups a piece of advice names. It is the
// fallback when no group name, component or operation appears in the text.
const MapSystem = `You map a short piece of manufacturing advice to a fixed list of operation groups. The advice says what to make or do in a different place. Return the ids of the groups from the list whose components or operations the advice asks to move. Use only ids from the list. If the advice names none of them, return an empty list. Return ONLY JSON.`

// MapSchema constrains the mapping answer.
var MapSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"groups": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
	},
	"required":             []string{"groups"},
	"additionalProperties": false,
}

// RenderMap is the user message MapGroups sends.
func RenderMap(advice string, groups []domain.OperationGroup) string {
	var b strings.Builder
	b.WriteString("ADVICE:\n" + corpus.Truncate(advice, 1000) + "\n\nGROUPS:\n")
	for _, g := range groups {
		fmt.Fprintf(&b, "- %s: %s", g.ID, g.Name)
		if len(g.Components) > 0 {
			b.WriteString("; components: " + strings.Join(g.Components, ", "))
		}
		if len(g.Operations) > 0 {
			b.WriteString("; operations: " + strings.Join(g.Operations, "; "))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// Mapper names the groups advice asks to move with one small model call.
type Mapper struct {
	model   llm.Model
	modelID string
}

// NewMapper returns a mapper.
func NewMapper(m llm.Model, modelID string) *Mapper { return &Mapper{model: m, modelID: modelID} }

// MapGroups returns the ids of groups the advice names, only ids from groups.
func (m *Mapper) MapGroups(ctx context.Context, advice string, groups []domain.OperationGroup) ([]string, llm.Usage, error) {
	resp, err := m.model.Generate(ctx, llm.Request{Model: m.modelID, System: MapSystem,
		Messages:   []llm.Message{{Role: llm.RoleUser, Content: RenderMap(advice, groups)}},
		JSONSchema: MapSchema, SchemaName: "near_miss_groups", MaxOutputTokens: 300})
	if err != nil {
		return nil, llm.Usage{}, fmt.Errorf("nearmiss: map: %w", err)
	}
	var ans struct {
		Groups []string `json:"groups"`
	}
	if err := resp.DecodeJSON(&ans); err != nil {
		return nil, resp.Usage, fmt.Errorf("nearmiss: map: %w", err)
	}
	known := map[string]bool{}
	for _, g := range groups {
		known[g.ID] = true
	}
	var ids []string
	for _, id := range ans.Groups {
		if known[id] && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids, resp.Usage, nil
}

// Cache stores advice so the same move and ruling get the same text without
// another model call, in the same store as the verdicts (D50).
type Cache struct {
	inner Adviser
	blobs blob.Store
	model string
}

// NewCache wraps inner with a cache in blobs, keyed by prompt version,
// model, facts, ruling, date and exclusions.
func NewCache(inner Adviser, blobs blob.Store, model string) *Cache {
	return &Cache{inner: inner, blobs: blobs, model: model}
}

func (c *Cache) key(t Task) string {
	ex := slices.Clone(t.Exclude)
	slices.Sort(ex)
	b, _ := json.Marshal(struct {
		Prompt, Model, Facts, Ruling, Before string
		Exclude                              []string
	}{PromptVersion, c.model, t.Facts, t.Ruling, t.Before.Format(time.DateOnly), ex})
	h := sha256.Sum256(b)
	return "nearmiss-" + hex.EncodeToString(h[:])
}

// Advise returns the stored advice for t, or asks inner and stores it. A
// cached answer reports no tokens or latency, and Trace.Cached.
func (c *Cache) Advise(ctx context.Context, t Task) (domain.NearMiss, error) {
	key := c.key(t)
	if b, ok, err := c.blobs.Get(ctx, key); err != nil {
		return domain.NearMiss{}, fmt.Errorf("nearmiss: read cache: %w", err)
	} else if ok {
		var n domain.NearMiss
		if json.Unmarshal(b, &n) == nil && n.Difference != "" {
			n.Trace.InputTokens, n.Trace.OutputTokens, n.Trace.LatencyMS, n.Trace.Cached = 0, 0, 0, true
			return n, nil
		}
	}
	n, err := c.inner.Advise(ctx, t)
	if err != nil {
		return n, err
	}
	if b, err := json.Marshal(n); err == nil {
		_ = c.blobs.Put(ctx, key, b) // best effort
	}
	return n, nil
}
