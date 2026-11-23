package usmca

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

// SchemaVersion is the only rule-data schema this loader understands.
const SchemaVersion = 1

// Condition kinds. The four change_from_* kinds are deliberately distinct:
// each compares exactly the leading 2, 4, 6 or 8 digits of the good against
// each non-originating material.
const (
	KindChangeFromChapter    = "change_from_chapter"
	KindChangeFromHeading    = "change_from_heading"
	KindChangeFromSubheading = "change_from_subheading"
	KindChangeFromTariffItem = "change_from_tariff_item"
	KindForbiddenInputs      = "forbidden_inputs"
	KindSiblingParts         = "sibling_parts"
	KindAllInputsFrom        = "all_inputs_from"
	KindNoShiftRequired      = "no_shift_required"
	KindRVC                  = "rvc"
)

// Levels usable in Condition.SameAsGoodLevel.
const (
	LevelChapter    = "chapter"
	LevelHeading    = "heading"
	LevelSubheading = "subheading"
	LevelTariffItem = "tariff_item"
)

func levelDigits(level string) int {
	switch level {
	case LevelChapter:
		return 2
	case LevelHeading:
		return 4
	case LevelSubheading:
		return 6
	case LevelTariffItem:
		return 8
	}
	return 0
}

// DataFile is the versioned rule data record (embedded JSON).
type DataFile struct {
	SchemaVersion int      `json:"schema_version"`
	DataVersion   string   `json:"data_version"`
	Note          string   `json:"note,omitempty"`
	Snapshot      Snapshot `json:"snapshot"`
	Rows          []Row    `json:"rows"`
}

// Snapshot identifies the verified source the rows were taken from.
type Snapshot struct {
	Source      string `json:"source"`
	Revision    string `json:"revision"`
	RetrievedOn string `json:"retrieved_on"`
	SourceHash  string `json:"source_hash"`
}

// Row is one product-specific rule keyed by a 4/6/8-digit code prefix.
type Row struct {
	ID           string        `json:"id"`
	Code         string        `json:"code"`
	Description  string        `json:"description"`
	Source       RowSource     `json:"source"`
	Alternatives []Alternative `json:"alternatives"`
}

// RowSource keeps the exact source excerpt next to the modeled conditions.
type RowSource struct {
	Citation string `json:"citation"`
	Excerpt  string `json:"excerpt"`
}

// Alternative is an AND over Conditions; a Row is an OR over Alternatives.
type Alternative struct {
	ID         string      `json:"id"`
	Label      string      `json:"label"`
	Note       string      `json:"note,omitempty"`
	Conditions []Condition `json:"conditions"`
}

// Thresholds are separate minimum RVC percentages per method. A nil entry
// means the alternative does not allow that method.
type Thresholds struct {
	TransactionValue *string `json:"transaction_value,omitempty"`
	NetCost          *string `json:"net_cost,omitempty"`
}

// Condition is one predicate. Which fields apply depends on Kind; the loader
// rejects stray fields.
type Condition struct {
	Kind string `json:"kind"`
	// change_from_*: materials under these prefixes do not count as a shift.
	ExcludePrefixes []string `json:"exclude_prefixes,omitempty"`
	// forbidden_inputs: non-originating materials under Prefixes, or sharing
	// SameAsGoodLevel with the good, fail the condition.
	Prefixes        []string `json:"prefixes,omitempty"`
	SameAsGoodLevel string   `json:"same_as_good_level,omitempty"`
	// sibling_parts: at least one non-originating material under
	// AtLeastOneFrom AND every other non-originating material under OthersFrom
	// OR outside chapter OthersOutsideChapter.
	AtLeastOneFrom       []string    `json:"at_least_one_from,omitempty"`
	OthersFrom           []string    `json:"others_from,omitempty"`
	OthersOutsideChapter string      `json:"others_outside_chapter,omitempty"`
	Thresholds           *Thresholds `json:"thresholds,omitempty"`
}

// RuleSet is an immutable, validated rule data record. Safe for concurrent use.
type RuleSet struct {
	data DataFile
}

