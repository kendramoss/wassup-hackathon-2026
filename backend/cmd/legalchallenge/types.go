package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// Row statuses. Every case produces exactly one row in each scored run.
const (
	StatusMatch    = "match"
	StatusMismatch = "mismatch"
	StatusError    = "error"
)

// Feature is one scored capability and its dataset file prefix.
type Feature struct {
	ID     string
	Prefix string
	Label  string
}

// Features are the three capabilities of this study.
var Features = []Feature{
	{"usmca_rules", "usmca", "USMCA preferential rules"},
	{"textile_rules", "textile", "Textile origin (19 CFR 102.21)"},
	{"tariff_watch_rates", "tariffwatch", "Tariff-watch rate changes"},
}

func featureIDs() []string {
	var out []string
	for _, f := range Features {
		out = append(out, f.ID)
	}
	return out
}

func featureByID(id string) (Feature, bool) {
	for _, f := range Features {
		if f.ID == id {
			return f, true
		}
	}
	return Feature{}, false
}

// SourceRef points a case at a registered source and the clause in it.
type SourceRef struct {
	ID      string `json:"id"`
	Locator string `json:"locator"`
}

// SourceRec is one registered source file (sources.json).
type SourceRec struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Kind   string `json:"kind"`
}

// Case is one frozen challenge case. Expected holds the label written by the
// reference oracle before any candidate is run.
type Case struct {
	ID             string          `json:"id"`
	Feature        string          `json:"feature"`
	Split          string          `json:"split"`
	SourceFamily   string          `json:"source_family"`
	ScenarioFamily string          `json:"scenario_family"`
	Template       string          `json:"template"`
	Parent         string          `json:"parent,omitempty"`
	VariationTags  []string        `json:"variation_tags"`
	Sources        []SourceRef     `json:"sources"`
	Rationale      string          `json:"rationale"`
	Synthetic      bool            `json:"synthetic"`
	ExpectInvalid  bool            `json:"expect_invalid"`
	Input          json.RawMessage `json:"input"`
	Expected       json.RawMessage `json:"expected"`
}

// Row is the retained result of one case.
type Row struct {
	ID       string          `json:"id"`
	Feature  string          `json:"feature"`
	Split    string          `json:"split"`
	Status   string          `json:"status"`
	Expected json.RawMessage `json:"expected"`
	Actual   json.RawMessage `json:"actual,omitempty"`
	Why      string          `json:"why,omitempty"`
}

// LoadCases reads JSON lines strictly: unknown fields and repeated ids are refused.
func LoadCases(r io.Reader) ([]Case, error) {
	var out []Case
	seen := map[string]bool{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	line := 0
	for sc.Scan() {
		line++
		b := bytes.TrimSpace(sc.Bytes())
		if len(b) == 0 {
			continue
		}
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.DisallowUnknownFields()
		var c Case
		if err := dec.Decode(&c); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if seen[c.ID] {
			return nil, fmt.Errorf("line %d: repeated id %q", line, c.ID)
		}
		seen[c.ID] = true
		out = append(out, c)
	}
	return out, sc.Err()
}

// WriteRows writes one JSON line per row, in the order given.
func WriteRows(w io.Writer, rows []Row) error {
	for _, r := range rows {
		b, err := marshal(r)
		if err != nil {
			return err
		}
		if _, err := w.Write(append(b, '\n')); err != nil {
			return err
		}
	}
	return nil
}

// ReadRows reads rows written by WriteRows.
func ReadRows(r io.Reader) ([]Row, error) {
	var out []Row
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		b := bytes.TrimSpace(sc.Bytes())
		if len(b) == 0 {
			continue
		}
		var row Row
		if err := json.Unmarshal(b, &row); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, sc.Err()
}

// marshal is json.Marshal without HTML escaping and without a trailing newline.
func marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// canon re-encodes JSON with sorted keys and no insignificant whitespace.
func canon(raw []byte) ([]byte, error) {
	var v any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return marshal(v)
}

func dataRoot() string {
	if d := os.Getenv("LEGAL_CHALLENGE_DIR"); d != "" {
		return d
	}
	return filepath.Join(repoRoot(), "eval", "challenges", "legal-rules")
}

func repoRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	// backend/cmd/legalchallenge/types.go -> repository root
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func loadFile(path string) ([]Case, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cs, err := LoadCases(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return cs, nil
}

// LoadAll reads every dataset split and the source registry under root.
func LoadAll(root string) (dev, test []Case, srcs []SourceRec, err error) {
	for _, f := range Features {
		d, e := loadFile(filepath.Join(root, f.Prefix+"-dev.jsonl"))
		if e != nil {
			return nil, nil, nil, e
		}
		t, e := loadFile(filepath.Join(root, f.Prefix+"-test.jsonl"))
		if e != nil {
			return nil, nil, nil, e
		}
		dev, test = append(dev, d...), append(test, t...)
	}
	b, e := os.ReadFile(filepath.Join(root, "sources.json"))
	if e != nil {
		return nil, nil, nil, e
	}
	var reg struct {
		Sources []SourceRec `json:"sources"`
	}
	if e := json.Unmarshal(b, &reg); e != nil {
		return nil, nil, nil, e
	}
	return dev, test, reg.Sources, nil
}

func byFeature(cs []Case, id string) []Case {
	var out []Case
	for _, c := range cs {
		if c.Feature == id {
			out = append(out, c)
		}
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func cleanAmount(s string) string {
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimSuffix(s, ".")
	}
	return s
}
