package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/clarify"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evreport"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jurisdiction"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// maxFamilyAnswers caps one save; no family asks more than five questions.
const maxFamilyAnswers = 20

// familyFactsRequest is the body of PUT /v1/cases/{id}/family-facts: every
// answer to the case's family questions, replacing what was saved before.
// An empty body clears them.
type familyFactsRequest struct {
	// ExpectedUpdatedAt, when sent, is the updated_at of the saved answers
	// as the client last read them ("" when none were saved): 409 when they
	// changed since (as D806).
	ExpectedUpdatedAt *string                        `json:"expected_updated_at,omitempty"`
	Answers           map[string]domain.FamilyAnswer `json:"answers,omitempty"`
}

// FamilyFactsView is the answer of PUT /v1/cases/{id}/family-facts: what is
// saved now and the case's family questions read with it.
type FamilyFactsView struct {
	CaseID       string                      `json:"case_id"`
	FamilyFacts  *domain.SuppliedFamilyFacts `json:"family_facts"`
	FamilyIntake clarify.FamilyIntake        `json:"family_intake"`
}

var errFamilyFactsChanged = errors.New("conflict: the answers to these questions changed since you loaded them; reload and answer again")

// familyFactsOf is the case's family answers as read: a copy, marked stale
// when the parts list was replaced or the family changed after they were
// saved (D812). Never stored.
func familyFactsOf(c store.Case) *domain.SuppliedFamilyFacts {
	s := c.FamilyFacts.Clone()
	if s == nil {
		return nil
	}
	if s.Family != c.Family {
		s.Stale = true
	}
	for _, e := range c.History {
		if e.Kind == "bom_updated" && e.At.After(s.UpdatedAt) {
			s.Stale = true
			break
		}
	}
	return s
}

// familyIntakeOf works out the case's family questions (deterministic, no
// model call; D810).
func (a *api) familyIntakeOf(c store.Case) clarify.FamilyIntake {
	return clarify.Intake(clarify.FamilyCase{
		US: jurisdiction.Of(c.Jurisdiction).Code == jurisdiction.US, Family: c.Family, Facts: c.Facts, Product: c.Product,
		Supplied: familyFactsOf(c),
	}, a.d.Templates)
}

func sameFamilyStamp(want string, s *domain.SuppliedFamilyFacts) bool {
	if s == nil {
		return want == ""
	}
	t, err := time.Parse(time.RFC3339Nano, want)
	return err == nil && t.Equal(s.UpdatedAt)
}

