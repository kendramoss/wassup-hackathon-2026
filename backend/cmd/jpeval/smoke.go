package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jpengine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llmbudget"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/logsafe"
)

// smokeResult is the whole artifact of an admission smoke check. It holds
// no case data, no model text, no address and no key.
type smokeResult struct {
	RunID          string   `json:"run_id"`
	Model          string   `json:"model"`
	MaxOutput      int      `json:"max_output_tokens"`
	RequestBytes   int      `json:"request_bytes"`
	Sent           bool     `json:"request_sent"`
	Error          string   `json:"error,omitempty"`
	InputTokens    int      `json:"input_tokens"`
	OutputTokens   int      `json:"output_tokens"`
	ReportedCost   *float64 `json:"reported_cost_usd"` // null when the gateway reported none; never 0 by default
	CostProvenance string   `json:"cost_provenance,omitempty"`
	BudgetUSD      float64  `json:"budget_charged_usd"`
	RatesSource    string   `json:"rates_source"`
	Time           string   `json:"time"`
}

const smokePrompt = "Reply with the single word OK."

// smoke sends one tiny chat request: no case data, a low output cap chosen
// only to check health, one attempt, charged to the same shared budget. It
// says nothing about the study and never touches a measured path.
func smoke(args []string) error {
	fs := flag.NewFlagSet("smoke", flag.ExitOnError)
	id := fs.String("run-id", "", "unique id of this smoke check")
	out := fs.String("out", "", "new file for the result (exclusive create, never under data/ or eval/)")
	maxOut := fs.Int("max-output", 64, "output token cap for this health check, 1 to 256")
	ledgerPath := fs.String("budget-ledger", "", "shared budget ledger file")
	capUSD := fs.Float64("budget-usd", llmbudget.MaxCapUSD, "total USD cap across all runs sharing the ledger")
	rin := fs.Float64("rate-in", 0, "input USD per million tokens (an explicit estimate unless a quote was checked)")
	rout := fs.Float64("rate-out", 0, "output USD per million tokens")
	rsrc := fs.String("rates-source", "", "where the rates come from")
	_ = fs.Parse(args)
	if fs.NArg() != 0 || !runID.MatchString(*id) || *out == "" || *maxOut < 1 || *maxOut > 256 || *ledgerPath == "" {
		return errors.New("smoke needs -run-id, -out, -budget-ledger, rates and 1 to 256 -max-output")
	}
	rates := llmbudget.Rates{InputPerMTok: *rin, OutputPerMTok: *rout, Source: *rsrc}
	if err := rates.Validate(); err != nil {
		return err
	}
	if err := safePlanPath(*out); err != nil {
		return err
	}
	ledger, err := llmbudget.Open(*ledgerPath, *capUSD, llmbudget.Binding{Model: jpengine.DefaultModel, Rates: rates})
	if err != nil {
		return err
	}
	if _, err := os.Lstat(*out); err == nil {
		return errors.New("the result file already exists; choose a new path")
	}
	base, key := os.Getenv("LITELLM_BASE_URL"), os.Getenv("LITELLM_API_KEY")
	if base == "" || key == "" {
		return errors.New("needs LITELLM_BASE_URL and LITELLM_API_KEY")
	}
	gateway := llm.NewLiteLLM(base, key, 60*time.Second)
	gateway.SkipCache(true)
	return runSmoke(*id, *out, *maxOut, ledger, rates, gateway, gateway)
}

func runSmoke(id, out string, maxOut int, ledger *llmbudget.Ledger, rates llmbudget.Rates, inner llm.Model, sizer interface {
	RequestBody(llm.Request) ([]byte, error)
}) error {
	f, err := os.OpenFile(out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return errors.New("the result file cannot be created exclusively; choose a new path")
	}
	req := llm.Request{Model: jpengine.DefaultModel, MaxOutputTokens: maxOut,
		Messages: []llm.Message{{Role: llm.RoleUser, Content: smokePrompt}}}
	size := func(r llm.Request) (int, error) {
		b, err := sizer.RequestBody(r)
		return len(b), err
	}
	m := &llmbudget.Model{Inner: inner, Ledger: ledger, Rates: rates, Size: size}
	res := smokeResult{RunID: id, Model: req.Model, MaxOutput: maxOut, RatesSource: rates.Source, Time: time.Now().UTC().Format(time.RFC3339)}
	res.RequestBytes, _ = size(req)
	tally := &llm.CostTally{}
	var cost llmbudget.Cost
	resp, callErr := m.Generate(llmbudget.WithSink(llm.ContextWithCostTally(context.Background(), tally), &cost), req)
	res.Sent = cost.Provenance != ""
	res.InputTokens, res.OutputTokens = resp.Usage.InputTokens, resp.Usage.OutputTokens
	res.CostProvenance, res.BudgetUSD = cost.Provenance, cost.USD
	if tally.Reported() > 0 {
		v := tally.USD()
		res.ReportedCost = &v
	}
	if callErr != nil {
		code, status := logsafe.Classify(callErr)
		res.Error = string(code)
		if status > 0 {
			res.Error = fmt.Sprintf("%s (HTTP %d)", code, status)
		}
		if errors.Is(callErr, llmbudget.ErrStopped) {
			res.Error = "budget_stopped"
		} else if errors.Is(callErr, llmbudget.ErrBudget) {
			res.Error = "budget_cap_refused"
		}
	}
	if cost.Overrun {
		res.Error = "budget_overrun"
		callErr = errors.Join(callErr, llmbudget.ErrOverrun)
	}
	b, err := json.MarshalIndent(res, "", " ")
	if err != nil {
		return err
	}
	_, werr := f.Write(append(b, '\n'))
	if err := errors.Join(werr, f.Close()); err != nil {
		return err
	}
	fmt.Printf("smoke %s: sent %v, error %q, budget charged $%.6f (%s)\n", id, res.Sent, res.Error, res.BudgetUSD, res.CostProvenance)
	if callErr != nil {
		// The raw error may carry provider text; only the saved class is shown.
		return fmt.Errorf("smoke call failed: %s", res.Error)
	}
	return nil
}
