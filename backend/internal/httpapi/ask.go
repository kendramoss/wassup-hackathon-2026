package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/caseagent"
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
	// History is the visitor's own completed turns on a read-only pinned
	// demo (#309). It is untrusted conversation text, not case evidence: it
	// is validated, only affects this request's answer and memo key, and is
	// never stored. On every other case it is ignored; the saved chat is the
	// sole source.
	History []askHistoryTurn `json:"history"`
}

type askHistoryTurn struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// MaxAskAnswer caps one supplied answer, in runes (questions keep the
// 2,000-rune ask limit). A supplied history holds at most caseagent.MaxHistory
// turns, so the payload is bounded; out-of-bound input is refused, never cut.
const MaxAskAnswer = 4000

// clientHistory validates the supplied turns of a pinned-demo ask.
func clientHistory(in []askHistoryTurn) ([]caseagent.Turn, error) {
	if len(in) > caseagent.MaxHistory {
		return nil, fmt.Errorf("%w: history holds at most %d turns", domain.ErrInvalid, caseagent.MaxHistory)
	}
	out := make([]caseagent.Turn, len(in))
	for i, t := range in {
		q, a := strings.TrimSpace(t.Question), strings.TrimSpace(t.Answer)
		switch {
		case q == "" || a == "":
			return nil, fmt.Errorf("%w: history turn %d needs a question and an answer", domain.ErrInvalid, i+1)
		case utf8.RuneCountInString(q) > 2000:
			return nil, fmt.Errorf("%w: history question %d is longer than 2,000 characters", domain.ErrInvalid, i+1)
		case utf8.RuneCountInString(a) > MaxAskAnswer:
			return nil, fmt.Errorf("%w: history answer %d is longer than %d characters", domain.ErrInvalid, i+1, MaxAskAnswer)
		}
		out[i] = caseagent.Turn{Question: q, Answer: a}
	}
	return out, nil
}

