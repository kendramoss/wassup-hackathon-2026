package corpus

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// LoadReasoning reads {ruling: reasoning} from a file written by
// "corpus reasoning" (data/textile_reasoning.json, D53).
func LoadReasoning(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("corpus: read %s: %w", path, err)
	}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("corpus: decode %s: %w", path, err)
	}
	return m, nil
}

// WithReasoning returns copies of rs with Reasoning filled from m.
func WithReasoning(rs []domain.Ruling, m map[string]string) []domain.Ruling {
	out := make([]domain.Ruling, len(rs))
	for i, r := range rs {
		if t, ok := m[r.ID]; ok {
			r.Reasoning = t
		}
		out[i] = r
	}
	return out
}
