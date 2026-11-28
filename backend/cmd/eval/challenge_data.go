package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

type moveChallengeExpected struct {
	OriginB       string `json:"origin_b"`
	ChangesOrigin *bool  `json:"changes_origin"`
	Supported     *bool  `json:"supported,omitempty"`
}
type moveChallengeCase struct {
	ID           string `json:"id"`
	Split        string `json:"split"`
	SourceFamily string `json:"source_family"`
	SourceParent string `json:"source_parent"`
	SourceRefs   []struct {
		Ruling string `json:"ruling"`
		URL    string `json:"url"`
	} `json:"source_refs"`
	VariationTags []string `json:"variation_tags"`
	Rationale     string   `json:"rationale"`
	Input         struct {
		FactsA       string `json:"facts_a"`
		FactsB       string `json:"facts_b"`
		KnownOriginA string `json:"known_origin_a"`
		Target       string `json:"target"`
		Before       string `json:"before"`
	} `json:"input"`
	Expected moveChallengeExpected `json:"expected"`
}
type moveChallengeDataset struct {
	SchemaVersion   int                 `json:"schema_version"`
	EvidenceKind    string              `json:"evidence_kind"`
	ExcludedRulings []string            `json:"excluded_rulings"`
	Limits          []string            `json:"limits"`
	Cases           []moveChallengeCase `json:"cases"`
}
type moveChallengeRow struct {
	ID            string  `json:"id"`
	Split         string  `json:"split"`
	DatasetSHA256 string  `json:"dataset_sha256"`
	Candidate     string  `json:"candidate_revision"`
	ModelID       string  `json:"model_id"`
	PromptVersion string  `json:"prompt_version"`
	Panel         bool    `json:"panel"`
	PlanBar       float64 `json:"plan_bar"`
	OriginA       string  `json:"origin_a"`
	OriginB       string  `json:"origin_b"`
	ChangesOrigin bool    `json:"changes_origin"`
	Supported     bool    `json:"shown"`
	Confidence    float64 `json:"confidence"`
	Supporting    string  `json:"supporting_ruling"`
	Counter       string  `json:"counter_ruling"`
	ModelCalls    int     `json:"model_calls"`
	CostUSD       float64 `json:"cost_usd"`
	LatencyMS     int64   `json:"latency_ms"`
	Error         string  `json:"error,omitempty"`
}
type moveChallengeStats struct {
	Cases          int `json:"cases"`
	Matched        int `json:"matched"`
	Mismatched     int `json:"mismatched"`
	Errors         int `json:"errors"`
	SourceFamilies int `json:"source_families"`
	PositiveCases  int `json:"positive_cases"`
	NegativeCases  int `json:"negative_cases"`
	UncertainCases int `json:"uncertain_cases"`
	TruePositive   int `json:"true_positive"`
	FalsePositive  int `json:"false_positive"`
	FalseNegative  int `json:"false_negative"`
	TrueNegative   int `json:"true_negative"`
	UncertainShown int `json:"uncertain_shown"`
}

func loadMoveChallenge(path string) (moveChallengeDataset, string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return moveChallengeDataset{}, "", err
	}
	var d moveChallengeDataset
	if err = json.Unmarshal(b, &d); err != nil {
		return d, "", err
	}
	if err = validateMoveChallenge(d); err != nil {
		return d, "", err
	}
	h := sha256.Sum256(b)
	return d, hex.EncodeToString(h[:]), nil
}

