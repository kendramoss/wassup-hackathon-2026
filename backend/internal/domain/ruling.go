package domain

import (
	"regexp"
	"time"
)

// rulingIDPattern matches CBP CROSS ruling numbers: NY "N" rulings and HQ "H"
// rulings, e.g. N357771 or H334437.
var rulingIDPattern = regexp.MustCompile(`^[NH][0-9]{6}$`)

// ValidRulingID reports whether id looks like a CBP ruling number.
func ValidRulingID(id string) bool { return rulingIDPattern.MatchString(id) }

// RulingURL is the public page for a ruling on rulings.cbp.gov.
func RulingURL(id string) string { return "https://rulings.cbp.gov/ruling/" + id }

// Ruling is an earlier CBP ruling used as precedent. Facts is the facts
// section only (CBP's analysis removed, holding sentences redacted); Holding is
// CBP's stated holding; Origin is the country CBP held, normalised.
type Ruling struct {
	ID      string    `json:"id"`
	Date    time.Time `json:"date"`
	Subject string    `json:"subject,omitempty"`
	Facts   string    `json:"facts"`
	Holding string    `json:"holding,omitempty"`
	Origin  string    `json:"origin,omitempty"`
	// Reasoning is CBP's application of the test to the facts (eval
	// experiments; empty unless the corpus attached it).
	Reasoning string `json:"reasoning,omitempty"`
}

// Validate checks the ruling has a valid ID and a date.
func (r Ruling) Validate() error {
	var errs problems
	if !ValidRulingID(r.ID) {
		errs.add("ruling id %q is not a CBP ruling number", r.ID)
	}
	if r.Date.IsZero() {
		errs.add("ruling %s: date is required", r.ID)
	}
	return errs.err()
}

// UsableAsPrecedentFor reports whether the ruling may be cited for a case
// dated caseDate: it must be strictly earlier, not the case itself, and not
// revoked or modified by CBP before caseDate (D320, D370). A ruling revoked
// on or after caseDate was still CBP's position then and stays usable, so
// backtests keep the evidence a reader had at the time.
func (r Ruling) UsableAsPrecedentFor(caseID string, caseDate time.Time) bool {
	if r.ID == caseID || !r.Date.Before(caseDate) {
		return false
	}
	_, superseded := SupersededAsOf(r.ID, caseDate)
	return !superseded
}
