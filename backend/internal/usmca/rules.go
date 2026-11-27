package usmca

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
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
	// KindUnmodeled holds an alternative whose wording carries a product,
	// process or end-use condition the engine does not model (generated
	// GN 11(o) rows only). It always evaluates to unknown.
	KindUnmodeled = "unmodeled"
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
	// Deemed lists the goods GN 11(p) deems originating when imported from
	// another USMCA country (extended rules only).
	Deemed *Deemed `json:"deemed_originating,omitempty"`
}

// Deemed is GN 11(p): goods deemed originating, notwithstanding subdivision (o).
type Deemed struct {
	Citation string        `json:"citation"`
	Excerpt  string        `json:"excerpt"`
	Goods    []DeemedGoods `json:"goods"`
}

// DeemedGoods is one line of the GN 11(p) table.
type DeemedGoods struct {
	Name  string   `json:"name"`
	Codes []string `json:"codes"`
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
	ID   string `json:"id"`
	Code string `json:"code"`
	// CodeTo, when set, makes the row cover every code from Code to CodeTo
	// inclusive at Code's length ("subheadings 8401.10 through 8401.30").
	CodeTo string `json:"code_to,omitempty"`
	// Unsupported, when set, answers not_supported for the row's goods with
	// this reason (an end-use or vehicle-type rule, or an automotive good).
	Unsupported  string        `json:"unsupported,omitempty"`
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
	// change_from_*: materials under these codes or inclusive "lo-hi" code
	// ranges do not count as a shift either.
	ExcludeRanges []string `json:"exclude_ranges,omitempty"`
	// forbidden_inputs: non-originating materials under Prefixes, or sharing
	// SameAsGoodLevel with the good, fail the condition.
	Prefixes        []string `json:"prefixes,omitempty"`
	SameAsGoodLevel string   `json:"same_as_good_level,omitempty"`
	// sibling_parts: at least one non-originating material under
	// AtLeastOneFrom AND every other non-originating material under OthersFrom
	// OR outside chapter OthersOutsideChapter.
	AtLeastOneFrom       []string `json:"at_least_one_from,omitempty"`
	OthersFrom           []string `json:"others_from,omitempty"`
	OthersOutsideChapter string   `json:"others_outside_chapter,omitempty"`
	// all_inputs_from: a material outside the good's chapter, heading,
	// subheading or tariff_item at this level also satisfies the condition.
	// OthersFrom entries may be codes or inclusive "lo-hi" ranges.
	OthersOutsideLevel string `json:"others_outside_level,omitempty"`
	// unmodeled: the verbatim alternative text.
	Text       string      `json:"text,omitempty"`
	Thresholds *Thresholds `json:"thresholds,omitempty"`
}