// Version returns the data version string.
func (rs *RuleSet) Version() string { return rs.data.DataVersion }

// Snapshot returns the source snapshot header.
func (rs *RuleSet) Snapshot() Snapshot { return rs.data.Snapshot }

// Rows returns a copy of the row list.
func (rs *RuleSet) Rows() []Row {
	out := make([]Row, len(rs.data.Rows))
	for i, row := range rs.data.Rows {
		row.Alternatives = cloneAlternatives(row.Alternatives)
		out[i] = row
	}
	return out
}

func cloneAlternatives(in []Alternative) []Alternative {
	out := make([]Alternative, len(in))
	for i, a := range in {
		conds := make([]Condition, len(a.Conditions))
		for j, c := range a.Conditions {
			c.ExcludePrefixes = append([]string(nil), c.ExcludePrefixes...)
			c.Prefixes = append([]string(nil), c.Prefixes...)
			c.AtLeastOneFrom = append([]string(nil), c.AtLeastOneFrom...)
			c.OthersFrom = append([]string(nil), c.OthersFrom...)
			if c.Thresholds != nil {
				t := Thresholds{}
				if c.Thresholds.TransactionValue != nil {
					v := *c.Thresholds.TransactionValue
					t.TransactionValue = &v
				}
				if c.Thresholds.NetCost != nil {
					v := *c.Thresholds.NetCost
					t.NetCost = &v
				}
				c.Thresholds = &t
			}
			conds[j] = c
		}
		a.Conditions = conds
		out[i] = a
	}
	return out
}

//go:embed data/usmca_rules.json
var embeddedRules []byte

var (
	defaultOnce  sync.Once
	defaultRules *RuleSet
	defaultErr   error
)

// DefaultRules loads the embedded rule data once.
func DefaultRules() (*RuleSet, error) {
	defaultOnce.Do(func() { defaultRules, defaultErr = LoadRules(embeddedRules) })
	return defaultRules, defaultErr
}

// LoadRules strictly decodes and validates a rule data record.
func LoadRules(data []byte) (*RuleSet, error) {
	var df DataFile
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&df); err != nil {
		return nil, fmt.Errorf("usmca rules: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("usmca rules: trailing data")
	}
	if err := validateData(df); err != nil {
		return nil, fmt.Errorf("usmca rules: %w", err)
	}
	return &RuleSet{data: df}, nil
}

func validateData(df DataFile) error {
	if df.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported schema_version %d", df.SchemaVersion)
	}
	if df.DataVersion == "" {
		return errors.New("data_version is required")
	}
	if len(df.Rows) > 0 && (df.Snapshot.Source == "" || df.Snapshot.Revision == "" || df.Snapshot.SourceHash == "" || df.Snapshot.RetrievedOn == "") {
		return errors.New("snapshot source, revision, retrieved_on and source_hash are required when rows exist")
	}
	ids := map[string]bool{}
	codes := map[string]bool{}
	for _, r := range df.Rows {
		if r.ID == "" || ids[r.ID] {
			return fmt.Errorf("row id %q is empty or duplicated", r.ID)
		}
		ids[r.ID] = true
		if !isDigits(r.Code) || (len(r.Code) != 4 && len(r.Code) != 6 && len(r.Code) != 8) {
			return fmt.Errorf("row %s: code must be 4, 6 or 8 digits", r.ID)
		}
		if codes[r.Code] {
			return fmt.Errorf("row %s: duplicate code %s", r.ID, r.Code)
		}
		codes[r.Code] = true
		if r.Source.Citation == "" || r.Source.Excerpt == "" {
			return fmt.Errorf("row %s: source citation and excerpt are required", r.ID)
		}
		if len(r.Alternatives) == 0 {
			return fmt.Errorf("row %s: at least one alternative is required", r.ID)
		}
		altIDs := map[string]bool{}
		for _, a := range r.Alternatives {
			if a.ID == "" || len(a.Conditions) == 0 {
				return fmt.Errorf("row %s: alternative needs an id and conditions", r.ID)
			}
			if altIDs[a.ID] {
				return fmt.Errorf("row %s: duplicate alternative id %q", r.ID, a.ID)
			}
			altIDs[a.ID] = true
			for _, c := range a.Conditions {
				if err := validateCondition(c); err != nil {
					return fmt.Errorf("row %s alternative %s: %w", r.ID, a.ID, err)
				}
			}
		}
	}
	return nil
}

