package httpapi

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jurisdiction"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/nporigin"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// originFactsRequest is the body of PUT /v1/cases/{id}/origin-facts/{market}:
// every fact the requester supplies to that market's check, replacing what
// was saved before. An empty body clears them.
type originFactsRequest struct {
	// ExpectedUpdatedAt, when sent, is the updated_at of this market's saved
	// facts as the client last read them ("" when none were saved). If the
	// saved facts changed since, nothing is saved: 409 (D806).
	ExpectedUpdatedAt *string                   `json:"expected_updated_at,omitempty"`
	GoodHTS           string                    `json:"good_hts,omitempty"`
	Row               string                    `json:"row,omitempty"`
	Answers           map[string]bool           `json:"answers,omitempty"`
	Materials         []domain.SuppliedMaterial `json:"materials,omitempty"`
}

// OriginFactsView is the answer of PUT /v1/cases/{id}/origin-facts/{market}:
// what is saved now for every market, and the market's check re-run with it.
type OriginFactsView struct {
	CaseID      string                                 `json:"case_id"`
	Market      string                                 `json:"market"`
	OriginFacts map[string]*domain.SuppliedOriginFacts `json:"origin_facts"`
	Check       OriginRulesView                        `json:"check"`
}

// originMarketOf is the EU or UK market and its rules for a path value.
// known is false for any other value; ok is false too when the server has
// not loaded that market's rules (RULE_CHECK off).
func (a *api) originMarketOf(code string) (m originMarket, rules *nporigin.Rules, known, ok bool) {
	switch code {
	case jurisdiction.EU:
		return euMarket, a.d.EUOrigin, true, a.d.EUOrigin != nil
	case jurisdiction.UK:
		return ukMarket, a.d.UKOrigin, true, a.d.UKOrigin != nil
	}
	return originMarket{}, nil, false, false
}

// errOriginFactsChanged: the saved facts changed since the client read them.
var errOriginFactsChanged = errors.New("conflict: the answers for this check changed since you loaded them; reload and answer again")

// originFactsOf is the case's supplied facts as read: a deep copy, each set
// marked stale when the parts list was replaced (a bom_updated event) after
// it was saved (D806). Never stored.
func originFactsOf(c store.Case) map[string]*domain.SuppliedOriginFacts {
	out := domain.CloneOriginFacts(c.OriginFacts)
	for _, s := range out {
		if s == nil {
			continue
		}
		for _, e := range c.History {
			if e.Kind == "bom_updated" && e.At.After(s.UpdatedAt) {
				s.Stale = true
				break
			}
		}
	}
	return out
}

func sameStamp(want string, s *domain.SuppliedOriginFacts) bool {
	if s == nil {
		return want == ""
	}
	t, err := time.Parse(time.RFC3339Nano, want)
	return err == nil && t.Equal(s.UpdatedAt)
}

// putOriginFacts saves the facts a requester supplied to a case's EU or UK
// origin check and answers with the check re-run (#770, D801). The facts
// are check inputs, stored beside the case's product with source
// "user-supplied" and a timestamp; the product and facts text are never
// changed. No model call.
func (a *api) putOriginFacts(w http.ResponseWriter, r *http.Request) {
	c, err := a.loadCase(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	m, rules, known, ok := a.originMarketOf(r.PathValue("market"))
	if !known {
		a.fail(w, r, fmt.Errorf("%w: market must be eu or uk", domain.ErrInvalid))
		return
	}
	if !ok {
		a.fail(w, r, fmt.Errorf("%w: the %s origin rules are not loaded on this server (the rule check is off), so there is no check to answer", domain.ErrInvalid, strings.ToUpper(m.code)))
		return
	}
	if a.refuseDemoEdit(w, r, c) {
		return
	}
	var in originFactsRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if c.Product == nil || strings.TrimSpace(c.Product.HTS) == "" {
		a.fail(w, r, fmt.Errorf("%w: the case has no product code; add the product's HS code first", domain.ErrInvalid))
		return
	}
	if len(in.Answers) > maxOriginAnswers || len(in.Materials) > maxOriginMaterials {
		a.fail(w, r, fmt.Errorf("%w: at most %d answers and %d materials", domain.ErrInvalid, maxOriginAnswers, maxOriginMaterials))
		return
	}
	s := &domain.SuppliedOriginFacts{Source: domain.SourceUserSupplied, UpdatedAt: time.Now().UTC(), Actor: identityOf(r.Context()).email,
		GoodHTS: strings.TrimSpace(in.GoodHTS), Row: strings.TrimSpace(in.Row), Answers: in.Answers, Materials: in.Materials}
	for i := range s.Materials {
		s.Materials[i].HTS = strings.TrimSpace(s.Materials[i].HTS)
		s.Materials[i].Origin = domain.NormalizeCountry(s.Materials[i].Origin)
	}
	// Checked against the case as loaded; the update below re-checks it
	// against the latest product so a concurrent parts change cannot slip
	// in facts for a part that is gone.
	if in.ExpectedUpdatedAt != nil && !sameStamp(*in.ExpectedUpdatedAt, c.OriginFacts[m.code]) {
		a.fail(w, r, errOriginFactsChanged)
		return
	}
	if _, err := originCheck(m, rules, *c.Product, s, nporigin.Input{}, true); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.auditCase(r, c, "origin facts "+m.code); err != nil {
		a.fail(w, r, err)
		return
	}
	var v OriginRulesView
	saved, err := a.d.Cases.Update(r.Context(), c.ID, func(latest *store.Case) error {
		if latest.Product == nil {
			return fmt.Errorf("%w: the case has no product", domain.ErrInvalid)
		}
		if in.ExpectedUpdatedAt != nil && !sameStamp(*in.ExpectedUpdatedAt, latest.OriginFacts[m.code]) {
			return errOriginFactsChanged
		}
		var err error
		if v, err = originCheck(m, rules, *latest.Product, s, nporigin.Input{}, true); err != nil {
			return err
		}
		// A new map, never a write into the stored one (readers hold it).
		facts := maps.Clone(latest.OriginFacts)
		sum := fmt.Sprintf("%s origin facts cleared", strings.ToUpper(m.code))
		if s.Empty() {
			delete(facts, m.code)
		} else {
			if facts == nil {
				facts = map[string]*domain.SuppliedOriginFacts{}
			}
			facts[m.code] = s
			sum = fmt.Sprintf("%s origin facts supplied: %d answers, %d parts%s; check %s", strings.ToUpper(m.code), len(s.Answers), len(s.Materials), rowNote(s.Row), v.Result.Status)
		}
		if len(facts) == 0 {
			facts = nil
		}
		latest.OriginFacts = facts
		latest.UpdatedAt = s.UpdatedAt
		record(r.Context(), latest, store.Event{Kind: "origin_facts", Summary: sum})
		return nil
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	facts := originFactsOf(saved)
	if facts == nil {
		facts = map[string]*domain.SuppliedOriginFacts{}
	}
	writeJSON(w, http.StatusOK, OriginFactsView{CaseID: saved.ID, Market: m.code, OriginFacts: facts, Check: v})
}

func rowNote(row string) string {
	if row == "" {
		return ""
	}
	return ", row " + row
}
