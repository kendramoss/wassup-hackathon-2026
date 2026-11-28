package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/textilerules"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/usmca"
)

// Score runs one case through the candidate and compares the result with the
// frozen label. A panic or an unexpected engine error becomes an error row;
// nothing is skipped and nothing is retried.
func Score(c Case) (row Row) {
	row = Row{ID: c.ID, Feature: c.Feature, Split: c.Split}
	if e, err := canon(c.Expected); err == nil {
		row.Expected = e
	} else {
		row.Expected = c.Expected
	}
	defer func() {
		if r := recover(); r != nil {
			row.Status, row.Why, row.Actual = StatusError, fmt.Sprintf("panic: %v", r), nil
		}
	}()
	var actual any
	var err error
	switch c.Feature {
	case "usmca_rules":
		actual, err = runUSMCA(c)
	case "textile_rules":
		actual, err = runTextile(c)
	case "tariff_watch_rates":
		actual, err = runTariff(c)
	default:
		err = fmt.Errorf("unknown feature %q", c.Feature)
	}
	if err != nil {
		row.Status, row.Why = StatusError, err.Error()
		return row
	}
	b, err := marshal(actual)
	if err != nil {
		row.Status, row.Why = StatusError, err.Error()
		return row
	}
	row.Actual, _ = canon(b)
	ok, why := compareRow(c, row.Expected, row.Actual)
	if ok {
		row.Status = StatusMatch
	} else {
		row.Status, row.Why = StatusMismatch, why
	}
	return row
}

func compareRow(c Case, exp, act []byte) (bool, string) {
	switch c.Feature {
	case "usmca_rules":
		return compareUSMCA(exp, act)
	case "textile_rules":
		return compareTextile(exp, act)
	}
	return compareTariff(exp, act)
}

// ---------------------------------------------------------------- USMCA

type usmcaBody struct {
	Ruleset string          `json:"ruleset"`
	Input   json.RawMessage `json:"input"`
	Raw     *string         `json:"raw"`
}

type usmcaActual struct {
	Status       string            `json:"status,omitempty"`
	Alternatives map[string]string `json:"alternatives,omitempty"`
	RVCStatus    string            `json:"rvc_status,omitempty"`
	RVCExact     string            `json:"rvc_exact,omitempty"`
	Needs        []string          `json:"needs,omitempty"`
	Invalid      bool              `json:"invalid,omitempty"`
	ErrorField   string            `json:"error_field,omitempty"`
}

type usmcaExpected struct {
	Status       string            `json:"status"`
	Alternatives map[string]string `json:"alternatives"`
	RVCStatus    *string           `json:"rvc_status"`
	RVCExact     *string           `json:"rvc_exact"`
	Needs        []string          `json:"needs"`
	Invalid      bool              `json:"invalid"`
	ErrorField   string            `json:"error_field"`
}

// needKinds names the information that would resolve an open question,
// independent of the candidate's own codes.
var needKinds = map[string]string{
	"profile_unconfirmed":             "profile_confirmation",
	"insufficient_precision":          "more_digits_good_code",
	"origin_status_unknown":           "material_origin_status",
	"material_hs_unknown":             "material_code",
	"material_hs_imprecise":           "material_code",
	"materials_incomplete":            "materials_list_confirmation",
	"rvc_method_required":             "rvc_method",
	"alternative_unmodeled":           "manual_check",
	"sibling_part_unconfirmed":        "sibling_part",
	"material_value_unknown":          "material_value",
	"de_minimis_basis_required":       "de_minimis_basis",
	"de_minimis_base_unknown":         "de_minimis_base",
	"de_minimis_base_zero":            "de_minimis_base",
	"produced_in_usmca_unconfirmed":   "produced_in_usmca",
	"parts_route_basis_required":      "parts_route_basis",
	"parts_classification_unknown":    "parts_classification",
	"imported_from_usmca_unconfirmed": "imported_from_usmca",
}

func runUSMCA(c Case) (any, error) {
	var body usmcaBody
	if err := json.Unmarshal(c.Input, &body); err != nil {
		return nil, err
	}
	var rs *usmca.RuleSet
	var err error
	switch body.Ruleset {
	case "pilot":
		rs, err = usmca.DefaultRules()
	case "extended":
		rs, err = usmca.ExtendedRules()
	default:
		return nil, fmt.Errorf("unknown ruleset %q", body.Ruleset)
	}
	if err != nil {
		return nil, err
	}
	raw := []byte(body.Input)
	if body.Raw != nil {
		raw = []byte(*body.Raw)
	}
	in, err := usmca.DecodeInput(raw)
	var out usmca.Output
	if err == nil {
		out, err = rs.Check(in)
	}
	if err != nil {
		var ie *usmca.InputError
		if errors.As(err, &ie) {
			return usmcaActual{Invalid: true, ErrorField: ie.Field}, nil
		}
		return nil, err
	}
	a := usmcaActual{Status: string(out.Status), Alternatives: map[string]string{}}
	for _, alt := range out.Alternatives {
		a.Alternatives[alt.ID] = string(alt.Result)
	}
	if out.RVC != nil {
		a.RVCStatus, a.RVCExact = string(out.RVC.Status), out.RVC.Exact
	}
	kinds := map[string]bool{}
	for _, f := range out.MissingFacts {
		if k, ok := needKinds[f.Code]; ok {
			kinds[k] = true
		} else {
			kinds["other:"+f.Code] = true
		}
	}
	if out.RVC != nil && out.RVC.Status != usmca.StatusComputed {
		kinds["rvc_values"] = true
	}
	a.Needs = sortedKeys(kinds)
	return a, nil
}

