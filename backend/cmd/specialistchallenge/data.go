package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var idRE = regexp.MustCompile(`^c[0-9a-f]{12}$`)
var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// NormalizeFacts is the key for "same input": lower case, punctuation and
// spacing removed.
func NormalizeFacts(s string) string {
	return strings.TrimSpace(nonAlnum.ReplaceAllString(strings.ToLower(s), " "))
}

// RoutingFeature is the legal-routing study's feature id.
const RoutingFeature = "legal_routing"

func featureKnown(id string) bool {
	if id == RoutingFeature {
		return true
	}
	for _, f := range Features {
		if f.ID == id {
			return true
		}
	}
	return false
}

// IntegrityOptions tunes CheckIntegrity.
type IntegrityOptions struct{ MinTest int }

// CheckIntegrity returns every structural problem of the two splits: ids,
// provenance, labels, split isolation and the reserved minimum per feature.
func CheckIntegrity(dev, test []Case, o IntegrityOptions) []string {
	var p []string
	ids := map[string]string{}
	inputs := map[string]string{}
	perFeatureTest := map[string]int{}
	for _, part := range []struct {
		name  string
		cases []Case
	}{{"dev", dev}, {"test", test}} {
		for _, c := range part.cases {
			bad := func(f string, a ...any) {
				p = append(p, fmt.Sprintf("%s %s: %s", part.name, c.ID, fmt.Sprintf(f, a...)))
			}
			if !idRE.MatchString(c.ID) {
				bad("id is not an opaque c+12 hex id")
			}
			if prev, dup := ids[c.ID]; dup {
				bad("duplicate id (also in %s)", prev)
			}
			ids[c.ID] = part.name
			if !featureKnown(c.Feature) {
				bad("unknown feature %q", c.Feature)
			}
			if c.Split != part.name {
				bad("split %q in the %s file", c.Split, part.name)
			}
			if !c.Synthetic {
				bad("synthetic flag missing: every case here is synthetic")
			}
			if len(c.Sources) == 0 {
				bad("no primary-source reference")
			}
			for _, s := range c.Sources {
				if s.ID == "" || s.Locator == "" {
					bad("source without id or locator")
				}
			}
			if strings.TrimSpace(c.Rationale) == "" {
				bad("no rationale")
			}
			if c.SourceFamily == "" || c.Template == "" || c.ScenarioFamily == "" {
				bad("source family, scenario family or template missing")
			}
			if strings.TrimSpace(c.Input.Facts) == "" {
				bad("empty facts")
			}
			p = append(p, expectedProblems(part.name, c)...)
			n := NormalizeFacts(c.Input.Facts)
			if n != "" {
				if prev, dup := inputs[n]; dup {
					bad("normalised input already used by %s", prev)
				}
				inputs[n] = c.ID
			}
			if part.name == "test" {
				perFeatureTest[c.Feature]++
			}
		}
	}
	seen := map[string]bool{}
	for _, c := range append(append([]Case{}, dev...), test...) {
		seen[c.Feature] = true
	}
	for f := range seen {
		if perFeatureTest[f] < o.MinTest {
			p = append(p, fmt.Sprintf("%s has %d reserved cases, fewer than %d", f, perFeatureTest[f], o.MinTest))
		}
	}
	sort.Strings(p)
	return p
}

func expectedProblems(split string, c Case) []string {
	var p []string
	bad := func(f string, a ...any) { p = append(p, fmt.Sprintf("%s %s: %s", split, c.ID, fmt.Sprintf(f, a...))) }
	e := c.Expected
	switch e.Kind {
	case "decided":
		if c.Feature == "japan_engine" {
			if e.Outcome != "qualifies" && e.Outcome != "does_not_qualify" {
				bad("decided Japan case needs outcome qualifies or does_not_qualify, got %q", e.Outcome)
			}
		} else if strings.TrimSpace(e.Origin) == "" {
			bad("decided case has no expected origin")
		}
	case "uncertain":
		if e.Origin != "" {
			bad("uncertain case must not carry an origin")
		}
		if c.Feature == "japan_engine" && e.Outcome != "undetermined" {
			bad("uncertain Japan case needs outcome undetermined")
		}
	default:
		bad("unknown expected kind %q", e.Kind)
	}
	return p
}

var labelWords = regexp.MustCompile(`(?i)\b(expected|decided|uncertain|label|gold)\b`)

// Leaks lists what in the model-visible input (facts, date, Japan fields)
// gives away the answer or the case's own metadata.
func Leaks(c Case) []string {
	vis := c.Input.Facts + "\n" + c.Input.Date
	if c.Input.Japan != nil {
		vis += "\n" + c.Input.Japan.Agreement + " " + c.Input.Japan.HS
	}
	low := strings.ToLower(vis)
	var out []string
	has := func(what, s string) {
		if len(s) >= 4 && strings.Contains(low, strings.ToLower(s)) {
			out = append(out, what+" appears in the input")
		}
	}
	has("id", c.ID)
	has("template", c.Template)
	has("rationale", c.Rationale)
	has("source family", c.SourceFamily)
	if strings.Contains(c.ScenarioFamily, "-") { // single plain words such as "yarn" are ordinary facts text
		has("scenario family", c.ScenarioFamily)
	}
	for _, r := range c.ExcludeRulings {
		has("source ruling "+r, r)
	}
	if m := labelWords.FindString(vis); m != "" {
		out = append(out, fmt.Sprintf("label word %q appears in the input", m))
	}
	return out
}

func readCases(path string) ([]Case, []byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var out []Case
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		dec := json.NewDecoder(bytes.NewReader(sc.Bytes()))
		dec.DisallowUnknownFields()
		var c Case
		if err := dec.Decode(&c); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", path, err)
		}
		out = append(out, c)
	}
	return out, b, sc.Err()
}

// LoadAll reads every feature's dev and test file.
func LoadAll(dir string) (dev, test []Case, err error) {
	for _, f := range Features {
		d, _, err := readCases(filepath.Join(dir, f.File+"-dev.jsonl"))
		if err != nil {
			return nil, nil, err
		}
		t, _, err := readCases(filepath.Join(dir, f.File+"-test.jsonl"))
		if err != nil {
			return nil, nil, err
		}
		dev, test = append(dev, d...), append(test, t...)
	}
	return dev, test, nil
}

func fileSHA(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

func byFeature(cases []Case, id string) []Case {
	var out []Case
	for _, c := range cases {
		if c.Feature == id {
			out = append(out, c)
		}
	}
	return out
}

// LoadRouting reads the routing dev and test files.
func LoadRouting(dir string) (dev, test []Case, err error) {
	if dev, _, err = readCases(filepath.Join(dir, "routing-dev.jsonl")); err != nil {
		return nil, nil, err
	}
	if test, _, err = readCases(filepath.Join(dir, "routing-test.jsonl")); err != nil {
		return nil, nil, err
	}
	return dev, test, nil
}