// putFamilyFacts saves the requester's answers to the case's family
// questions (#770, D810-D812). They sit beside the case, source
// "user-supplied", timestamped; the facts text, product and parts list are
// never changed, and no model is called. The stored prediction is not
// re-run (it is a model answer): the case list marks it (D814).
func (a *api) putFamilyFacts(w http.ResponseWriter, r *http.Request) {
	c, err := a.loadCase(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if a.refuseDemoEdit(w, r, c) {
		return
	}
	var in familyFactsRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if len(in.Answers) > maxFamilyAnswers {
		a.fail(w, r, fmt.Errorf("%w: at most %d answers", domain.ErrInvalid, maxFamilyAnswers))
		return
	}
	answers := make(map[string]domain.FamilyAnswer, len(in.Answers))
	for id, ans := range in.Answers {
		ans.Country = strings.TrimSpace(ans.Country)
		if ans.Country != "" {
			ans.Country = domain.NormalizeCountry(ans.Country)
		}
		answers[strings.TrimSpace(id)] = ans
	}
	check := func(latest store.Case) error {
		if in.ExpectedUpdatedAt != nil && !sameFamilyStamp(*in.ExpectedUpdatedAt, latest.FamilyFacts) {
			return errFamilyFactsChanged
		}
		if len(answers) == 0 {
			return nil
		}
		if err := clarify.ValidateFamilyAnswers(a.familyIntakeOf(latest), answers); err != nil {
			return fmt.Errorf("%w: %v", domain.ErrInvalid, err)
		}
		return nil
	}
	// Checked against the case as loaded, then again inside the update
	// against the latest case, so a concurrent product change cannot slip
	// in an answer to a question it no longer asks.
	if err := check(c); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.auditCase(r, c, "family facts"); err != nil {
		a.fail(w, r, err)
		return
	}
	now := time.Now().UTC()
	saved, err := a.d.Cases.Update(r.Context(), c.ID, func(latest *store.Case) error {
		if err := check(*latest); err != nil {
			return err
		}
		sum := "Family answers cleared"
		if len(answers) == 0 {
			latest.FamilyFacts = nil
		} else {
			tv := ""
			if tm, ok := a.d.Templates[latest.Family]; ok {
				tv = tm.Version
			}
			// A new value, never a write into the stored one.
			latest.FamilyFacts = &domain.SuppliedFamilyFacts{Source: domain.SourceUserSupplied, UpdatedAt: now, Actor: identityOf(r.Context()).email,
				Family: latest.Family, TemplateVersion: tv, Answers: answers}
			open := a.familyIntakeOf(*latest).Open()
			sum = fmt.Sprintf("Family answers supplied: %d answered, %d open (%s)", len(answers), open, clarify.FamilyVersion)
		}
		latest.UpdatedAt = now
		record(r.Context(), latest, store.Event{Kind: "family_facts", Summary: sum, Prompt: clarify.FamilyVersion})
		return nil
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, FamilyFactsView{CaseID: saved.ID, FamilyFacts: familyFactsOf(saved), FamilyIntake: a.familyIntakeOf(saved)})
}

// predictionKinds are the history events that store a new prediction.
var predictionKinds = map[string]bool{"predicted": true, "answered": true, "bom_updated": true, rechecked: true}

// rechecked is the event of a rules-only market's stored check re-run with
// supplied answers (D815).
const rechecked = "rechecked"

// answersNote is the case list's note on answers (D814): "rechecked" when
// the newest stored prediction is a re-check with supplied answers, "added"
// when family answers or origin facts were saved after the newest stored
// prediction and did not re-run it.
func answersNote(c store.Case) string {
	var last store.Event
	for _, e := range c.History {
		if predictionKinds[e.Kind] {
			last = e
		}
	}
	if last.Kind == rechecked {
		if !c.OriginFacts[jurisdiction.Of(c.Jurisdiction).Code].Empty() {
			return "rechecked"
		}
		return "" // answers cleared: the stored check is the codes-only one again
	}
	for _, e := range c.History {
		if (e.Kind == "family_facts" || e.Kind == "origin_facts") && e.At.After(last.At) && (c.FamilyFacts != nil || len(c.OriginFacts) > 0) {
			return "added"
		}
	}
	return ""
}

// familyAnswersOf is the report's list of family answers (D810): each saved
// answer beside its question, when the case has some.
func (a *api) familyAnswersOf(c store.Case) *evreport.FamilyAnswers {
	s := familyFactsOf(c)
	if s.Empty() {
		return nil
	}
	out := &evreport.FamilyAnswers{Family: s.Family, Version: clarify.FamilyVersion, Source: s.Source, Saved: s.UpdatedAt, Stale: s.Stale}
	if tm, ok := a.d.Templates[s.Family]; ok {
		out.Family = tm.Name
	}
	in := a.familyIntakeOf(c)
	seen := map[string]bool{}
	for _, q := range in.Questions {
		if ans, ok := s.Answers[q.ID]; ok {
			seen[q.ID] = true
			out.Lines = append(out.Lines, evreport.FamilyAnswerLine{Question: q.Question, Answer: answerText(ans), Evidence: q.Evidence})
		}
	}
	ids := make([]string, 0, len(s.Answers))
	for id := range s.Answers {
		if !seen[id] {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	for _, id := range ids {
		out.Lines = append(out.Lines, evreport.FamilyAnswerLine{Question: id + " (no longer asked)", Answer: answerText(s.Answers[id]), Evidence: "-"})
	}
	return out
}

func answerText(a domain.FamilyAnswer) string {
	switch {
	case a.Country != "":
		return a.Country
	case a.Yes != nil && *a.Yes:
		return "Yes"
	case a.Yes != nil:
		return "No"
	}
	return "-"
}