func compareUSMCA(exp, act []byte) (bool, string) {
	var e usmcaExpected
	var a usmcaActual
	if err := json.Unmarshal(exp, &e); err != nil {
		return false, "unreadable expected label: " + err.Error()
	}
	if err := json.Unmarshal(act, &a); err != nil {
		return false, "unreadable actual: " + err.Error()
	}
	if e.Invalid {
		if !a.Invalid {
			return false, fmt.Sprintf("input expected to be refused (%s) but the check answered %q", e.ErrorField, a.Status)
		}
		if a.ErrorField != e.ErrorField {
			return false, fmt.Sprintf("refused on field %q, expected %q", a.ErrorField, e.ErrorField)
		}
		return true, ""
	}
	if a.Invalid {
		return false, fmt.Sprintf("input refused on field %q but an answer was expected", a.ErrorField)
	}
	var why []string
	if a.Status != e.Status {
		why = append(why, fmt.Sprintf("status %q, expected %q", a.Status, e.Status))
	}
	ids := make([]string, 0, len(e.Alternatives))
	for id := range e.Alternatives {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if got, ok := a.Alternatives[id]; !ok {
			why = append(why, fmt.Sprintf("alternative %s missing, expected %q", id, e.Alternatives[id]))
		} else if got != e.Alternatives[id] {
			why = append(why, fmt.Sprintf("alternative %s %q, expected %q", id, got, e.Alternatives[id]))
		}
	}
	if e.RVCStatus != nil && a.RVCStatus != *e.RVCStatus {
		why = append(why, fmt.Sprintf("rvc status %q, expected %q", a.RVCStatus, *e.RVCStatus))
	}
	if e.RVCExact != nil && a.RVCExact != *e.RVCExact {
		why = append(why, fmt.Sprintf("rvc %q, expected %q", a.RVCExact, *e.RVCExact))
	}
	if e.RVCStatus != nil && e.RVCExact == nil && a.RVCExact != "" {
		why = append(why, fmt.Sprintf("rvc %q where none is defined", a.RVCExact))
	}
	have := map[string]bool{}
	for _, n := range a.Needs {
		have[n] = true
	}
	for _, n := range e.Needs {
		if !have[n] {
			why = append(why, fmt.Sprintf("does not ask for %s (asks for %v)", n, a.Needs))
		}
	}
	return len(why) == 0, strings.Join(why, "; ")
}

// ---------------------------------------------------------------- textile

type textileActual struct {
	Status string `json:"status"`
	Origin string `json:"origin,omitempty"`
	Para   string `json:"para,omitempty"`
	Class  string `json:"class,omitempty"`
}

func decodeFacts(raw json.RawMessage) (textilerules.Facts, error) {
	var body struct {
		Facts json.RawMessage `json:"facts"`
	}
	var f textilerules.Facts
	if err := json.Unmarshal(raw, &body); err != nil {
		return f, err
	}
	dec := json.NewDecoder(bytes.NewReader(body.Facts))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return f, err
	}
	return f, nil
}

func runTextile(c Case) (any, error) {
	f, err := decodeFacts(c.Input)
	if err != nil {
		return nil, err
	}
	rules, err := textilerules.Default()
	if err != nil {
		return nil, err
	}
	r := rules.Evaluate(f)
	return textileActual{Status: r.Status, Origin: r.Origin, Para: r.Para, Class: r.Class}, nil
}

func compareTextile(exp, act []byte) (bool, string) {
	var e, a textileActual
	if err := json.Unmarshal(exp, &e); err != nil {
		return false, "unreadable expected label: " + err.Error()
	}
	if err := json.Unmarshal(act, &a); err != nil {
		return false, "unreadable actual: " + err.Error()
	}
	var why []string
	if a.Status != e.Status {
		why = append(why, fmt.Sprintf("status %q, expected %q", a.Status, e.Status))
	}
	if a.Origin != e.Origin {
		why = append(why, fmt.Sprintf("origin %q, expected %q", a.Origin, e.Origin))
	}
	if e.Para != "" && a.Para != e.Para {
		why = append(why, fmt.Sprintf("paragraph %q, expected %q", a.Para, e.Para))
	}
	return len(why) == 0, strings.Join(why, "; ")
}

// ---------------------------------------------------------------- numbers

// rate1e4 canonicalises an ad valorem fraction to whole units of 0.0001
// (half away from zero), so a last-bit float difference cannot reach a result
// row or a comparison.
func rate1e4(r float64) int64 {
	return int64(math.Round(math.Round(r*1e9) / 1e5))
}

// toCents canonicalises a dollar amount to whole cents the same way.
func toCents(v float64) int64 {
	return int64(math.Round(math.Round(v*1e6) / 1e4))
}

func parseDay(s string) (time.Time, error) { return time.Parse(time.DateOnly, s) }
