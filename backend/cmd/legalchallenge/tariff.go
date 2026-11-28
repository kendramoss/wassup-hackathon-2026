package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffwatch"
)

type rawDS json.RawMessage

func (d *rawDS) UnmarshalJSON(b []byte) error { *d = append((*d)[0:0], b...); return nil }
func (d rawDS) MarshalJSON() ([]byte, error)  { return d, nil }

// decode reads and validates a synthetic data set; an error is a refusal.
func (d rawDS) decode() (*tariffwatch.Dataset, error) {
	var ds tariffwatch.Dataset
	if err := json.Unmarshal(d, &ds); err != nil {
		return nil, err
	}
	if _, err := tariffwatch.NewEvaluator(&ds); err != nil {
		return nil, err
	}
	return &ds, nil
}

type tariffInput struct {
	Kind       string            `json:"kind"`
	Basis      tariffwatch.Basis `json:"basis"`
	Dataset    rawDS             `json:"dataset"`
	AsOf       string            `json:"as_of"`
	BaseOrigin string            `json:"base_origin"`
	Plan       *struct {
		To                string  `json:"to"`
		DutyEffectPerYear float64 `json:"duty_effect_per_year"`
	} `json:"plan"`
	Now     string `json:"now"`
	OldText string `json:"old_text"`
	NewText string `json:"new_text"`
}

type tChange struct {
	Origin  string `json:"origin"`
	Kind    string `json:"kind"`
	Heading string `json:"heading"`
	Old     *int64 `json:"old"`
	New     *int64 `json:"new"`
	Moves   bool   `json:"moves"`
}

type tRate struct {
	Origin   string `json:"origin"`
	Delta    int64  `json:"delta"`
	NewTotal int64  `json:"new_total"`
}

type tMoney struct {
	Origin string `json:"origin,omitempty"`
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
	Before int64  `json:"before"`
	After  int64  `json:"after"`
	Change int64  `json:"change"`
}

type watchActual struct {
	Affected bool      `json:"affected"`
	Changes  []tChange `json:"changes"`
	Rates    []tRate   `json:"rates"`
	Notes    []string  `json:"notes"`
	Annual   *tMoney   `json:"annual"`
	Plan     *tMoney   `json:"plan"`
}

type reviseAction struct {
	Heading     string   `json:"heading"`
	Rate        int64    `json:"rate"`
	From        string   `json:"from"`
	Through     string   `json:"through"`
	Subheadings []string `json:"subheadings"`
}

type reviseActual struct {
	Actions     []reviseAction `json:"actions"`
	NotModelled []string       `json:"not_modelled"`
	ExEnds      []string       `json:"exclusion_end_dates"`
	Warnings    int            `json:"warnings"`
	Revision    string         `json:"revision"`
}

func ptr(v int64) *int64 { return &v }

func runTariff(c Case) (any, error) {
	var in tariffInput
	if err := json.Unmarshal(c.Input, &in); err != nil {
		return nil, err
	}
	ds, err := in.Dataset.decode()
	if in.Kind == "revise" {
		return runRevise(in, ds, err)
	}
	if err != nil {
		return map[string]any{"dataset_refused": true}, nil
	}
	ev, err := tariffwatch.NewEvaluator(ds)
	if err != nil {
		return map[string]any{"dataset_refused": true}, nil
	}
	asOf, err := parseDay(in.AsOf)
	if err != nil {
		return nil, err
	}
	now := time.Date(2026, 11, 28, 0, 0, 0, 0, time.UTC)
	if in.Now != "" {
		if now, err = time.Parse(time.RFC3339, in.Now); err != nil {
			return nil, err
		}
	}
	ci := tariffwatch.CaseInput{Basis: in.Basis, BaseOrigin: in.BaseOrigin}
	if in.Plan != nil {
		ci.Plan = &tariffwatch.PlanInput{To: in.Plan.To, DutyEffectYear: in.Plan.DutyEffectPerYear}
	}
	res, err := ev.Watch(ci, asOf, now)
	if err != nil {
		return nil, fmt.Errorf("watch: %w", err)
	}
	a := watchActual{Affected: res.Affected, Changes: []tChange{}, Rates: []tRate{}, Notes: []string{}}
	for _, ch := range res.Changes {
		tc := tChange{Origin: ch.Origin, Kind: ch.Kind, Heading: ch.Heading, Moves: ch.MovesRate}
		if ch.OldRate != nil {
			tc.Old = ptr(rate1e4(*ch.OldRate))
		}
		if ch.NewRate != nil {
			tc.New = ptr(rate1e4(*ch.NewRate))
		}
		a.Changes = append(a.Changes, tc)
	}
	for _, r := range res.Rates {
		a.Rates = append(a.Rates, tRate{Origin: r.Origin, Delta: rate1e4(r.Delta), NewTotal: rate1e4(r.NewRate)})
	}
	for _, n := range res.Notes {
		switch {
		case strings.Contains(n, "entered by the user"):
			a.Notes = append(a.Notes, "user:"+strings.SplitN(n, ":", 2)[0])
		case strings.Contains(n, "no annual import value"):
			a.Notes = append(a.Notes, "no_annual")
		case strings.Contains(n, "not rechecked"):
			a.Notes = append(a.Notes, "plan_skipped")
		default:
			a.Notes = append(a.Notes, "other:"+n)
		}
	}
	if res.Annual != nil {
		a.Annual = &tMoney{Origin: res.Annual.Origin, Before: toCents(res.Annual.Before), After: toCents(res.Annual.After), Change: toCents(res.Annual.Change)}
	}
	if res.Plan != nil {
		a.Plan = &tMoney{From: res.Plan.From, To: res.Plan.To, Before: toCents(res.Plan.Before), After: toCents(res.Plan.After), Change: toCents(res.Plan.Change)}
	}
	return a, nil
}

