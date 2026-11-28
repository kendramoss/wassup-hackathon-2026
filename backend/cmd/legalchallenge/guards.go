package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/duty"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/usmca"
)

// IntegrityOptions tunes CheckIntegrity.
type IntegrityOptions struct {
	// MinTest is the least number of test cases each of Features must have.
	MinTest  int
	Features []string
}

// NormalizeKey is the identity of a case's engine input for duplicate
// detection: material ids and order, and the trailing zeros of amounts, carry
// no meaning, so they are removed before comparison.
func NormalizeKey(c Case) (string, error) {
	var v any
	dec := json.NewDecoder(bytes.NewReader(c.Input))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return "", err
	}
	obj, _ := v.(map[string]any)
	switch c.Feature {
	case "usmca_rules":
		if in, ok := obj["input"].(map[string]any); ok {
			for _, k := range []string{"transaction_value", "net_cost", "vnm", "total_cost"} {
				if s, ok := in[k].(string); ok {
					in[k] = cleanAmount(s)
				}
			}
			in["materials"] = normMaterials(in["materials"], true)
		}
	case "textile_rules":
		if f, ok := obj["facts"].(map[string]any); ok {
			f["materials"] = normMaterials(f["materials"], false)
		}
	}
	b, err := marshal(v)
	return string(b), err
}

func normMaterials(v any, usmcaShape bool) any {
	list, _ := v.([]any)
	if len(list) == 0 {
		return []any{}
	}
	type kv struct {
		key string
		v   any
	}
	var items []kv
	for _, m := range list {
		if mm, ok := m.(map[string]any); ok {
			delete(mm, "id")
			if usmcaShape {
				if s, ok := mm["value"].(string); ok {
					mm["value"] = cleanAmount(s)
				}
			}
		}
		b, _ := marshal(m)
		items = append(items, kv{string(b), m})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].key < items[j].key })
	out := make([]any, len(items))
	for i, it := range items {
		out[i] = it.v
	}
	return out
}

func expectedFlags(raw json.RawMessage) (invalid, refused bool, errorField string) {
	var e struct {
		Invalid        bool   `json:"invalid"`
		DatasetRefused bool   `json:"dataset_refused"`
		ErrorField     string `json:"error_field"`
	}
	_ = json.Unmarshal(raw, &e)
	return e.Invalid, e.DatasetRefused, e.ErrorField
}

// CheckIntegrity returns every structural problem in the two splits; an empty
// result means the datasets may be frozen.
func CheckIntegrity(dev, test []Case, srcs []SourceRec, opt IntegrityOptions) []string {
	var probs []string
	add := func(format string, a ...any) { probs = append(probs, fmt.Sprintf(format, a...)) }
	known := map[string]bool{}
	for _, s := range srcs {
		known[s.ID] = true
	}
	ids := map[string]string{}
	keys := map[string]map[string]string{"dev": {}, "test": {}}
	one := func(cs []Case, split string) {
		inSplit := map[string]bool{}
		for _, c := range cs {
			inSplit[c.ID] = true
		}
		for _, c := range cs {
			where := fmt.Sprintf("%s (%s)", c.ID, split)
			if c.ID == "" {
				add("%s: empty id", where)
			}
			if prev, ok := ids[c.ID]; ok {
				add("%s: id already used by a %s case", where, prev)
			}
			ids[c.ID] = split
			if _, ok := featureByID(c.Feature); !ok {
				add("%s: unknown feature %q", where, c.Feature)
			}
			if c.Split != split {
				add("%s: split %q does not match the file's split", where, c.Split)
			}
			for name, v := range map[string]string{"source_family": c.SourceFamily, "scenario_family": c.ScenarioFamily, "template": c.Template, "rationale": c.Rationale} {
				if strings.TrimSpace(v) == "" {
					add("%s: missing %s", where, name)
				}
			}
			if len(c.VariationTags) == 0 {
				add("%s: no variation tags", where)
			}
			if !c.Synthetic {
				add("%s: not marked synthetic (every case here is constructed)", where)
			}
			if len(c.Sources) == 0 {
				add("%s: no source references", where)
			}
			for _, s := range c.Sources {
				if !known[s.ID] {
					add("%s: source %q is not registered", where, s.ID)
				}
				if strings.TrimSpace(s.Locator) == "" {
					add("%s: source %q has no locator", where, s.ID)
				}
			}
			if len(bytes.TrimSpace(c.Input)) == 0 || !json.Valid(c.Input) {
				add("%s: input missing or not JSON", where)
			}
			if len(bytes.TrimSpace(c.Expected)) == 0 || !json.Valid(c.Expected) {
				add("%s: expected label missing or not JSON", where)
			}
			inv, refused, field := expectedFlags(c.Expected)
			switch {
			case (inv || refused) && !c.ExpectInvalid:
				add("%s: expected label is a refusal but expect_invalid is false", where)
			case c.ExpectInvalid && !(inv || refused):
				add("%s: expect_invalid is set but the expected label is not a refusal", where)
			case c.ExpectInvalid && inv && field == "":
				add("%s: a refused input must name the error_field", where)
			}
			if c.Parent != "" && !inSplit[c.Parent] {
				add("%s: parent %q is not a case of the same split", where, c.Parent)
			}
			if k, err := NormalizeKey(c); err == nil {
				id := c.Feature + "|" + k
				if prev, ok := keys[split][id]; ok {
					add("%s: duplicate normalized input of %s", where, prev)
				} else {
					keys[split][id] = c.ID
				}
			}
		}
	}
	one(dev, "dev")
	one(test, "test")
	for k, d := range keys["dev"] {
		if t, ok := keys["test"][k]; ok {
			add("cross-split duplicate normalized input: dev %s and test %s", d, t)
		}
	}
	for _, f := range opt.Features {
		if n := len(byFeature(test, f)); n < opt.MinTest {
			add("feature %s has %d test cases (need at least %d)", f, n, opt.MinTest)
		}
	}
	sort.Strings(probs)
	return probs
}

