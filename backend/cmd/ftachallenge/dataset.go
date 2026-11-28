package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"slices"
	"sort"
	"strings"
)

const (
	schemaVersion = 1
	evidenceKind  = "source_grounded_synthetic"
	// MinTestCases is the smallest reserved test set a feature may report.
	MinTestCases = 100
)

var features = []string{"atiga", "akfta", "acfta", "rcep"}

var labels = map[string]string{
	"atiga": "ATIGA rules",
	"akfta": "AKFTA rules",
	"acfta": "ACFTA rules",
	"rcep":  "RCEP rules",
}

// parties lists, per agreement, the countries a case may treat as members.
// They come from the agreements, not from the engines under test.
var parties = map[string][]string{
	"atiga": {"brunei", "cambodia", "indonesia", "laos", "malaysia", "myanmar", "philippines", "singapore", "thailand", "vietnam"},
	"akfta": {"brunei", "cambodia", "indonesia", "laos", "malaysia", "myanmar", "philippines", "singapore", "thailand", "vietnam", "south korea"},
	"acfta": {"brunei", "cambodia", "indonesia", "laos", "malaysia", "myanmar", "philippines", "singapore", "thailand", "vietnam", "china"},
	"rcep": {"brunei", "cambodia", "indonesia", "laos", "malaysia", "myanmar", "philippines", "singapore", "thailand", "vietnam",
		"australia", "china", "japan", "new zealand", "south korea"},
}

var partyAliases = map[string]string{
	"viet nam": "vietnam", "lao pdr": "laos", "brunei darussalam": "brunei",
	"republic of korea": "south korea", "korea": "south korea", "prc": "china",
}

func memberOf(feature, country string) string {
	c := strings.ToLower(strings.TrimSpace(country))
	if a, ok := partyAliases[c]; ok {
		c = a
	}
	if slices.Contains(parties[feature], c) {
		return c
	}
	return ""
}

type SourceRef struct {
	Source  string `json:"source"`
	Locator string `json:"locator"`
}

type RuleRef struct {
	Row    string `json:"row"`
	Text   string `json:"text"`
	Listed bool   `json:"listed"`
}

type Expect struct {
	Outcome       string            `json:"outcome"`
	Applied       string            `json:"applied,omitempty"`
	Alternative   map[string]string `json:"alternative,omitempty"`
	Accumulation  *bool             `json:"accumulation,omitempty"`
	DeMinimisUsed *bool             `json:"de_minimis_used,omitempty"`
	RVCDownMin    *float64          `json:"rvc_down_min,omitempty"`
	RVCDownMax    *float64          `json:"rvc_down_max,omitempty"`
	RVCUpMin      *float64          `json:"rvc_up_min,omitempty"`
	RVCUpMax      *float64          `json:"rvc_up_max,omitempty"`
}

type Case struct {
	ID                 string          `json:"id"`
	Split              string          `json:"split"`
	Feature            string          `json:"feature"`
	SourceFamilyID     string          `json:"source_family_id"`
	ScenarioFamily     string          `json:"scenario_family"`
	ParentID           string          `json:"parent_id,omitempty"`
	VariationTags      []string        `json:"variation_tags"`
	Sources            []SourceRef     `json:"sources"`
	Rule               *RuleRef        `json:"rule,omitempty"`
	Rationale          string          `json:"rationale"`
	Input              json.RawMessage `json:"input"`
	Expect             Expect          `json:"expect"`
	ExpectedAbstention bool            `json:"expected_abstention"`
	FormattingOnly     bool            `json:"formatting_only,omitempty"`
}

type Dataset struct {
	SchemaVersion int                        `json:"schema_version"`
	DatasetVer    string                     `json:"dataset_version"`
	EvidenceKind  string                     `json:"evidence_kind"`
	Feature       string                     `json:"feature"`
	Split         string                     `json:"split"`
	About         string                     `json:"about"`
	LabelMethod   string                     `json:"label_method"`
	CaseCount     int                        `json:"case_count"`
	Sources       map[string]json.RawMessage `json:"sources"`
	Cases         []Case                     `json:"cases"`

	SHA256 string `json:"-"`
}

func parseDataset(data []byte) (*Dataset, error) {
	var ds Dataset
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&ds); err != nil {
		return nil, err
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("expected one JSON document")
	}
	ds.SHA256 = fmt.Sprintf("%x", sha256.Sum256(data))
	return &ds, nil
}

var validOutcomes = map[string]bool{"yes": true, "no": true, "undetermined": true, "refused": true}

var sha40 = regexp.MustCompile(`^[0-9a-f]{40}$`)

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

