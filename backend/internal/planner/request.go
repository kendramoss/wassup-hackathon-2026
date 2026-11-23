package planner

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/duty"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner/opcost"
)

// Request is one plan search: a product as of a date, the product family
// (which template to use), the target origin, and the user's economics.
type Request struct {
	CaseID    string         `json:"case_id"`
	Date      Date           `json:"date"`
	Family    string         `json:"family"`
	Target    string         `json:"target"`
	Product   domain.Product `json:"product"`
	Economics Economics      `json:"economics"`
	// Exclude lists rulings that describe this very case (e.g. its own
	// earlier ruling); they are never used as precedent or evidence.
	Exclude []string `json:"exclude_rulings,omitempty"`
	// CurrentOrigin, when known from a CBP ruling on the current setup, is used
	// as the starting origin (economics, verifier task) instead of the
	// engine's prediction; the engine's prediction is still reported.
	CurrentOrigin *KnownOrigin `json:"current_origin,omitempty"`
	// ActualChange, when known (backtests), is what the manufacturer really
	// did; the result compares it with the best plan.
	ActualChange *ActualChange `json:"actual_change,omitempty"`
	// Regime is the origin regime the case is routed to (the router's
	// label). When the planner has an engine for it (Options.Engines), that
	// engine predicts the current setup and every move (D540); otherwise
	// the general engine does. Empty: the general engine.
	Regime string `json:"regime,omitempty"`
}

// KnownOrigin is CBP's holding for the current setup.
type KnownOrigin struct {
	Country string `json:"country"`
	Ruling  string `json:"ruling,omitempty"`
}

// ActualChange records a real production change and CBP's ruling on it.
type ActualChange struct {
	Source          string   `json:"_source"`
	MovedTo         string   `json:"moved_to,omitempty"`
	MovedComponents []string `json:"moved_to_thailand,omitempty"`
	StillImported   []string `json:"still_from_china,omitempty"`
	// AlreadyInTarget lists parts the later ruling names as made in the target
	// country that were already made there before the change, so they did not
	// change origin and are not counted as moved (judge panel round 5, D310).
	AlreadyInTarget []string `json:"already_in_target,omitempty"`
	// StillImportedAs maps each still-imported part, in the ruling's words, to
	// the product component that holds it, so a backtest can say which of them
	// the plan moves anyway (D310).
	StillImportedAs map[string]string `json:"still_from_china_as,omitempty"`
}

// KeptPart is the product component that holds a part the real change kept
// in the old country: the StillImportedAs entry when there is one, else a
// component of the same name, ignoring case (#519). False when neither.
func (a ActualChange) KeptPart(p domain.Product, word string) (string, bool) {
	if part, ok := a.StillImportedAs[word]; ok {
		return part, true
	}
	for _, c := range p.Components {
		if strings.EqualFold(strings.TrimSpace(c.Name), strings.TrimSpace(word)) {
			return c.Name, true
		}
	}
	return "", false
}

// Economics are user inputs. Defaults in fixtures are illustrative and must be
// labelled "illustrative — editable" wherever they are shown.
type Economics struct {
	Illustrative         bool    `json:"illustrative"`
	AnnualImportValueUSD float64 `json:"annual_import_value_usd"`
	// DutyRates is the total ad-valorem duty rate by origin country (0.25 = 25%).
	DutyRates     map[string]float64 `json:"duty_rates"`
	DutyRatesNote string             `json:"duty_rates_note,omitempty"`
	// DutyRatesFromTable lists the origins whose rate came from the sourced
	// duty table rather than the user (D52).
	DutyRatesFromTable []string `json:"duty_rates_from_table,omitempty"`
	// DutyRatesLiveMFN lists the origins whose rate is the live HTS MFN rate
	// (column 1 general only, no additional duties; D65).
	DutyRatesLiveMFN []string `json:"duty_rates_live_mfn,omitempty"`
	// DutyAdditional lists, per origin, the Chapter 99 duties added to a live
	// MFN rate (D90): China's Section 301 lists and the 2026 country-wide
	// Section 301 duties.
	DutyAdditional map[string][]duty.Extra `json:"duty_additional,omitempty"`
	// GroupCosts is the annual cost of performing an operation group at a
	// plant: group ID -> plant ID -> USD.
	GroupCosts map[string]map[string]float64 `json:"group_costs"`
	// Operations are the optional operational inputs (#281): labour,
	// logistics, one-time investment, lead time and minimum orders. Absent,
	// null, {} and {"groups":{}} are inactive: the legacy calculation runs.
	Operations *opcost.Operations `json:"operations,omitempty"`
}

// Validate checks the request is complete.
func (r Request) Validate() error {
	if r.Date.IsZero() {
		return fmt.Errorf("planner: %w: date is required", domain.ErrInvalid)
	}
	if r.Target == "" || r.Family == "" {
		return fmt.Errorf("planner: %w: target and family are required", domain.ErrInvalid)
	}
	if err := r.Product.Validate(); err != nil {
		return fmt.Errorf("planner: %w", err)
	}
	return nil
}

// DutyRate is the rate for an origin, matching country names by their duty
// identity (domain.DutyCountry, #718); ok is false when the user gave no rate
// for it. When several keys name the same country (an alias and its canonical
// name) the choice is deterministic: the key that is exactly the canonical
// name, else the first in sorted key order. That tie-rule is a fallback for
// duplicates that arrive over the API: two explicit rates carry no mark of
// which one the user typed, so nothing is promised about manual versus derived.
// The money forms never write such duplicates (frontend/src/lib/duty-rates.ts
// collapses every spelling to one rate on edit) and show a note when a saved
// economics has them. Rates are only read, never merged.
func (e Economics) DutyRate(country string) (float64, bool) {
	want := domain.DutyCountry(country)
	best, found := "", false
	for c := range e.DutyRates {
		if domain.DutyCountry(c) != want {
			continue
		}
		if !found || (c == want && best != want) || (best != want && c < best) {
			best, found = c, true
		}
	}
	if !found {
		return 0, false
	}
	return e.DutyRates[best], true
}

// Date is a calendar date in JSON as "YYYY-MM-DD".
type Date struct{ time.Time }

// UnmarshalJSON parses "YYYY-MM-DD".
func (d *Date) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return fmt.Errorf("date %q: want YYYY-MM-DD", s)
	}
	d.Time = t
	return nil
}

// MarshalJSON writes "YYYY-MM-DD".
func (d Date) MarshalJSON() ([]byte, error) { return json.Marshal(d.Format(time.DateOnly)) }

// LoadRequest reads a request (e.g. a demo fixture) from a JSON file.
func LoadRequest(path string) (Request, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Request{}, fmt.Errorf("planner: read %s: %w", path, err)
	}
	var r Request
	if err := json.Unmarshal(b, &r); err != nil {
		return Request{}, fmt.Errorf("planner: decode %s: %w", path, err)
	}
	return r, r.Validate()
}