func runRevise(in tariffInput, base *tariffwatch.Dataset, baseErr error) (any, error) {
	if baseErr != nil {
		return nil, fmt.Errorf("base data set: %w", baseErr)
	}
	out, rep, err := tariffwatch.Revise(base, in.OldText, in.NewText)
	if err != nil {
		return map[string]any{"error": true}, nil
	}
	a := reviseActual{Actions: []reviseAction{}, NotModelled: append([]string{}, rep.NotModelled...), ExEnds: []string{}, Warnings: len(rep.Warnings), Revision: out.Revision}
	for _, x := range out.Actions {
		subs := append([]string{}, x.Subheadings...)
		sort.Strings(subs)
		a.Actions = append(a.Actions, reviseAction{Heading: x.Heading, Rate: rate1e4(x.Rate), From: x.EffectiveFrom, Through: x.EffectiveThrough, Subheadings: subs})
	}
	sort.Slice(a.Actions, func(i, j int) bool { return a.Actions[i].Heading < a.Actions[j].Heading })
	sort.Strings(a.NotModelled)
	for _, e := range out.Additional.Exclusions {
		a.ExEnds = append(a.ExEnds, e.EndDate)
	}
	return a, nil
}

// compareTariff compares canonical JSON after ordering the lists whose order
// carries no meaning. Every number in both is an integer.
func compareTariff(exp, act []byte) (bool, string) {
	ne, err := normTariff(exp)
	if err != nil {
		return false, "unreadable expected label: " + err.Error()
	}
	na, err := normTariff(act)
	if err != nil {
		return false, "unreadable actual: " + err.Error()
	}
	if bytes.Equal(ne, na) {
		return true, ""
	}
	return false, "expected " + clip(string(ne)) + " got " + clip(string(na))
}

func clip(s string) string {
	if len(s) > 1200 {
		return s[:1200] + "..."
	}
	return s
}

func normTariff(raw []byte) ([]byte, error) {
	var v any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if m, ok := v.(map[string]any); ok {
		sortList(m, "changes")
		sortList(m, "rates")
		sortStrings(m, "notes")
		sortStrings(m, "not_modelled")
	}
	return marshal(v)
}

func sortList(m map[string]any, key string) {
	list, ok := m[key].([]any)
	if !ok {
		return
	}
	keys := make([]string, len(list))
	idx := make([]int, len(list))
	for i, x := range list {
		b, _ := marshal(x)
		keys[i], idx[i] = string(b), i
	}
	sort.Slice(idx, func(a, b int) bool { return keys[idx[a]] < keys[idx[b]] })
	out := make([]any, len(list))
	for i, j := range idx {
		out[i] = list[j]
	}
	m[key] = out
}

func sortStrings(m map[string]any, key string) {
	list, ok := m[key].([]any)
	if !ok {
		return
	}
	sort.Slice(list, func(i, j int) bool { return fmt.Sprint(list[i]) < fmt.Sprint(list[j]) })
}
