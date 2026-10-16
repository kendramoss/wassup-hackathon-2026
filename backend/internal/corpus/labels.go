package corpus

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// Label is CBP's holding for one ruling, from eval/data/labels.jsonl.
type Label struct {
	Ruling        string   `json:"ruling"`
	Date          string   `json:"date"` // YYYY-MM-DD
	Collection    string   `json:"collection"`
	Subject       string   `json:"subject"`
	Regime        string   `json:"regime"`
	MultiCountry  bool     `json:"multi_country"`
	Method        string   `json:"method"`         // "regex" or "manual"
	PrimaryOrigin *string  `json:"primary_origin"` // nil when no single origin
	Evidence      []string `json:"evidence"`       // holding sentence(s)
}

// Origin is the held origin, or "" when there is none.
func (l Label) Origin() string {
	if l.PrimaryOrigin == nil {
		return ""
	}
	return *l.PrimaryOrigin
}

// Holding joins the holding sentences, as shown to the engine for a precedent.
func (l Label) Holding() string { return strings.Join(l.Evidence, " ") }

// Time parses Date.
func (l Label) Time() (time.Time, error) { return time.Parse(time.DateOnly, l.Date) }

// LoadLabels reads a labels JSONL file, keeping file order (precedent ties are
// broken by it, as in the research harness).
func LoadLabels(path string) ([]Label, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("corpus: open labels: %w", err)
	}
	defer f.Close()
	var out []Label
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for line := 1; sc.Scan(); line++ {
		var l Label
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			return nil, fmt.Errorf("corpus: labels line %d: %w", line, err)
		}
		if _, err := l.Time(); err != nil {
			return nil, fmt.Errorf("corpus: labels line %d (%s): bad date %q", line, l.Ruling, l.Date)
		}
		out = append(out, l)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("corpus: read labels: %w", err)
	}
	return out, nil
}

// Splits are the frozen eval splits from eval/splits/splits_final.json, or
// the tuning splits from eval/splits/splits_tune.json (train, tune_check).
type Splits struct {
	FrozenAt  string   `json:"frozen_at"`
	Final     []string `json:"final,omitempty"`
	Dev       []string `json:"dev,omitempty"`
	Pilot     []string `json:"pilot,omitempty"`
	Train     []string `json:"train,omitempty"`
	TuneCheck []string `json:"tune_check,omitempty"`
}

// LoadSplits reads the splits file.
func LoadSplits(path string) (Splits, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Splits{}, fmt.Errorf("corpus: read splits: %w", err)
	}
	var s Splits
	if err := json.Unmarshal(b, &s); err != nil {
		return Splits{}, fmt.Errorf("corpus: decode splits: %w", err)
	}
	return s, nil
}

// IDs returns the ruling IDs of a split by name.
func (s Splits) IDs(name string) ([]string, error) {
	switch name {
	case "final":
		return s.Final, nil
	case "dev":
		return s.Dev, nil
	case "pilot":
		return s.Pilot, nil
	case "train":
		return s.Train, nil
	case "tune_check":
		return s.TuneCheck, nil
	}
	return nil, fmt.Errorf("corpus: unknown split %q (want final, dev, pilot, train or tune_check)", name)
}