// CheckSources verifies every registered source file by hash.
func CheckSources(srcs []SourceRec, root string) []string {
	var probs []string
	for _, s := range srcs {
		if s.Kind == "" {
			probs = append(probs, fmt.Sprintf("source %s: no kind", s.ID))
		}
		b, err := os.ReadFile(filepath.Join(root, s.Path))
		if err != nil {
			probs = append(probs, fmt.Sprintf("source %s: %v", s.ID, err))
			continue
		}
		h := sha256.Sum256(b)
		if hex.EncodeToString(h[:]) != s.SHA256 {
			probs = append(probs, fmt.Sprintf("source %s: sha256 of %s differs from the registry", s.ID, s.Path))
		}
	}
	return probs
}

var textileProcesses = map[string]bool{"spinning": true, "extrusion": true, "fabric_making": true, "knit_to_shape": true, "assembly": true,
	"dyeing": true, "printing": true, "finishing": true, "cutting": true, "packing": true, "embroidery": true, "garnetting": true}

// CheckInputValidity checks, before any scored run, that every input is what
// its author meant: a case not declared invalid must be accepted by the
// candidate's input validation, and a case declared invalid must be refused.
// It reads only whether an input is accepted, never the outcome of a rule.
func CheckInputValidity(cases []Case) []string {
	var probs []string
	for _, c := range cases {
		refused, err := inputRefused(c)
		switch {
		case err != nil:
			probs = append(probs, fmt.Sprintf("%s: authoring error: %v", c.ID, err))
		case refused != c.ExpectInvalid && refused:
			probs = append(probs, fmt.Sprintf("%s: authoring error: input is refused but not declared invalid", c.ID))
		case refused != c.ExpectInvalid:
			probs = append(probs, fmt.Sprintf("%s: authoring error: declared invalid but the input is accepted", c.ID))
		}
	}
	return probs
}

func inputRefused(c Case) (bool, error) {
	switch c.Feature {
	case "usmca_rules":
		var body struct {
			Ruleset string          `json:"ruleset"`
			Input   json.RawMessage `json:"input"`
			Raw     *string         `json:"raw"`
		}
		if err := json.Unmarshal(c.Input, &body); err != nil {
			return false, err
		}
		raw := []byte(body.Input)
		if body.Raw != nil {
			raw = []byte(*body.Raw)
		}
		in, err := usmca.DecodeInput(raw)
		if err == nil {
			err = in.Validate()
		}
		if err == nil {
			return false, nil
		}
		if errors.Is(err, usmca.ErrInput) {
			return true, nil
		}
		return false, err
	case "textile_rules":
		f, err := decodeFacts(c.Input)
		if err != nil {
			return false, err
		}
		for _, s := range f.Stages {
			if !textileProcesses[s.Process] {
				return false, fmt.Errorf("unknown process %q", s.Process)
			}
		}
		return false, nil
	case "tariff_watch_rates":
		return tariffInputRefused(c)
	}
	return false, fmt.Errorf("unknown feature %q", c.Feature)
}