func validPrefixes(ps []string) error {
	for _, p := range ps {
		if !isDigits(p) || len(p) < 2 || len(p) > 10 || len(p)%2 != 0 {
			return fmt.Errorf("invalid code prefix %q", p)
		}
	}
	return nil
}

func validateCondition(c Condition) error {
	hasExcl := len(c.ExcludePrefixes) > 0
	hasForb := len(c.Prefixes) > 0 || c.SameAsGoodLevel != ""
	hasSib := len(c.AtLeastOneFrom) > 0 || len(c.OthersFrom) > 0 || c.OthersOutsideChapter != ""
	hasRVC := c.Thresholds != nil
	if err := validPrefixes(append(append(append(append([]string{}, c.ExcludePrefixes...), c.Prefixes...), c.AtLeastOneFrom...), c.OthersFrom...)); err != nil {
		return err
	}
	only := func(others ...bool) error {
		for _, o := range others {
			if o {
				return fmt.Errorf("kind %s has fields that do not apply", c.Kind)
			}
		}
		return nil
	}
	switch c.Kind {
	case KindChangeFromChapter, KindChangeFromHeading, KindChangeFromSubheading, KindChangeFromTariffItem:
		return only(hasForb, hasSib, hasRVC)
	case KindForbiddenInputs:
		if err := only(hasExcl, hasSib, hasRVC); err != nil {
			return err
		}
		if !hasForb {
			return errors.New("forbidden_inputs needs prefixes or same_as_good_level")
		}
		if c.SameAsGoodLevel != "" && levelDigits(c.SameAsGoodLevel) == 0 {
			return fmt.Errorf("unknown level %q", c.SameAsGoodLevel)
		}
	case KindSiblingParts:
		if err := only(hasExcl, hasForb, hasRVC); err != nil {
			return err
		}
		if len(c.AtLeastOneFrom) == 0 {
			return errors.New("sibling_parts needs at_least_one_from")
		}
		if c.OthersOutsideChapter != "" && (len(c.OthersOutsideChapter) != 2 || !isDigits(c.OthersOutsideChapter)) {
			return errors.New("others_outside_chapter must be a 2-digit chapter")
		}
	case KindAllInputsFrom:
		if err := only(hasExcl, hasForb, hasRVC, len(c.AtLeastOneFrom) > 0); err != nil {
			return err
		}
		if len(c.OthersFrom) == 0 && c.OthersOutsideChapter == "" {
			return errors.New("all_inputs_from needs others_from or others_outside_chapter")
		}
		if c.OthersOutsideChapter != "" && (len(c.OthersOutsideChapter) != 2 || !isDigits(c.OthersOutsideChapter)) {
			return errors.New("others_outside_chapter must be a 2-digit chapter")
		}
	case KindNoShiftRequired:
		return only(hasExcl, hasForb, hasSib, hasRVC)
	case KindRVC:
		if err := only(hasExcl, hasForb, hasSib); err != nil {
			return err
		}
		if c.Thresholds == nil || (c.Thresholds.TransactionValue == nil && c.Thresholds.NetCost == nil) {
			return errors.New("rvc needs at least one method threshold")
		}
		for _, th := range []*string{c.Thresholds.TransactionValue, c.Thresholds.NetCost} {
			if th == nil {
				continue
			}
			v, err := ParseDecimal(*th, false)
			if err != nil {
				return fmt.Errorf("rvc threshold: %w", err)
			}
			if v.Cmp(bigHundred) > 0 {
				return errors.New("rvc threshold exceeds 100")
			}
		}
	default:
		return fmt.Errorf("unknown condition kind %q", c.Kind)
	}
	return nil
}

func isDigits(s string) bool { return s != "" && allDigits(s) }
