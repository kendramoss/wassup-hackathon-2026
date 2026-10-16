package evalkit

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// Adjustments are scoring corrections applied identically to every run
// (eval/data/scoring_adjustments.json): rulings with no single held origin
// are excluded; rulings where CBP held either of several countries accept
// any of them.
type Adjustments struct {
	Exclude      map[string]string   `json:"exclude"`
	Alternatives map[string][]string `json:"alternatives"`
}

// LoadAdjustments reads an adjustments file.
func LoadAdjustments(path string) (Adjustments, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Adjustments{}, fmt.Errorf("evalkit: %w", err)
	}
	var a Adjustments
	if err := json.Unmarshal(b, &a); err != nil {
		return Adjustments{}, fmt.Errorf("evalkit: %s: %w", path, err)
	}
	return a, nil
}

// Apply drops excluded rulings and re-scores rulings with alternative golds:
// correct when every country in the prediction is one CBP allowed.
func (a Adjustments) Apply(rows []Row) []Row {
	out := make([]Row, 0, len(rows))
	for _, r := range rows {
		if _, skip := a.Exclude[r.Ruling]; skip {
			continue
		}
		if alts, ok := a.Alternatives[r.Ruling]; ok && r.Error == "" {
			parts := domain.CountryParts(domain.NormalizeCountry(r.Pred))
			r.Correct = len(parts) > 0
			for _, p := range parts {
				if !slices.Contains(alts, p) {
					r.Correct = false
				}
			}
		}
		out = append(out, r)
	}
	return out
}