func validateMoveChallenge(d moveChallengeDataset) error {
	if d.SchemaVersion != 1 || d.EvidenceKind != "source_grounded_synthetic" || len(d.Cases) == 0 {
		return fmt.Errorf("invalid challenge schema or empty cases")
	}
	ids, inputs, families, rulings := map[string]bool{}, map[string]string{}, map[string]string{}, map[string]string{}
	excluded := map[string]bool{}
	for _, id := range d.ExcludedRulings {
		excluded[id] = true
	}
	for _, c := range d.Cases {
		if c.ID == "" || ids[c.ID] {
			return fmt.Errorf("duplicate or missing case id %q", c.ID)
		}
		ids[c.ID] = true
		if c.Split != "dev" && c.Split != "test" {
			return fmt.Errorf("%s: invalid split", c.ID)
		}
		if c.SourceFamily == "" || c.SourceParent == "" || c.Rationale == "" || len(c.SourceRefs) == 0 || len(c.VariationTags) == 0 {
			return fmt.Errorf("%s: missing provenance", c.ID)
		}
		if old := families[c.SourceFamily]; old != "" && old != c.Split {
			return fmt.Errorf("%s: source family crosses splits", c.ID)
		}
		families[c.SourceFamily] = c.Split
		for _, r := range c.SourceRefs {
			if !domain.ValidRulingID(r.Ruling) || r.URL != "https://rulings.cbp.gov/ruling/"+r.Ruling || !excluded[r.Ruling] {
				return fmt.Errorf("%s: invalid or unexcluded source", c.ID)
			}
			if old := rulings[r.Ruling]; old != "" && old != c.Split {
				return fmt.Errorf("%s: source ruling crosses splits", c.ID)
			}
			rulings[r.Ruling] = c.Split
		}
		if c.Input.FactsA == "" || c.Input.FactsB == "" || c.Input.KnownOriginA == "" || c.Input.Target == "" {
			return fmt.Errorf("%s: missing input facts", c.ID)
		}
		if _, err := time.Parse(time.DateOnly, c.Input.Before); err != nil {
			return fmt.Errorf("%s: invalid date", c.ID)
		}
		if c.Expected.ChangesOrigin == nil {
			if c.Expected.Supported == nil || *c.Expected.Supported {
				return fmt.Errorf("%s: uncertain case needs explicit withheld-support expectation", c.ID)
			}
		} else if c.Expected.OriginB == "" {
			return fmt.Errorf("%s: missing expected origin", c.ID)
		}
		normal := strings.ToLower(strings.Join(strings.Fields(c.Input.FactsA+"\x00"+c.Input.FactsB+"\x00"+c.Input.KnownOriginA+"\x00"+c.Input.Target+"\x00"+c.Input.Before), " "))
		if previous := inputs[normal]; previous != "" {
			return fmt.Errorf("duplicate engine inputs: %s and %s", previous, c.ID)
		}
		inputs[normal] = c.ID
	}
	return nil
}

func scoreMoveChallenge(cases []moveChallengeCase, rows []moveChallengeRow) (moveChallengeStats, error) {
	s := moveChallengeStats{Cases: len(cases)}
	byID := map[string]moveChallengeRow{}
	for _, r := range rows {
		if _, ok := byID[r.ID]; ok {
			return s, fmt.Errorf("duplicate result %s", r.ID)
		}
		byID[r.ID] = r
	}
	if len(byID) != len(cases) {
		return s, fmt.Errorf("incomplete results: %d for %d cases", len(byID), len(cases))
	}
	families := map[string]bool{}
	for _, c := range cases {
		r, ok := byID[c.ID]
		if !ok {
			return s, fmt.Errorf("missing result %s", c.ID)
		}
		families[c.SourceFamily] = true
		if c.Expected.ChangesOrigin == nil {
			s.UncertainCases++
			if r.Supported {
				s.UncertainShown++
			}
		} else if *c.Expected.ChangesOrigin {
			s.PositiveCases++
			if r.Supported && r.Error == "" && r.ChangesOrigin && domain.NormalizeCountry(r.OriginB) == domain.NormalizeCountry(c.Expected.OriginB) {
				s.TruePositive++
			} else {
				s.FalseNegative++
				if r.Supported && r.Error == "" {
					s.FalsePositive++
				}
			}
		} else {
			s.NegativeCases++
			if r.Supported && r.Error == "" {
				s.FalsePositive++
			} else if r.Error == "" {
				s.TrueNegative++
			}
		}
		if r.Error != "" {
			s.Errors++
			continue
		}
		matched := false
		if c.Expected.ChangesOrigin == nil {
			matched = c.Expected.Supported != nil && r.Supported == *c.Expected.Supported
		} else {
			matched = r.ChangesOrigin == *c.Expected.ChangesOrigin && domain.NormalizeCountry(r.OriginB) == domain.NormalizeCountry(c.Expected.OriginB)
		}
		if matched {
			s.Matched++
		} else {
			s.Mismatched++
		}
	}
	s.SourceFamilies = len(families)
	return s, nil
}
