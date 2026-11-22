package jpcorpus

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
)

// Input is one material as the ruling lists it, with the tariff chapters,
// headings and subheadings it cites and any country it names.
type Input struct {
	TextJA      string   `json:"text_ja"`
	NameJA      string   `json:"name_ja"`
	Chapters    []int    `json:"chapters"`
	Headings    []string `json:"headings"`
	Subheadings []string `json:"subheadings"`
	Countries   []string `json:"countries"`
}

// Ruling is one parsed advance ruling (eval/jp/parse.py). Japanese fields
// are as published.
type Ruling struct {
	ID          string  `json:"id"`
	URL         string  `json:"url"`
	OfficeJA    string  `json:"office_ja"`
	Date        string  `json:"date"`
	ProductJA   string  `json:"product_ja"`
	HS          string  `json:"hs"`
	HSChapter   int     `json:"hs_chapter"`
	HSHeading   string  `json:"hs_heading"`
	AgreementJA string  `json:"agreement_ja"`
	Agreement   string  `json:"agreement"`
	SummaryJA   string  `json:"summary_ja"`
	InputsJA    string  `json:"inputs_ja"`
	ProcessJA   string  `json:"process_ja"`
	Inputs      []Input `json:"inputs"`
	ReasoningJA string  `json:"reasoning_ja"`
	ArticlesJA  string  `json:"articles_ja"`
	OtherJA     string  `json:"other_ja"`
	AnswerJA    string  `json:"answer_ja"`
	RawSHA256   string  `json:"raw_sha256"`
}

// Time is the issue date.
func (r Ruling) Time() (time.Time, error) { return time.Parse(time.DateOnly, r.Date) }

// English is a ruling's machine translation (eval/jp/translate.py).
type English struct {
	Product   string `json:"product"`
	Inputs    string `json:"inputs"`
	Process   string `json:"process"`
	Reasoning string `json:"reasoning"`
	Articles  string `json:"articles"`
	Answer    string `json:"answer"`
	Other     string `json:"other"`
}

// Translation is one row of translations-en.jsonl.
type Translation struct {
	ID      string  `json:"id"`
	Model   string  `json:"model"`
	Version string  `json:"version"`
	EN      English `json:"en"`
}

// Label is one row of labels-*.jsonl (eval/jp/labels.py).
type Label struct {
	ID           string   `json:"id"`
	Agreement    string   `json:"agreement"`
	Outcome      string   `json:"outcome"`
	StatedOrigin string   `json:"stated_origin"`
	Criteria     []string `json:"criteria"`
	Criterion    string   `json:"criterion"`
}

// Splits is data/jp/splits.json (eval/jp/split.py, D710).
type Splits struct {
	Rule    string   `json:"rule"`
	Frozen  string   `json:"frozen"`
	Dev     []string `json:"dev"`
	DevEval []string `json:"dev_eval"`
	HeldOut []string `json:"held_out"`
}

// Criteria are the origin criteria a label or an answer may name.
var Criteria = []string{"WO", "PE", "CC", "CTH", "CTSH", "RVC", "SP"}

// AgreementNames gives the English name of each agreement code.
var AgreementNames = map[string]string{
	"JP-EU EPA":          "the Japan-EU Economic Partnership Agreement",
	"CPTPP":              "the CPTPP (TPP11)",
	"AJCEP":              "the ASEAN-Japan Comprehensive Economic Partnership (AJCEP)",
	"RCEP":               "the Regional Comprehensive Economic Partnership (RCEP)",
	"JP-US TA":           "the Japan-US Trade Agreement",
	"GSP":                "Japan's Generalised System of Preferences (GSP)",
	"JP-India CEPA":      "the Japan-India CEPA",
	"JP-Thailand EPA":    "the Japan-Thailand EPA",
	"JP-Indonesia EPA":   "the Japan-Indonesia EPA",
	"JP-Australia EPA":   "the Japan-Australia EPA",
	"JP-UK CEPA":         "the Japan-UK CEPA",
	"JP-Vietnam EPA":     "the Japan-Vietnam EPA",
	"JP-Malaysia EPA":    "the Japan-Malaysia EPA",
	"JP-Mexico EPA":      "the Japan-Mexico EPA",
	"JP-Philippines EPA": "the Japan-Philippines EPA",
	"JP-Switzerland EPA": "the Japan-Switzerland EPA",
	"JP-Chile EPA":       "the Japan-Chile EPA",
	"Non-preferential":   "Japan's non-preferential rules of origin",
}

// Corpus holds the rulings and their translations.
type Corpus struct {
	rulings []Ruling
	byID    map[string]int
	en      map[string]English
}