// RuleSet is an immutable, validated rule data record. Safe for concurrent use.
type RuleSet struct {
	data DataFile
	// extended turns on de minimis and the parts route (ExtendedRules).
	extended bool
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
			c.ExcludeRanges = append([]string(nil), c.ExcludeRanges...)
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

//go:embed data/usmca_rules_ext.json
var embeddedExtRules []byte

var (
	extOnce  sync.Once
	extRules *RuleSet
	extErr   error
)

// ExtendedRules is the pilot rule data plus the rows generated from GN 11(o)
// (usmca_rules_ext.json). A pilot row replaces a generated row for exactly
// the same single code; a generated range that contains a pilot code is kept
// and the pilot row wins inside it as the more specific match. It also turns on de minimis and the parts route.
func ExtendedRules() (*RuleSet, error) {
	extOnce.Do(func() {
		pilot, err := DefaultRules()
		if err != nil {
			extErr = err
			return
		}
		gen, err := LoadRules(embeddedExtRules)
		if err != nil {
			extErr = err
			return
		}
		extRules, extErr = mergeRules(pilot, gen)
	})
	return extRules, extErr
}

func mergeRules(pilot, gen *RuleSet) (*RuleSet, error) {
	df := gen.data
	df.Note = pilot.data.Note + " " + gen.data.Note
	df.Rows = append([]Row(nil), pilot.data.Rows...)
	for _, r := range gen.data.Rows {
		clash := false
		for _, p := range pilot.data.Rows {
			if p.Code == r.Code && (r.CodeTo == "" || r.CodeTo == r.Code) {
				clash = true
			}
		}
		if !clash {
			df.Rows = append(df.Rows, r)
		}
	}
	if err := validateData(df); err != nil {
		return nil, fmt.Errorf("usmca rules: merged: %w", err)
	}
	return &RuleSet{data: df, extended: true}, nil
}

var extendedOn atomic.Bool

// SetExtended chooses the rule set Check uses: the pilot rows only (false,
// the default) or ExtendedRules (true). The server sets it once at start
// from USMCA_RULES_EXT.
func SetExtended(on bool) { extendedOn.Store(on) }

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
	if df.Deemed != nil {
		if df.Deemed.Citation == "" || df.Deemed.Excerpt == "" || len(df.Deemed.Goods) == 0 {
			return errors.New("deemed_originating needs a citation, an excerpt and goods")
		}
		for _, g := range df.Deemed.Goods {
			if g.Name == "" || len(g.Codes) == 0 {
				return errors.New("deemed_originating goods need a name and codes")
			}
			if err := validPrefixes(g.Codes); err != nil {
				return err
			}
		}
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
		if r.CodeTo != "" && (len(r.CodeTo) != len(r.Code) || !isDigits(r.CodeTo) || r.CodeTo < r.Code) {
			return fmt.Errorf("row %s: code_to must have code's length and not precede it", r.ID)
		}
		key := r.Code + "-" + r.CodeTo
		if codes[key] {
			return fmt.Errorf("row %s: duplicate code %s", r.ID, key)
		}
		codes[key] = true
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

// validRanges accepts codes and inclusive "lo-hi" ranges of equal length.
func validRanges(rs []string) error {
	for _, r := range rs {
		lo, hi, isRange := strings.Cut(r, "-")
		if err := validPrefixes([]string{lo}); err != nil {
			return err
		}
		if isRange && (len(hi) != len(lo) || !isDigits(hi) || hi < lo) {
			return fmt.Errorf("invalid code range %q", r)
		}
	}
	return nil
}

func validateCondition(c Condition) error {
	hasExcl := len(c.ExcludePrefixes) > 0 || len(c.ExcludeRanges) > 0
	hasForb := len(c.Prefixes) > 0 || c.SameAsGoodLevel != ""
	hasSib := len(c.AtLeastOneFrom) > 0 || len(c.OthersFrom) > 0 || c.OthersOutsideChapter != "" || c.OthersOutsideLevel != ""
	hasRVC := c.Thresholds != nil
	hasText := c.Text != ""
	if err := validPrefixes(append(append(append([]string{}, c.ExcludePrefixes...), c.Prefixes...), c.AtLeastOneFrom...)); err != nil {
		return err
	}
	if err := validRanges(append(append([]string{}, c.OthersFrom...), c.ExcludeRanges...)); err != nil {
		return err
	}
	if hasText && c.Kind != KindUnmodeled {
		return fmt.Errorf("kind %s has fields that do not apply", c.Kind)
	}
	if c.OthersOutsideLevel != "" && (c.Kind != KindAllInputsFrom || levelDigits(c.OthersOutsideLevel) == 0) {
		return fmt.Errorf("others_outside_level %q does not apply", c.OthersOutsideLevel)
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
		if len(c.OthersFrom) == 0 && c.OthersOutsideChapter == "" && c.OthersOutsideLevel == "" {
			return errors.New("all_inputs_from needs others_from, others_outside_chapter or others_outside_level")
		}
		if c.OthersOutsideChapter != "" && c.OthersOutsideLevel != "" {
			return errors.New("all_inputs_from takes others_outside_chapter or others_outside_level, not both")
		}
		if c.OthersOutsideChapter != "" && (len(c.OthersOutsideChapter) != 2 || !isDigits(c.OthersOutsideChapter)) {
			return errors.New("others_outside_chapter must be a 2-digit chapter")
		}
	case KindNoShiftRequired:
		return only(hasExcl, hasForb, hasSib, hasRVC)
	case KindUnmodeled:
		if err := only(hasExcl, hasForb, hasSib, hasRVC); err != nil {
			return err
		}
		if !hasText {
			return errors.New("unmodeled needs text")
		}
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
