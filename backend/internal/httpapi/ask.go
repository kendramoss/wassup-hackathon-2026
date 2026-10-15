package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/caseagent"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// CaseAgent answers questions about a case (caseagent.Agent).
type CaseAgent interface {
	Ask(ctx context.Context, in caseagent.Input) (caseagent.Answer, error)
}

// MaxChat caps the turns kept on a case.
const MaxChat = 20

type askRequest struct {
	Question string `json:"question"`
	// Reset clears the case's conversation before asking (or alone).
	Reset bool `json:"reset"`
}

func (a *api) ask(w http.ResponseWriter, r *http.Request) {
	if a.d.Agent == nil {
		writeError(w, http.StatusServiceUnavailable, "internal", "case agent is not configured", nil)
		return
	}
	c, err := a.d.Cases.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var in askRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if in.Reset {
		c.Chat = nil
	}
	q := strings.TrimSpace(in.Question)
	if q == "" {
		if !in.Reset {
			a.fail(w, r, fmt.Errorf("%w: question is required", domain.ErrInvalid))
			return
		}
		if err := a.d.Cases.Put(r.Context(), c); err != nil {
			a.fail(w, r, err)
			return
		}
		a.respond(w, http.StatusOK, map[string]any{"case_id": c.ID, "chat": []store.ChatTurn{}})
		return
	}
	if len([]rune(q)) > 2000 {
		a.fail(w, r, fmt.Errorf("%w: question is longer than 2,000 characters", domain.ErrInvalid))
		return
	}
	date, _ := time.Parse(time.DateOnly, c.Date)
	hist := make([]caseagent.Turn, len(c.Chat))
	for i, t := range c.Chat {
		hist[i] = caseagent.Turn{Question: t.Question, Answer: t.Answer}
	}
	ans, err := a.d.Agent.Ask(r.Context(), caseagent.Input{Question: q, History: hist, CaseSummary: agentCaseText(c),
		Facts: c.Facts, Date: date, Exclude: c.Exclude})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	c.Chat = append(c.Chat, store.ChatTurn{Question: q, Answer: ans.Answer, Cited: ans.Cited, At: time.Now().UTC()})
	if len(c.Chat) > MaxChat {
		c.Chat = c.Chat[len(c.Chat)-MaxChat:]
	}
	c.Record(store.Event{Kind: "asked", Summary: fmt.Sprintf("%q (%d what-ifs, cited %s)", q, len(ans.WhatIfs), strings.Join(ans.Cited, ", ")),
		Provider: ans.Trace.Provider, Model: ans.Trace.ModelID, Prompt: ans.Trace.PromptVersion})
	if err := a.d.Cases.Put(r.Context(), c); err != nil { // asking is not case activity: updated_at stays
		a.fail(w, r, err)
		return
	}
	a.respond(w, http.StatusOK, map[string]any{"case_id": c.ID, "reply": ans, "cited_detail": a.precedentRefs(ans.Cited), "chat": c.Chat})
}

// agentCaseText is what the agent's get_case tool returns.
func agentCaseText(c store.Case) string {
	var b strings.Builder
	fmt.Fprintf(&b, "CASE %s, dated %s.\n", c.ID, c.Date)
	if c.Product != nil {
		fmt.Fprintf(&b, "Product: %s.\n", c.Product.Name)
	}
	if c.CurrentOrigin != nil && c.CurrentOrigin.Country != "" {
		fmt.Fprintf(&b, "CBP's ruling on the current setup: %s (%s).\n", c.CurrentOrigin.Country, c.CurrentOrigin.Ruling)
	}
	fmt.Fprintf(&b, "\nFACTS:\n%s\n", corpus.Truncate(c.Facts, 6000))
	p := c.Prediction
	fmt.Fprintf(&b, "\nPREDICTION: origin %s, confidence %.2f. Decisive step: %s. Reasoning: %s. Earlier rulings used: %s. Review route: %s (%s).\n",
		p.Origin, p.Confidence, p.DecisiveStep, p.Reasoning, strings.Join(p.Precedents, ", "), c.Triage.Route, c.Triage.Reason)
	if c.Clarify != nil {
		fmt.Fprintf(&b, "Follow-up questions suggested: %s\n", strings.Join(c.Clarify.Questions, " | "))
	}
	if c.Plan != nil {
		fmt.Fprintf(&b, "\nPLAN SEARCH (target %s):\n", c.Plan.Target)
		for _, pl := range c.Plan.Plans {
			fmt.Fprintf(&b, "- SHOWN: %s; predicted %s; precedent check %.2f, supporting %s; net $%.0f/yr\n", pl.Description, pl.Prediction.Origin,
				verdictConf(pl), verdictRuling(pl), pl.NetYear)
		}
		for _, pl := range c.Plan.Rejected {
			fmt.Fprintf(&b, "- not shown (%s): %s; predicted %s; precedent check %.2f\n", pl.RejectionKind, pl.Description, pl.Prediction.Origin, verdictConf(pl))
		}
	}
	for _, s := range c.Scenarios {
		fmt.Fprintf(&b, "SCENARIO %q: predicted %s at %.2f (%s)\n", s.Label, s.Prediction.Origin, s.Prediction.Confidence, s.Triage.Route)
	}
	if c.Targets != nil {
		for _, t := range c.Targets.Ranked {
			fmt.Fprintf(&b, "TARGET COUNTRY #%d %s: %s\n", t.Rank, t.Target, t.Status)
		}
	}
	return b.String()
}

func verdictConf(p domain.Plan) float64 {
	if p.Verdict == nil {
		return 0
	}
	return p.Verdict.Confidence
}

func verdictRuling(p domain.Plan) string {
	if p.Verdict == nil || p.Verdict.SupportingRuling == "" {
		return "none"
	}
	return p.Verdict.SupportingRuling
}