func (a *api) ask(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	if a.d.Agent == nil {
		writeError(w, http.StatusServiceUnavailable, "internal", "case agent is not configured", nil)
		return
	}
	c, err := a.loadCase(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var in askRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	readOnly := a.demoReadOnly(r, c)
	var visitor []caseagent.Turn
	if readOnly && !in.Reset {
		if visitor, err = clientHistory(in.History); err != nil {
			a.fail(w, r, err)
			return
		}
	}
	if in.Reset && readOnly {
		a.refuseDemoEdit(w, r, c)
		return
	}
	q := strings.TrimSpace(in.Question)
	if q == "" && !in.Reset {
		a.fail(w, r, fmt.Errorf("%w: question is required", domain.ErrInvalid))
		return
	}
	if len([]rune(q)) > 2000 {
		a.fail(w, r, fmt.Errorf("%w: question is longer than 2,000 characters", domain.ErrInvalid))
		return
	}
	// Validated: only now is the intent recorded (an ask appends a chat turn, or
	// resets the chat, unless the case is a pinned demo).
	if err := a.auditCase(r, c, "ask"); err != nil {
		a.fail(w, r, err)
		return
	}
	if q == "" {
		saved, err := a.d.Cases.Update(r.Context(), c.ID, func(latest *store.Case) error {
			latest.Chat = nil
			return nil
		})
		if err != nil {
			a.fail(w, r, err)
			return
		}
		a.respond(w, http.StatusOK, map[string]any{"case_id": saved.ID, "chat": chatField(saved.Chat)})
		return
	}
	// The model sees the conversation as it was when the question was
	// asked. The saved turn is applied to whatever the case is when the
	// answer comes back, so an export during the ask is not overwritten.
	if in.Reset {
		c.Chat = nil
	}
	date, _ := time.Parse(time.DateOnly, c.Date)
	hist := make([]caseagent.Turn, len(c.Chat))
	for i, t := range c.Chat {
		hist[i] = caseagent.Turn{Question: t.Question, Answer: t.Answer}
	}
	// A pinned demo has no visitor-specific saved chat: the visitor's own
	// completed turns follow the seeded ones. The agent sees only the last
	// MaxHistory turns, so that is also what the memo key covers.
	if len(visitor) > 0 {
		hist = append(hist, visitor...)
	}
	if len(hist) > caseagent.MaxHistory {
		hist = hist[len(hist)-caseagent.MaxHistory:]
	}
	// The same question on the same case and conversation is answered from
	// memory: the preset questions are asked again and again (18 Oct).
	key := askKey(c, hist, q)
	a.askMu.Lock()
	ans, hit := a.askMemo[key]
	a.askMu.Unlock()
	if !hit {
		ans, err = a.d.Agent.Ask(r.Context(), caseagent.Input{Question: q, History: hist, CaseSummary: agentCaseText(c),
			Facts: c.Facts, Date: date, Exclude: c.Exclude})
		if err != nil {
			a.fail(w, r, err)
			return
		}
		a.askMu.Lock()
		if a.askMemo == nil {
			a.askMemo = map[string]caseagent.Answer{}
		}
		if len(a.askMemo) >= maxAskMemo {
			for k := range a.askMemo { // drop one at random; the memo is a convenience
				delete(a.askMemo, k)
				break
			}
		}
		a.askMemo[key] = ans
		a.askMu.Unlock()
	}
	turn := chatTurn(q, ans)
	// Usage is the agent model's own calls; engine calls made by its
	// what_if tool are not included. An answer served from memory made no
	// model call, so it charges nothing again; it keeps the answer's
	// provenance and says where it came from.
	var u usage
	// The summary carries the question's LENGTH, not its text: the text stays
	// in the case chat, which a reset can clear, while history keeps at most
	// 200 events and survives a reset (#285). Cited ids are public CBP ruling
	// numbers the agent saw in a tool result.
	summary := fmt.Sprintf("question (%d characters), %d what-ifs, cited %s", utf8.RuneCountInString(q), len(ans.WhatIfs), strings.Join(ans.Cited, ", "))
	if hit {
		summary += ", answered from memory"
	} else {
		u.trace(ans.Trace)
	}
	ev := u.on(store.Event{Kind: "asked", Summary: summary,
		Provider: ans.Trace.Provider, Model: ans.Trace.ModelID, Prompt: ans.Trace.PromptVersion}, start)
	// On a demo case the visitor gets the answer, but it is not saved to the
	// case's chat, so the next viewer does not see it (D57).
	if readOnly {
		chat := append(append([]store.ChatTurn{}, c.Chat...), turn)
		if len(chat) > MaxChat {
			chat = chat[len(chat)-MaxChat:]
		}
		a.respond(w, http.StatusOK, map[string]any{"case_id": c.ID, "reply": ans, "cited_detail": a.precedentRefs(ans.Cited), "chat": chatField(chat)})
		return
	}
	saved, err := a.d.Cases.Update(r.Context(), c.ID, func(latest *store.Case) error {
		if in.Reset {
			latest.Chat = nil
		}
		latest.Chat = append(latest.Chat, turn)
		if len(latest.Chat) > MaxChat {
			latest.Chat = latest.Chat[len(latest.Chat)-MaxChat:]
		}
		record(r.Context(), latest, ev) // asking is not case activity: updated_at stays
		return nil
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.respond(w, http.StatusOK, map[string]any{"case_id": saved.ID, "reply": ans, "cited_detail": a.precedentRefs(ans.Cited), "chat": chatField(saved.Chat)})
}

// chatTurn stores the answer with the what-ifs (including revised facts)
// and the steps the agent took.
func chatTurn(q string, ans caseagent.Answer) store.ChatTurn {
	whats := make([]store.ChatWhatIf, len(ans.WhatIfs))
	for i, w := range ans.WhatIfs {
		whats[i] = store.ChatWhatIf{Change: w.Change, Origin: w.Origin, Confidence: w.Confidence, Decisive: w.Decisive, Facts: w.Facts,
			Regime: w.Regime, Specialist: w.Specialist, Scope: w.Scope, Route: string(w.Route), Reason: w.Reason, Provisional: w.Provisional}
	}
	steps := make([]store.ChatStep, len(ans.Steps))
	for i, s := range ans.Steps {
		steps[i] = store.ChatStep{Tool: s.Tool, Arg: s.Arg}
	}
	return store.ChatTurn{Question: q, Answer: ans.Answer, Cited: ans.Cited, At: time.Now().UTC(), WhatIfs: whats, Steps: steps}
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
	fmt.Fprintf(&b, "\n%s", caseagent.FactsSection(c.Facts))
	p := c.Prediction
	fmt.Fprintf(&b, "\nPREDICTION: origin %s, confidence %.2f. Decisive step: %s. Reasoning: %s. Earlier rulings used: %s. Review route: %s (%s).\n",
		p.Origin, p.Confidence, p.DecisiveStep, p.Reasoning, strings.Join(p.Precedents, ", "), c.Triage.Route, c.Triage.Reason)
	if c.Clarify != nil {
		fmt.Fprintf(&b, "Follow-up questions suggested: %s\n", strings.Join(c.Clarify.Questions, " | "))
	}
	if c.Plan != nil {
		stale := planMoneyStale(c)
		fmt.Fprintf(&b, "\nPLAN SEARCH (target %s):\n", c.Plan.Target)
		for _, pl := range c.Plan.Plans {
			fmt.Fprintf(&b, "- SHOWN: %s; predicted %s; %s; %s\n", pl.Description, pl.Prediction.Origin,
				precedentCheckText(pl, true), planNetText(pl, stale))
		}
		for _, pl := range c.Plan.Rejected {
			fmt.Fprintf(&b, "- not shown (%s): %s; predicted %s; %s%s\n", pl.RejectionKind, pl.Description, pl.Prediction.Origin, precedentCheckText(pl, false), costRejectionText(pl, stale))
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

// precedentCheckText states the precedent check as it stands: a plan with no
// verdict never had one run, which is not the same as a confidence of 0.00.
func precedentCheckText(p domain.Plan, withRuling bool) string {
	if p.Verdict == nil {
		return "precedent check not run"
	}
	s := fmt.Sprintf("precedent check %.2f", p.Verdict.Confidence)
	if withRuling {
		ruling := p.Verdict.SupportingRuling
		if ruling == "" {
			ruling = "none"
		}
		s += ", supporting " + ruling
	}
	return s
}

// netText states a plan's yearly net by what the search that made the plan
// recorded (Plan.NetPriced), never by the case's current economics. Priced:
// the figure, including a real zero. Searched but not priced: never a figure.
// Stored before the flag existed: a nonzero recorded figure is kept, labelled
// as historical and unverified; an all-zero one can't be told from "unknown".
func planNetText(p domain.Plan, stale bool) string {
	if p.Operations != nil {
		return opsNetText(p, stale)
	}
	return netText(p)
}

// costRejectionText adds why a cost demotion happened, so the agent does not
// call a move the legal gate supported "not allowed". The reason carries money,
// so it is left out when the money has changed since.
func costRejectionText(p domain.Plan, stale bool) string {
	if p.Operations == nil || stale {
		return ""
	}
	if p.RejectionKind != domain.RejectNetCostlier && p.RejectionKind != domain.RejectOperationallyInfeasible {
		return ""
	}
	return "; supported by precedent but demoted on the inputs entered: " + p.RejectionReason
}

func netText(p domain.Plan) string {
	switch {
	case p.NetPriced != nil && *p.NetPriced:
		return fmt.Sprintf("net $%.0f/yr", p.NetYear)
	case p.NetPriced == nil && (p.NetYear != 0 || p.DutyEffectYear != 0 || p.AddedCostYear != 0):
		return fmt.Sprintf("net $%.0f/yr (recorded by an earlier search; not verified as fully priced)", p.NetYear)
	}
	return "net not priced"
}

// maxAskMemo caps the remembered answers per instance.
const maxAskMemo = 500

// askKey names a question on a case as the agent would see it: the case as
// last changed, the conversation so far and the question (case-folded).
func askKey(c store.Case, hist []caseagent.Turn, q string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00", c.ID, c.UpdatedAt.Format(time.RFC3339Nano), strings.ToLower(strings.Join(strings.Fields(q), " ")))
	// Length-prefixed, so turn boundaries cannot be forged by text that
	// contains the separator.
	fmt.Fprintf(h, "%d\x00", len(hist))
	for _, t := range hist {
		fmt.Fprintf(h, "%d:%s\x00%d:%s\x00", len(t.Question), t.Question, len(t.Answer), t.Answer)
	}
	return hex.EncodeToString(h.Sum(nil))
}
