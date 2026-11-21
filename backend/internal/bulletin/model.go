package bulletin

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// ReadModel is the model D400 pre-registers for flagged notices.
const ReadModel = "gemini-3.5-flash"

// readPrompt is the fallback instruction (D400; frozen).
const readPrompt = "You read one CBP Customs Bulletin notice about ruling letters (an excerpt: the notice header, then each attached ruling's number, subject line and the sentences that revoke or modify). " +
	"List every ruling letter that this notice revokes or modifies, or proposes to revoke or modify, including rulings the attached letter revokes or modifies by operation of law. " +
	"Do not list the attached letter that makes the change, and do not list rulings that are only cited. " +
	"Write each ruling number exactly as it appears in the excerpt, without the NY, HQ or PC prefix. kind is \"revoked\" or \"modified\"."

var readSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"rulings": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"ruling": map[string]any{"type": "string"},
					"kind":   map[string]any{"type": "string", "enum": []string{"revoked", "modified"}},
				},
				"required": []string{"ruling", "kind"},
			},
		},
	},
	"required": []string{"rulings"},
}

// Key names a notice: volume, number and printed page.
func (n Notice) Key() string { return fmt.Sprintf("%d/%d/p%d", n.Volume, n.Number, n.Page) }

// ModelRuling is one ruling a model read names.
type ModelRuling struct {
	Ruling string `json:"ruling"`
	Kind   string `json:"kind"`
}

// Read is one committed model read of a flagged notice.
type Read struct {
	Notice  string        `json:"notice"`
	Model   string        `json:"model"`
	ModelID string        `json:"model_id,omitempty"`
	Rulings []ModelRuling `json:"rulings"`
	Error   string        `json:"error,omitempty"`
	Tokens  int           `json:"tokens"`
	CostUSD float64       `json:"cost_usd"`
}

// ReadNotice asks the model once. Errors are recorded in the read, not
// returned: a failed read drops the notice's edges (D400).
func ReadNotice(ctx context.Context, m llm.Model, model string, n Notice) Read {
	rd := Read{Notice: n.Key(), Model: model}
	resp, err := m.Generate(ctx, llm.Request{
		Model:           model,
		System:          readPrompt,
		Messages:        []llm.Message{{Role: llm.RoleUser, Content: n.Excerpt}},
		JSONSchema:      readSchema,
		SchemaName:      "notice_rulings",
		MaxOutputTokens: 2048,
	})
	if err != nil {
		rd.Error = "model call failed"
		return rd
	}
	rd.ModelID, rd.Tokens = resp.ModelID, resp.Usage.TotalTokens
	var out struct {
		Rulings []ModelRuling `json:"rulings"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(resp.Text)), &out); err != nil {
		rd.Error = "unparseable reply"
		return rd
	}
	rd.Rulings = out.Rulings
	return rd
}

// LoadReads reads committed model reads, keyed by notice.
func LoadReads(path string) (map[string]Read, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := map[string]Read{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 1<<20), 8<<20)
	for sc.Scan() {
		if len(bytes.TrimSpace(sc.Bytes())) == 0 {
			continue
		}
		var r Read
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		out[r.Notice] = r
	}
	return out, sc.Err()
}

// Resolve applies D400's rule to a flagged notice: keep a parsed change the
// model also names; add one only the model names when its number is in the
// excerpt (dated and attributed like the notice's other changes). A failed
// read keeps nothing.
func Resolve(n Notice, r Result, rd Read) []Change {
	if rd.Error != "" {
		return nil
	}
	named := map[string]string{}
	for _, m := range rd.Rulings {
		named[strings.TrimSpace(m.Ruling)] = m.Kind
	}
	var out []Change
	seen := map[string]bool{}
	for _, c := range r.Changes {
		if _, ok := named[c.Ruling]; ok {
			out = append(out, c)
			seen[c.Ruling] = true
		}
	}
	by := ""
	for _, c := range r.Changes {
		if by == "" {
			by = c.By
		} else if by != c.By {
			by = "" // several changing letters: cannot attribute a model-only ruling
			break
		}
	}
	ids := idsIn(n.Excerpt)
	var extra []string
	for id := range named {
		if !seen[id] && by != "" && id != by && slices.Contains(ids, id) {
			extra = append(extra, id)
		}
	}
	slices.Sort(extra)
	for _, id := range extra {
		c := Change{Ruling: id, By: by, Kind: named[id], Status: r.Status, Quote: "model read (D400)"}
		if r.Status == "final" {
			c.Effective = r.Effective
		} else {
			c.CommentsBy = r.CommentsBy
		}
		out = append(out, c)
	}
	return out
}