type material struct {
	HS      string
	Country string
	Status  string
}

func inputView(c Case) (producedIn string, mats []material, err error) {
	var in struct {
		ProducedIn string `json:"produced_in"`
		Materials  []struct {
			HS      string `json:"hs"`
			Country string `json:"country"`
			Status  string `json:"status"`
		} `json:"materials"`
	}
	if err := json.Unmarshal(c.Input, &in); err != nil {
		return "", nil, err
	}
	for _, m := range in.Materials {
		mats = append(mats, material{m.HS, m.Country, m.Status})
	}
	return in.ProducedIn, mats, nil
}

// normalizedInput is the key two inputs share when only wording differs:
// names are dropped, HS codes keep their digits and text is lower-cased.
func normalizedInput(c Case) (string, error) {
	var v map[string]any
	if err := json.Unmarshal(c.Input, &v); err != nil {
		return "", err
	}
	if s, ok := v["hs"].(string); ok {
		v["hs"] = digitsOnly(s)
	}
	if s, ok := v["produced_in"].(string); ok {
		v["produced_in"] = strings.ToLower(strings.TrimSpace(s))
	}
	if ms, ok := v["materials"].([]any); ok {
		keys := make([]string, 0, len(ms))
		for _, m := range ms {
			mm, ok := m.(map[string]any)
			if !ok {
				continue
			}
			delete(mm, "name")
			if s, ok := mm["hs"].(string); ok {
				mm["hs"] = digitsOnly(s)
			}
			for _, k := range []string{"country", "status"} {
				if s, ok := mm[k].(string); ok {
					mm[k] = strings.ToLower(strings.TrimSpace(s))
				}
			}
			b, _ := json.Marshal(mm)
			keys = append(keys, string(b))
		}
		sort.Strings(keys)
		v["materials"] = keys
	}
	b, err := json.Marshal(v)
	return c.Feature + "|" + string(b), err
}

func hasTag(c Case, tag string) bool { return slices.Contains(c.VariationTags, tag) }

// validateMembership rejects cases that claim agreement membership the
// agreement does not give.
func validateMembership(c Case) error {
	producer, mats, err := inputView(c)
	if err != nil {
		return fmt.Errorf("%s: input: %w", c.ID, err)
	}
	prod := memberOf(c.Feature, producer)
	memberOrigin, outsiderMaterial := false, false
	for _, m := range mats {
		mem := memberOf(c.Feature, m.Country)
		if m.Status == "originating" && mem != "" && mem != prod {
			memberOrigin = true
		}
		if strings.TrimSpace(m.Country) != "" && mem == "" {
			outsiderMaterial = true
		}
	}
	if c.Expect.Accumulation != nil && *c.Expect.Accumulation {
		if prod == "" || !memberOrigin {
			return fmt.Errorf("%s: expects accumulation but no originating material comes from a different %s Party", c.ID, strings.ToUpper(c.Feature))
		}
	}
	if hasTag(c, "member-origin") && !materialFromOtherMember(c.Feature, prod, mats) {
		return fmt.Errorf("%s: tagged member-origin but no material comes from another %s Party", c.ID, strings.ToUpper(c.Feature))
	}
	if hasTag(c, "non-member-origin") && !outsiderMaterial {
		return fmt.Errorf("%s: tagged non-member-origin but every material country is a %s Party", c.ID, strings.ToUpper(c.Feature))
	}
	if hasTag(c, "non-member-producer") && (strings.TrimSpace(producer) == "" || prod != "") {
		return fmt.Errorf("%s: tagged non-member-producer but %q is a %s Party", c.ID, producer, strings.ToUpper(c.Feature))
	}
	return nil
}

func materialFromOtherMember(feature, prod string, mats []material) bool {
	for _, m := range mats {
		if mem := memberOf(feature, m.Country); mem != "" && mem != prod {
			return true
		}
	}
	return false
}

