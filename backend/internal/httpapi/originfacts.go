package httpapi

import (
	"fmt"
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
	GoodHTS   string                    `json:"good_hts,omitempty"`
	Row       string                    `json:"row,omitempty"`
	Answers   map[string]bool           `json:"answers,omitempty"`
	Materials []domain.SuppliedMaterial `json:"materials,omitempty"`
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
func (a *api) originMarketOf(code string) (originMarket, *nporigin.Rules, bool) {
	switch code {
	case jurisdiction.EU:
		return euMarket, a.d.EUOrigin, a.d.EUOrigin != nil
	case jurisdiction.UK:
		return ukMarket, a.d.UKOrigin, a.d.UKOrigin != nil
	}
	return originMarket{}, nil, false
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
	m, rules, ok := a.originMarketOf(r.PathValue("market"))
	if !ok {
		a.fail(w, r, fmt.Errorf("%w: market must be eu or uk", domain.ErrInvalid))
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
	if _, err := originCheck(m, rules, *c.Product, s, nporigin.Input{}); err != nil {
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
		var err error
		if v, err = originCheck(m, rules, *latest.Product, s, nporigin.Input{}); err != nil {
			return err
		}
		sum := fmt.Sprintf("%s origin facts cleared", strings.ToUpper(m.code))
		if s.Empty() {
			delete(latest.OriginFacts, m.code)
			if len(latest.OriginFacts) == 0 {
				latest.OriginFacts = nil
			}
		} else {
			if latest.OriginFacts == nil {
				latest.OriginFacts = map[string]*domain.SuppliedOriginFacts{}
			}
			latest.OriginFacts[m.code] = s
			sum = fmt.Sprintf("%s origin facts supplied: %d answers, %d parts%s; check %s", strings.ToUpper(m.code), len(s.Answers), len(s.Materials), rowNote(s.Row), v.Result.Status)
		}
		latest.UpdatedAt = s.UpdatedAt
		record(r.Context(), latest, store.Event{Kind: "origin_facts", Summary: sum})
		return nil
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	facts := saved.OriginFacts
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
