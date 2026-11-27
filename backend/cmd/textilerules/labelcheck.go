package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/cross"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// The label check (D957): a second model reads each ruling's whole CROSS text
// and says which country or territory CBP's own holding names, without seeing
// either label. Its answer is compared with the regex label. It is a check of
// the labeller, not of the rule layer.
const labelCheckSystem = `You read a U.S. Customs (CBP) ruling about a textile or apparel good. Report which country, territory or insular possession CBP's own HOLDING names as the country of origin of the good the ruling decides.

Rules:
- Use CBP's holding sentence (for example "The country of origin of the pullover is Hong Kong."), not the facts, the requester's claim, a hypothetical or a scenario CBP rejects.
- Hong Kong, Macau and Taiwan are places of their own: never write China for them.
- If the ruling holds different origins for different goods, answer "multiple".
- If the origin depends on a scenario (for example "India when the fabric is woven in India and Pakistan when it is woven in Pakistan"), answer "scenario".
- If the ruling states no holding on origin, answer "none".
- Otherwise answer the one place's name (English short form, e.g. "China", "Hong Kong", "Vietnam", "South Korea").
Return JSON: {"country": "...", "holding_sentence": "<the sentence copied verbatim>"}.`

var labelCheckSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"country":          map[string]any{"type": "string"},
		"holding_sentence": map[string]any{"type": "string"},
	},
	"required":             []string{"country", "holding_sentence"},
	"additionalProperties": false,
}

// LabelCheckRow is one row of labelcheck-d957.jsonl.
type LabelCheckRow struct {
	Ruling   string  `json:"ruling"`
	Set      string  `json:"set"`
	Stratum  string  `json:"stratum"` // "changed" (label differs from the legacy label) or "unchanged"
	Label    string  `json:"label"`
	Legacy   string  `json:"legacy_label"`
	Model    string  `json:"model_country"`
	Sentence string  `json:"model_holding_sentence"`
	AgreeNew bool    `json:"agree_label"`
	AgreeOld bool    `json:"agree_legacy"`
	Error    string  `json:"error,omitempty"`
	CostUSD  float64 `json:"cost_usd"`
}

// pickSample orders ids by a hash of the id and keeps the first n: a
// deterministic sample that does not depend on the labels or their order.
func pickSample(ids []string, n int) []string {
	type kv struct{ id, h string }
	ks := make([]kv, len(ids))
	for i, id := range ids {
		s := sha256.Sum256([]byte("textile-labelcheck-v4:" + id))
		ks[i] = kv{id, hex.EncodeToString(s[:])}
	}
	sort.Slice(ks, func(i, j int) bool { return ks[i].h < ks[j].h })
	if n > len(ks) {
		n = len(ks)
	}
	out := make([]string, n)
	for i := range out {
		out[i] = ks[i].id
	}
	return out
}

// cmdLabelCheck asks the second model about every ruling whose label changed
// and a hash-ordered sample of the others, per set.
//
//	go run ./cmd/textilerules labelcheck -raw DIR,DIR -per-set 25
func cmdLabelCheck(args []string) error {
	fl := flag.NewFlagSet("labelcheck", flag.ExitOnError)
	raw := fl.String("raw", "", "comma-separated directories of raw CROSS JSON")
	sets := fl.String("sets", "dev,final,final2,older,pool", "sets to check")
	perSet := fl.Int("per-set", 25, "unchanged labels sampled per set (every changed label is checked)")
	model := fl.String("model", "claude-sonnet-5-5", "gateway model (not the extraction model)")
	workers := fl.Int("workers", 4, "parallel calls")
	out := fl.String("out", "../eval/textile102/labelcheck-d957.jsonl", "output file")
	_ = fl.Parse(args)
	base, key := os.Getenv("LITELLM_BASE_URL"), os.Getenv("LITELLM_API_KEY")
	if base == "" || key == "" {
		return errors.New("needs LITELLM_BASE_URL and LITELLM_API_KEY")
	}
	dirs := strings.Split(*raw, ",")
	gold, err := loadGold(false)
	if err != nil {
		return err
	}
	type job struct{ id, set, stratum, label, legacy string }
	var jobs []job
	for _, s := range strings.Split(*sets, ",") {
		ids, err := splitIDs(s)
		if err != nil {
			return err
		}
		var unchanged []string
		lab := map[string]string{}
		for _, id := range ids {
			l, err := relabelRuling(dirs, id)
			if err != nil {
				return err
			}
			lab[id] = l.Origin()
			if l.Origin() != gold[id].Origin {
				jobs = append(jobs, job{id, s, "changed", l.Origin(), gold[id].Origin})
			} else {
				unchanged = append(unchanged, id)
			}
		}
		for _, id := range pickSample(unchanged, *perSet) {
			jobs = append(jobs, job{id, s, "unchanged", lab[id], gold[id].Origin})
		}
	}
	ll := llm.NewLiteLLM(base, key, 180*time.Second)
	ll.SkipCache(true)
	rows := make([]LabelCheckRow, len(jobs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, *workers)
	for i, j := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			rows[i] = labelCheckOne(context.Background(), ll, *model, dirs, j.id, j.set, j.stratum, j.label, j.legacy)
		}()
	}
	wg.Wait()
	var buf []byte
	var cost float64
	for _, r := range rows {
		cost += r.CostUSD
		b, _ := json.Marshal(r)
		buf = append(buf, append(b, '\n')...)
	}
	if err := os.WriteFile(*out, buf, 0o644); err != nil {
		return err
	}
	fmt.Printf("%d rulings checked, $%.4f\n", len(rows), cost)
	return nil
}

func labelCheckOne(ctx context.Context, ll *llm.LiteLLM, model string, dirs []string, id, set, stratum, label, legacy string) LabelCheckRow {
	row := LabelCheckRow{Ruling: id, Set: set, Stratum: stratum, Label: label, Legacy: legacy}
	var text string
	for _, d := range dirs {
		if b, err := os.ReadFile(filepath.Join(d, id+".json")); err == nil {
			var r cross.Ruling
			if err := json.Unmarshal(b, &r); err == nil {
				text = r.Text
				break
			}
		}
	}
	if text == "" {
		row.Error = "no raw text"
		return row
	}
	t := &llm.CostTally{}
	cctx := llm.ContextWithCostTally(ctx, t)
	var resp llm.Response
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		resp, err = ll.Generate(cctx, llm.Request{
			Model: model, System: labelCheckSystem,
			Messages:        []llm.Message{{Role: llm.RoleUser, Content: text}},
			JSONSchema:      labelCheckSchema,
			SchemaName:      "textile_labelcheck",
			MaxOutputTokens: 1024,
		})
		if err == nil {
			break
		}
	}
	row.CostUSD = round4(t.USD())
	if err != nil {
		row.Error = "model call failed: " + err.Error()
		return row
	}
	var x struct {
		Country  string `json:"country"`
		Sentence string `json:"holding_sentence"`
	}
	if err := resp.DecodeJSON(&x); err != nil {
		row.Error = "bad JSON: " + err.Error()
		return row
	}
	row.Model, row.Sentence = strings.TrimSpace(x.Country), strings.TrimSpace(x.Sentence)
	same := func(a, b string) bool {
		if b == "" { // the label says no single origin: the reader must say so too
			switch strings.ToLower(a) {
			case "multiple", "scenario", "none":
				return true
			}
			return false
		}
		return a != "" && domain.NormalizeCountry(a) == domain.NormalizeCountry(b)
	}
	row.AgreeNew, row.AgreeOld = same(row.Model, label), same(row.Model, legacy)
	return row
}