func tariffInputRefused(c Case) (bool, error) {
	var in tariffInput
	if err := json.Unmarshal(c.Input, &in); err != nil {
		return false, err
	}
	switch in.Kind {
	case "revise":
		if _, err := in.Dataset.decode(); err != nil {
			return true, nil
		}
		return false, nil
	case "watch":
		if _, err := in.Dataset.decode(); err != nil {
			return true, nil
		}
		if _, err := time.Parse(time.DateOnly, in.AsOf); err != nil {
			return false, fmt.Errorf("as_of: %w", err)
		}
		if _, err := duty.ParseHTS(in.Basis.HTS); err != nil {
			return false, err
		}
		return false, nil
	}
	return false, fmt.Errorf("unknown input kind %q", in.Kind)
}

// FamilyAccount is the visible accounting of families and shared templates.
type FamilyAccount struct {
	TestSourceFamilies          int `json:"test_source_families"`
	TestScenarioFamilies        int `json:"test_scenario_families"`
	TestTemplates               int `json:"test_templates"`
	DevSourceFamilies           int `json:"dev_source_families"`
	DevTemplates                int `json:"dev_templates"`
	SourceFamiliesSharedWithDev int `json:"test_source_families_also_in_dev"`
	TemplatesSharedWithDev      int `json:"test_templates_also_in_dev"`
	CasesWithParent             int `json:"test_cases_with_parent"`
	CasesInSharedTemplates      int `json:"test_cases_in_templates_shared_with_dev"`
}

// Families counts the source families and structural templates of one
// feature's splits and how many the test split shares with development.
func Families(dev, test []Case) FamilyAccount {
	set := func(cs []Case, f func(Case) string) map[string]bool {
		m := map[string]bool{}
		for _, c := range cs {
			m[f(c)] = true
		}
		return m
	}
	src := func(c Case) string { return c.SourceFamily }
	tpl := func(c Case) string { return c.Template }
	scn := func(c Case) string { return c.ScenarioFamily }
	ds, ts := set(dev, src), set(test, src)
	dt, tt := set(dev, tpl), set(test, tpl)
	fa := FamilyAccount{TestSourceFamilies: len(ts), TestScenarioFamilies: len(set(test, scn)), TestTemplates: len(tt), DevSourceFamilies: len(ds), DevTemplates: len(dt)}
	for k := range ts {
		if ds[k] {
			fa.SourceFamiliesSharedWithDev++
		}
	}
	for k := range tt {
		if dt[k] {
			fa.TemplatesSharedWithDev++
		}
	}
	for _, c := range test {
		if c.Parent != "" {
			fa.CasesWithParent++
		}
		if dt[c.Template] {
			fa.CasesInSharedTemplates++
		}
	}
	return fa
}

var shaRE = regexp.MustCompile(`^[0-9a-f]{40}$`)

// freezePaths are the paths whose committed state defines the candidate and the data.
var freezePaths = []string{"eval/challenges/legal-rules", "backend/internal/usmca", "backend/internal/textilerules", "backend/internal/tariffwatch",
	"backend/internal/duty", "backend/internal/domain", "backend/cmd/legalchallenge", "backend/go.mod", "backend/go.sum"}

// FrozenGuard refuses a reserved-test scoring run unless HEAD is the declared
// frozen commit, the candidate and data are committed unchanged, and no test
// result exists yet. git runs a git command in the repository root.
func FrozenGuard(git func(args ...string) (string, error), sha, resultsPath string) error {
	if !shaRE.MatchString(sha) {
		return fmt.Errorf("-freeze must be the full 40-character commit sha")
	}
	head, err := git("rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if strings.TrimSpace(head) != sha {
		return fmt.Errorf("HEAD is %s, not the declared frozen commit %s", strings.TrimSpace(head), sha)
	}
	st, err := git(append([]string{"status", "--porcelain", "--"}, freezePaths...)...)
	if err != nil {
		return err
	}
	if strings.TrimSpace(st) != "" {
		return fmt.Errorf("candidate or dataset files differ from the frozen commit:\n%s", st)
	}
	if _, err := os.Stat(resultsPath); err == nil {
		return fmt.Errorf("%s exists: the reserved test is scored once and never overwritten", filepath.Base(resultsPath))
	}
	return nil
}
