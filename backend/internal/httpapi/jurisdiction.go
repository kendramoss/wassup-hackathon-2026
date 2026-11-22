package httpapi

import (
	"fmt"
	"net/http"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jurisdiction"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// markets is the registry of supported markets (D680).
func (a *api) markets() *jurisdiction.Registry {
	if a.d.Jurisdictions == nil {
		return jurisdiction.Default()
	}
	return a.d.Jurisdictions
}

// caseMarket is a stored case's market for a dispatch (nil: a case made
// before D680, read as the United States).
func caseMarket(c store.Case) *domain.Jurisdiction {
	if c.Jurisdiction == nil {
		return nil
	}
	j := *c.Jurisdiction
	return &j
}

// marketText is what the detector reads: the facts and, for a structured
// product, its name and description.
func marketText(facts string, p *domain.Product) string {
	if p == nil {
		return facts
	}
	return p.Name + "\n" + p.Description + "\n" + facts
}

// requireMarket refuses a follow-up (plan, compare, scenarios, export, ask,
// watch, review link) on a case whose market has no registered authority,
// and a call whose jurisdiction field names another market than the case's.
// Every follow-up is a U.S. CBP search, letter or check.
func (a *api) requireMarket(c store.Case, asked string) error {
	code := jurisdiction.Of(c.Jurisdiction).Code
	if asked != "" && asked != code {
		if !jurisdiction.Valid(asked) {
			return fmt.Errorf("%w: jurisdiction %q is not one of %v", domain.ErrInvalid, asked, jurisdiction.Codes())
		}
		return fmt.Errorf("%w: this case is for %s, not %s; create a new case for that market", domain.ErrInvalid, jurisdiction.Name(code), jurisdiction.Name(asked))
	}
	if !a.markets().Supported(code) {
		return fmt.Errorf("%w: %s", domain.ErrInvalid, jurisdiction.UnsupportedReason(code))
	}
	if code != jurisdiction.US {
		// A registered non-U.S. authority has an origin engine only; plan
		// search, the CBP letter and broker review are U.S. features.
		return fmt.Errorf("%w: this step is for U.S. cases; %s cases get an origin answer only", domain.ErrInvalid, jurisdiction.Name(code))
	}
	return nil
}

// marketFields are the case's market fields in an API response.
func (a *api) marketFields(f map[string]any, c store.Case) {
	d := a.markets().DetailOf(c.Jurisdiction)
	f["jurisdiction"] = d.Code
	f["jurisdiction_detail"] = d
}

// listJurisdictions is GET /v1/jurisdictions: every market a case can name
// and whether it is supported.
func (a *api) listJurisdictions(w http.ResponseWriter, r *http.Request) {
	a.respond(w, http.StatusOK, map[string]any{"jurisdictions": a.markets().List(), "detector": jurisdiction.DetectorVersion})
}
