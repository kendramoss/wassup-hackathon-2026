package planner

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
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
	// ActualChange, when known (backtests), is what the manufacturer really
	// did; the result compares it with the best plan.
	ActualChange *ActualChange `json:"actual_change,omitempty"`
}

// ActualChange records a real production change and CBP's ruling on it.
type ActualChange struct {
	Source          string   `json:"_source"`
	MovedTo         string   `json:"moved_to,omitempty"`
	MovedComponents []string `json:"moved_to_thailand,omitempty"`
	StillImported   []string `json:"still_from_china,omitempty"`
}

// Economics are user inputs. Defaults in fixtures are illustrative and must be
// labelled "illustrative — editable" wherever they are shown.
type Economics struct {
	Illustrative         bool    `json:"illustrative"`
	AnnualImportValueUSD float64 `json:"annual_import_value_usd"`
	// DutyRates is the total ad-valorem duty rate by origin country (0.25 = 25%).
	DutyRates     map[string]float64 `json:"duty_rates"`
	DutyRatesNote string             `json:"duty_rates_note,omitempty"`
	// GroupCosts is the annual cost of performing an operation group at a
	// plant: group ID -> plant ID -> USD.
	GroupCosts map[string]map[string]float64 `json:"group_costs"`
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

// DutyRate is the rate for an origin, normalising country names; ok is false
// when the user gave no rate for it.
func (e Economics) DutyRate(country string) (float64, bool) {
	want := domain.NormalizeCountry(country)
	for c, r := range e.DutyRates {
		if domain.NormalizeCountry(c) == want {
			return r, true
		}
	}
	return 0, false
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