// Validate checks one dataset on its own.
func (ds *Dataset) Validate() error {
	if ds.SchemaVersion != schemaVersion {
		return fmt.Errorf("schema_version %d, want %d", ds.SchemaVersion, schemaVersion)
	}
	if ds.EvidenceKind != evidenceKind {
		return fmt.Errorf("evidence_kind %q, want %q", ds.EvidenceKind, evidenceKind)
	}
	if !slices.Contains(features, ds.Feature) {
		return fmt.Errorf("unknown feature %q", ds.Feature)
	}
	if ds.Split != "dev" && ds.Split != "test" {
		return fmt.Errorf("split %q must be dev or test", ds.Split)
	}
	if ds.CaseCount != len(ds.Cases) {
		return fmt.Errorf("case_count %d but %d cases present (missing or extra cases)", ds.CaseCount, len(ds.Cases))
	}
	if len(ds.Cases) == 0 {
		return fmt.Errorf("no cases")
	}
	if ds.Split == "test" && len(ds.Cases) < MinTestCases {
		return fmt.Errorf("%s test set has %d cases, need at least %d", ds.Feature, len(ds.Cases), MinTestCases)
	}
	ids := map[string]bool{}
	inputs := map[string]string{}
	for _, c := range ds.Cases {
		if c.ID == "" || ids[c.ID] {
			return fmt.Errorf("missing or duplicate case id %q", c.ID)
		}
		ids[c.ID] = true
		if c.Feature != ds.Feature || c.Split != ds.Split {
			return fmt.Errorf("%s: case says %s/%s inside a %s/%s dataset", c.ID, c.Feature, c.Split, ds.Feature, ds.Split)
		}
		if c.SourceFamilyID == "" || c.ScenarioFamily == "" {
			return fmt.Errorf("%s: missing source_family_id or scenario_family", c.ID)
		}
		if len(c.VariationTags) == 0 {
			return fmt.Errorf("%s: no variation tags", c.ID)
		}
		if strings.TrimSpace(c.Rationale) == "" {
			return fmt.Errorf("%s: no rationale", c.ID)
		}
		if len(c.Sources) == 0 {
			return fmt.Errorf("%s: no primary-source reference", c.ID)
		}
		for _, s := range c.Sources {
			if _, ok := ds.Sources[s.Source]; !ok || s.Locator == "" {
				return fmt.Errorf("%s: source reference %q is not described in the dataset or has no locator", c.ID, s.Source)
			}
		}
		if !validOutcomes[c.Expect.Outcome] {
			return fmt.Errorf("%s: invalid expected outcome %q", c.ID, c.Expect.Outcome)
		}
		if c.ExpectedAbstention != (c.Expect.Outcome == "undetermined") {
			return fmt.Errorf("%s: expected_abstention disagrees with expected outcome %q", c.ID, c.Expect.Outcome)
		}
		for rule, want := range c.Expect.Alternative {
			if rule == "" || !validOutcomes[want] || want == "refused" {
				return fmt.Errorf("%s: invalid alternative expectation", c.ID)
			}
		}
		if err := validateMembership(c); err != nil {
			return err
		}
		key, err := normalizedInput(c)
		if err != nil {
			return fmt.Errorf("%s: %w", c.ID, err)
		}
		if other, dup := inputs[key]; dup {
			return fmt.Errorf("%s repeats the normalized input of %s", c.ID, other)
		}
		inputs[key] = c.ID
	}
	return nil
}

// ValidateSplits checks that development and test data share no normalized
// input or id.
func ValidateSplits(dev, test *Dataset) error {
	if dev.Feature != test.Feature {
		return fmt.Errorf("splits are for different features: %s and %s", dev.Feature, test.Feature)
	}
	if dev.Split != "dev" || test.Split != "test" {
		return fmt.Errorf("expected a dev and a test dataset, got %s and %s", dev.Split, test.Split)
	}
	seen := map[string]string{}
	ids := map[string]bool{}
	for _, c := range dev.Cases {
		k, err := normalizedInput(c)
		if err != nil {
			return err
		}
		seen[k] = c.ID
		ids[c.ID] = true
	}
	for _, c := range test.Cases {
		if ids[c.ID] {
			return fmt.Errorf("case id %s appears in both splits", c.ID)
		}
		k, err := normalizedInput(c)
		if err != nil {
			return err
		}
		if d, dup := seen[k]; dup {
			return fmt.Errorf("test case %s has the same normalized input as development case %s", c.ID, d)
		}
	}
	return nil
}

// FamilyOverlap reports the scenario families both splits use, and the ones
// only the test set uses.
func FamilyOverlap(dev, test *Dataset) (shared, testOnly []string) {
	d := map[string]bool{}
	for _, c := range dev.Cases {
		d[c.ScenarioFamily] = true
	}
	t := map[string]bool{}
	for _, c := range test.Cases {
		t[c.ScenarioFamily] = true
	}
	for f := range t {
		if d[f] {
			shared = append(shared, f)
		} else {
			testOnly = append(testOnly, f)
		}
	}
	sort.Strings(shared)
	sort.Strings(testOnly)
	return
}

func distinct(cases []Case, f func(Case) string) int {
	m := map[string]bool{}
	for _, c := range cases {
		if v := f(c); v != "" {
			m[v] = true
		}
	}
	return len(m)
}