// Load reads dir/rulings.jsonl and, when present, dir/translations-en.jsonl.
func Load(dir string) (*Corpus, error) {
	c := &Corpus{byID: map[string]int{}, en: map[string]English{}}
	err := readJSONL(filepath.Join(dir, "rulings.jsonl"), func(b []byte) error {
		var r Ruling
		if err := json.Unmarshal(b, &r); err != nil {
			return err
		}
		if _, err := r.Time(); err != nil {
			return fmt.Errorf("ruling %s: %w", r.ID, err)
		}
		if _, dup := c.byID[r.ID]; dup {
			return fmt.Errorf("duplicate ruling %s", r.ID)
		}
		c.byID[r.ID] = len(c.rulings)
		c.rulings = append(c.rulings, r)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("jpcorpus: %w", err)
	}
	err = readJSONL(filepath.Join(dir, "translations-en.jsonl"), func(b []byte) error {
		var t Translation
		if err := json.Unmarshal(b, &t); err != nil {
			return err
		}
		if _, ok := c.byID[t.ID]; !ok {
			return fmt.Errorf("translation for unknown ruling %s", t.ID)
		}
		c.en[t.ID] = t.EN
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("jpcorpus: %w", err)
	}
	return c, nil
}

// Len is the number of rulings.
func (c *Corpus) Len() int { return len(c.rulings) }

// Rulings returns every ruling in file order.
func (c *Corpus) Rulings() []Ruling { return append([]Ruling(nil), c.rulings...) }

// Ruling returns a ruling by id.
func (c *Corpus) Ruling(id string) (Ruling, bool) {
	i, ok := c.byID[id]
	if !ok {
		return Ruling{}, false
	}
	return c.rulings[i], true
}

// English returns a ruling's translation.
func (c *Corpus) English(id string) (English, bool) {
	e, ok := c.en[id]
	return e, ok
}

// CaseFacts is what an engine may see of a ruling as a new case: product,
// tariff number, agreement, materials and process, in English. It never
// includes the reasoning, legal basis or answer.
func (c *Corpus) CaseFacts(id string) (string, error) {
	r, ok := c.Ruling(id)
	if !ok {
		return "", fmt.Errorf("jpcorpus: unknown ruling %s", id)
	}
	e, ok := c.en[id]
	if !ok {
		return "", fmt.Errorf("jpcorpus: ruling %s has no translation", id)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Product: %s\nTariff number of the good (Japan): %s\nPreference claimed: %s\n", e.Product, r.HS, AgreementNames[r.Agreement])
	if e.Inputs != "" {
		fmt.Fprintf(&b, "Materials: %s\n", e.Inputs)
	}
	if e.Process != "" {
		fmt.Fprintf(&b, "Production process: %s\n", e.Process)
	}
	return strings.TrimSpace(b.String()), nil
}

// PrecedentID is the id a Japan ruling carries as a domain.Ruling, kept
// apart from CBP ruling numbers.
func PrecedentID(id string) string { return "JP" + id }

// HoldingRunes caps the reasoning in a precedent. Japan Customs' reasoning
// names the rule met near its end, so the end is kept, not the start.
const HoldingRunes = 900

// holdingReasoning drops the closing "However, ... conditional on ..."
// boilerplate every ruling ends with and keeps the last HoldingRunes runes.
func holdingReasoning(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "However,"); i > len(s)/2 {
		s = strings.TrimSpace(s[:i])
	}
	r := []rune(s)
	if len(r) > HoldingRunes {
		return "..." + string(r[len(r)-HoldingRunes:])
	}
	return s
}

// Precedent is a ruling as a domain.Ruling for retrieval and precedent
// blocks: Facts is CaseFacts, Holding is the answer and the end of the
// reasoning in English.
func (c *Corpus) Precedent(id string) (domain.Ruling, error) {
	r, _ := c.Ruling(id)
	facts, err := c.CaseFacts(id)
	if err != nil {
		return domain.Ruling{}, err
	}
	t, _ := r.Time()
	e := c.en[id]
	return domain.Ruling{
		ID:      PrecedentID(id),
		Date:    t,
		Subject: e.Product,
		Facts:   facts,
		Holding: strings.TrimSpace(e.Answer + " Reasoning: " + holdingReasoning(e.Reasoning)),
	}, nil
}

// Index is a BM25 index over the English facts of the given rulings
// (retrieval.NewBM25, the U.S. index's settings). Search it with
// retrieval.PrecedentFilter so only strictly earlier rulings come back.
func (c *Corpus) Index(ids []string) (*retrieval.BM25, error) {
	docs := make([]domain.Ruling, 0, len(ids))
	for _, id := range ids {
		p, err := c.Precedent(id)
		if err != nil {
			return nil, err
		}
		docs = append(docs, p)
	}
	return retrieval.NewBM25(docs, func(r domain.Ruling) string { return r.Facts }), nil
}

// LoadSplits reads data/jp/splits.json.
func LoadSplits(path string) (Splits, error) {
	var s Splits
	b, err := os.ReadFile(path)
	if err != nil {
		return s, fmt.Errorf("jpcorpus: %w", err)
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return s, fmt.Errorf("jpcorpus: splits: %w", err)
	}
	return s, nil
}

// LoadLabels reads a labels-*.jsonl file.
func LoadLabels(path string) (map[string]Label, error) {
	out := map[string]Label{}
	err := readJSONL(path, func(b []byte) error {
		var l Label
		if err := json.Unmarshal(b, &l); err != nil {
			return err
		}
		out[l.ID] = l
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("jpcorpus: %w", err)
	}
	return out, nil
}

func readJSONL(path string, each func([]byte) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	n := 0
	for sc.Scan() {
		n++
		if len(strings.TrimSpace(sc.Text())) == 0 {
			continue
		}
		if err := each(sc.Bytes()); err != nil {
			return fmt.Errorf("%s line %d: %w", filepath.Base(path), n, err)
		}
	}
	return sc.Err()
}
