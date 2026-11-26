package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jpcorpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jpengine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jprules"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llmbudget"
)

// maxOutputTokens is the output cap the engine sends with every request
// (jpengine.Predict); the plan prices it in full for every call.
const maxOutputTokens = 8192

var errPlanOnly = errors.New("plan only: no request is sent")

// planModel counts the requests the engine would send and measures their
// exact bodies. It never contacts anything: the engine's own answer handling
// is cut short by errPlanOnly.
type planModel struct {
	body   *llm.LiteLLM // builds the body only; its address is never used
	sizes  []int
	hashes []string
}

func newPlanModel() *planModel {
	g := llm.NewLiteLLM("https://plan.invalid", "unused", time.Second)
	g.SkipCache(true) // the real run skips the gateway cache too
	return &planModel{body: g}
}

func (p *planModel) Provider() string { return "plan" }

func (p *planModel) Generate(_ context.Context, req llm.Request) (llm.Response, error) {
	b, err := p.body.RequestBody(req)
	if err != nil {
		return llm.Response{}, err
	}
	if req.MaxOutputTokens != maxOutputTokens {
		return llm.Response{}, fmt.Errorf("request output cap %d is not the fixed %d", req.MaxOutputTokens, maxOutputTokens)
	}
	p.sizes = append(p.sizes, len(b))
	p.hashes = append(p.hashes, fmt.Sprintf("%x", sha256.Sum256(b)))
	return llm.Response{}, errPlanOnly
}

// planReport is the offline count of one split. It holds no model output and
// no label-dependent number.
type planReport struct {
	Split               string            `json:"split"`
	Cases               int               `json:"cases"`
	Calls               int               `json:"eligible_requests"`
	AbstainedBeforeCall int               `json:"abstained_before_request"`
	RequestBytes        int               `json:"request_bytes_total"`
	MaxRequestBytes     int               `json:"request_bytes_max"`
	MinRequestBytes     int               `json:"request_bytes_min"`
	InputTokensUpper    int               `json:"input_tokens_upper_bound"`
	OutputTokensCap     int               `json:"output_tokens_cap_total"`
	MaxOutputPerRequest int               `json:"max_output_tokens_per_request"`
	RequestsSHA256      string            `json:"requests_sha256"`
	Model               string            `json:"model"`
	PromptVersion       string            `json:"prompt_version"`
	K                   int               `json:"precedents_k"`
	Files               map[string]string `json:"file_sha256"`
	GitRevision         string            `json:"git_revision"`
	CodeDirty           bool              `json:"code_dirty"`
	Basis               string            `json:"basis"`
	WorstCaseUSD        *float64          `json:"worst_case_usd,omitempty"`
	Rates               *llmbudget.Rates  `json:"rates,omitempty"`

	sizes  []int
	hashes []string
}

const planBasis = "A token is at least one byte, so request bytes bound input tokens from above. " +
	"Output is priced at the full 8192-token cap on every request. No retries. " +
	"Request bodies are measured offline with a counting fake model."

// worstCase prices every eligible request at its byte bound plus the output cap.
func (r planReport) worstCase(rates llmbudget.Rates) float64 {
	var total float64
	for _, n := range r.sizes {
		total += rates.WorstCase(n, maxOutputTokens)
	}
	return total
}

func planItems(split string, items []candidateInput, pool *jpcorpus.Pool, rules *jprules.Table) (planReport, error) {
	m := newPlanModel()
	eng := jpengine.New(m, jpengine.DefaultModel, pool, rules, jpengine.DefaultK)
	r := planReport{Split: split, Cases: len(items), Model: jpengine.DefaultModel, PromptVersion: jpengine.PromptVersion,
		K: jpengine.DefaultK, MaxOutputPerRequest: maxOutputTokens, Basis: planBasis, GitRevision: revision(), CodeDirty: codeDirty()}
	var lines []string
	for _, item := range items { // in order, one at a time: a stable hash
		before := len(m.sizes)
		_, err := eng.Predict(context.Background(), item.caseValue)
		if err != nil && !errors.Is(err, errPlanOnly) {
			return planReport{}, fmt.Errorf("planning %s: %w", item.label.ID, err)
		}
		if n := len(m.sizes) - before; n > 1 {
			return planReport{}, fmt.Errorf("case %s sent %d requests; the protocol allows one", item.label.ID, n)
		} else if n == 1 {
			lines = append(lines, fmt.Sprintf("%s\t%d\t%s", item.label.ID, m.sizes[before], m.hashes[before]))
		}
	}
	r.sizes, r.hashes, r.Calls = m.sizes, m.hashes, len(m.sizes)
	r.AbstainedBeforeCall = r.Cases - r.Calls
	for i, n := range m.sizes {
		r.RequestBytes += n
		if i == 0 || n > r.MaxRequestBytes {
			r.MaxRequestBytes = n
		}
		if i == 0 || n < r.MinRequestBytes {
			r.MinRequestBytes = n
		}
	}
	r.InputTokensUpper = r.RequestBytes
	r.OutputTokensCap = r.Calls * maxOutputTokens
	r.RequestsSHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(lines, "\n"))))
	var err error
	r.Files, err = datasetHashes(split)
	return r, err
}

