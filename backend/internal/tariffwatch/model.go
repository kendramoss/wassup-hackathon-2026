package tariffwatch

import "time"

// Component kinds.
const (
	KindMFN        = "mfn"
	KindSection301 = "section_301"
	KindSection232 = "section_232"
	KindOther      = "other"
)

// Component is one duty in an origin's rate.
type Component struct {
	Kind    string  `json:"kind"`
	Name    string  `json:"name"`
	Heading string  `json:"heading,omitempty"`
	Rate    float64 `json:"rate"` // ad valorem fraction: 0.25 = 25%
	// From and Through bound a dated action (YYYY-MM-DD); empty when the
	// duty has no date in the data.
	From    string `json:"from,omitempty"`
	Through string `json:"through,omitempty"`
}

// Exclusion is a Section 301 product exclusion the pricing reported (valid,
// to check, or lapsed). It does not change the rate: a described-product
// exclusion is only ever reported (D90).
type Exclusion struct {
	Heading string `json:"heading"`
	HTS     string `json:"hts"`
	EndDate string `json:"end_date,omitempty"`
	Status  string `json:"status,omitempty"` // "", "expired" or "review"
}

// OriginBasis is the duty an origin was priced at.
type OriginBasis struct {
	Origin string `json:"origin"`
	// Rate is the total the money was worked out with: the user's, or the
	// table's or live rate the pricing filled in.
	Rate float64 `json:"rate"`
	// Source is "table" (the researched duty table), "live_mfn" (the live
	// HTS MFN rate plus Chapter 99 duties) or "user" (a rate the user gave:
	// nothing to compare it with).
	Source     string      `json:"source"`
	MFN        *float64    `json:"mfn,omitempty"`
	Components []Component `json:"components,omitempty"`
	Exclusions []Exclusion `json:"exclusions,omitempty"`
}

// Section232 says whether the snapshot covers Section 232. The duty code does
// not compute it (it is a "check before relying" warning, D65, D90), so a
// snapshot made by the server records Modelled false and a newer data set
// that carries Section 232 coverage is reported as a change to review, never
// as a rate change.
type Section232 struct {
	Modelled bool   `json:"modelled"`
	Covered  bool   `json:"covered,omitempty"`
	Note     string `json:"note,omitempty"`
}

// Basis is the snapshot of the duty basis a case was priced on.
type Basis struct {
	HTS         string    `json:"hts"`
	HTSRevision string    `json:"hts_revision"`
	PricedAt    time.Time `json:"priced_at"`
	// AsOf is the entry date the duties were judged on (YYYY-MM-DD).
	AsOf            string        `json:"as_of"`
	AnnualImportUSD float64       `json:"annual_import_usd,omitempty"`
	Origins         []OriginBasis `json:"origins"`
	Section232      Section232    `json:"section_232"`
}

// Origin returns the basis of one origin (matched exactly on the stored
// name).
func (b Basis) Origin(name string) (OriginBasis, bool) {
	for _, o := range b.Origins {
		if o.Origin == name {
			return o, true
		}
	}
	return OriginBasis{}, false
}

// Change kinds.
const (
	ChangeRate            = "rate_changed"
	ChangeMFN             = "mfn_changed"
	ChangeAdded           = "duty_added"
	ChangeRemoved         = "duty_removed"
	ChangeWindow          = "duty_window_changed"
	ChangeExclusionWindow = "exclusion_window_changed"
	ChangeExclusionAdded  = "exclusion_added"
	ChangeExclusionGone   = "exclusion_removed"
	Change232             = "section_232_coverage"
)

// Change is one thing that differs between the basis a case was priced on
// and a newer data set.
type Change struct {
	Origin  string `json:"origin"`
	Kind    string `json:"kind"`
	Heading string `json:"heading,omitempty"`
	Name    string `json:"name,omitempty"`
	// OldRate and NewRate are set for the kinds that move a rate.
	OldRate *float64 `json:"old_rate,omitempty"`
	NewRate *float64 `json:"new_rate,omitempty"`
	Old     string   `json:"old,omitempty"`
	New     string   `json:"new,omitempty"`
	// MovesRate is false for a change that does not alter the rate the
	// money uses (an exclusion window, Section 232 coverage).
	MovesRate bool   `json:"moves_rate"`
	Note      string `json:"note,omitempty"`
}

// RateDelta is one origin's old and new total.
type RateDelta struct {
	Origin  string  `json:"origin"`
	OldRate float64 `json:"old_rate"`
	NewRate float64 `json:"new_rate"`
	Delta   float64 `json:"delta"`
}

// AnnualDuty is the annual duty of the current setup before and after.
type AnnualDuty struct {
	Origin string  `json:"origin"`
	Before float64 `json:"before_usd"`
	After  float64 `json:"after_usd"`
	Change float64 `json:"change_usd"`
}

// PlanDuty is the duty effect of the best shown plan before and after: the
// plan's duty_effect_per_year, which is import value times the rate at the
// current origin minus the rate at the plan's origin.
type PlanDuty struct {
	From   string  `json:"from"`
	To     string  `json:"to"`
	Before float64 `json:"before_usd"`
	After  float64 `json:"after_usd"`
	Change float64 `json:"change_usd"`
}

// Result is what one check records on a case.
type Result struct {
	CheckedAt       time.Time `json:"checked_at"`
	DatasetRevision string    `json:"dataset_revision"`
	// BasisRevision is the revision the case was priced on.
	BasisRevision string `json:"basis_revision"`
	AsOf          string `json:"as_of"`
	// Affected is true when anything in Changes differs.
	Affected bool        `json:"affected"`
	Changes  []Change    `json:"changes"`
	Rates    []RateDelta `json:"rates,omitempty"`
	Annual   *AnnualDuty `json:"annual_duty,omitempty"`
	Plan     *PlanDuty   `json:"plan_duty,omitempty"`
	Notes    []string    `json:"notes,omitempty"`
}