// datasetHashes names the frozen inputs of a split by content.
func datasetHashes(split string) (map[string]string, error) {
	files := map[string]string{
		"data/jp/rulings.jsonl":         filepath.Join(dataDir, "rulings.jsonl"),
		"data/jp/translations-en.jsonl": filepath.Join(dataDir, "translations-en.jsonl"),
		"data/jp/splits.json":           filepath.Join(dataDir, "splits.json"),
	}
	if b, ok := answeredBase[split]; ok {
		files["eval/jp/"+answersFile] = filepath.Join(outDir, answersFile)
		files["eval/jp/runs/"+b.run+".jsonl"] = candidateRowsPath(b.run)
	}
	switch baseSplit(split) {
	case "dev_eval":
		files["data/jp/labels-dev.jsonl"] = filepath.Join(dataDir, "labels-dev.jsonl")
	case "held_out":
		files["data/jp/labels-heldout.jsonl"] = filepath.Join(dataDir, "labels-heldout.jsonl")
	case "synthetic":
		files["eval/jp/synthetic-negatives.jsonl"] = filepath.Join(outDir, "synthetic-negatives.jsonl")
	case "fresh_1126", "fresh_1126_notwin":
		for _, f := range []string{"rulings.jsonl", "translations-en.jsonl", "labels.jsonl", "twins.json"} {
			files["data/jp/"+freshDir+"/"+f] = filepath.Join(dataDir, freshDir, f)
		}
	case "dev_neg":
		files["data/jp/labels-dev.jsonl"] = filepath.Join(dataDir, "labels-dev.jsonl")
		files["eval/jp/"+devNegFile] = filepath.Join(outDir, devNegFile)
	}
	out := map[string]string{}
	for name, p := range files {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		out[name] = fmt.Sprintf("%x", sha256.Sum256(b))
	}
	return out, nil
}

// planCommand prints the offline plan for a split as JSON. It never reads the
// environment, opens a network connection or writes under data/ or eval/.
func planCommand(args []string) error {
	fs := flag.NewFlagSet("plan", flag.ExitOnError)
	split := fs.String("split", "held_out", "dev_eval, held_out, or synthetic")
	out := fs.String("out", "", "also write the plan to this new file (exclusive create, never under data/ or eval/)")
	rin := fs.Float64("rate-in", 0, "input USD per million tokens, to price the worst case")
	rout := fs.Float64("rate-out", 0, "output USD per million tokens")
	rsrc := fs.String("rates-source", "", "where the rates come from")
	_ = fs.Parse(args)
	if fs.NArg() != 0 {
		return errors.New("no positional arguments")
	}
	if *split != "dev_eval" && *split != "dev_neg" && !isFresh(*split) && !isAnswered(*split) && *split != "held_out" && *split != "synthetic" {
		return fmt.Errorf("unknown candidate split %q", *split)
	}
	items, pool, _, err := loadCandidate(*split)
	if err != nil {
		return err
	}
	rules, err := jprules.Default()
	if err != nil {
		return err
	}
	r, err := planItems(*split, items, pool, rules)
	if err != nil {
		return err
	}
	if *rin != 0 || *rout != 0 || *rsrc != "" {
		rates := llmbudget.Rates{InputPerMTok: *rin, OutputPerMTok: *rout, Source: *rsrc}
		if err := rates.Validate(); err != nil {
			return err
		}
		w := r.worstCase(rates)
		r.WorstCaseUSD, r.Rates = &w, &rates
	}
	b, err := json.MarshalIndent(r, "", " ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if *out != "" {
		if err := safePlanPath(*out); err != nil {
			return err
		}
		f, err := os.OpenFile(*out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			return errors.New("the plan file cannot be created exclusively; choose a new path")
		}
		_, werr := f.Write(b)
		if err := errors.Join(werr, f.Close()); err != nil {
			return err
		}
	}
	_, err = os.Stdout.Write(b)
	return err
}

// safePlanPath keeps a plan out of the measured result directories.
func safePlanPath(p string) error {
	abs, err := filepath.Abs(p)
	if err != nil {
		return err
	}
	for _, dir := range []string{"../eval", "../data"} {
		root, err := filepath.Abs(dir)
		if err != nil {
			return err
		}
		if rel, err := filepath.Rel(root, abs); err == nil && !strings.HasPrefix(rel, "..") {
			return errors.New("plans are never written under data/ or eval/")
		}
	}
	return nil
}
