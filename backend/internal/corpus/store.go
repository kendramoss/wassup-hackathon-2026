package corpus

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// Precedent text limits from predict.py: a precedent's facts are kept to
// 3,000 characters in the pool, its holding to 600.
const (
	PoolFactsRunes   = 3000
	PoolHoldingRunes = 600
)

// Store is the in-memory ruling corpus. Rulings keep labels-file order.
type Store struct {
	rulings []domain.Ruling
	byID    map[string]int
	labels  map[string]Label
}

// Load builds a store from labels and a directory of facts files
// (<id>.txt, as written by SplitFacts plus a trailing newline). A ruling is
// included when its facts file exists and CBP held a single origin, the same
// rule the research harness used for its precedent pool.
func Load(labels []Label, factsDir string) (*Store, error) {
	s := &Store{byID: map[string]int{}, labels: map[string]Label{}}
	for _, l := range labels {
		s.labels[l.Ruling] = l
		if l.Origin() == "" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(factsDir, l.Ruling+".txt"))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("corpus: read facts %s: %w", l.Ruling, err)
		}
		date, _ := l.Time() // validated by LoadLabels
		s.byID[l.Ruling] = len(s.rulings)
		s.rulings = append(s.rulings, domain.Ruling{
			ID:      l.Ruling,
			Date:    date,
			Subject: l.Subject,
			Facts:   string(b),
			Holding: l.Holding(),
			Origin:  domain.NormalizeCountry(l.Origin()),
		})
	}
	return s, nil
}

// Rulings returns all rulings in corpus order. Callers must not modify it.
func (s *Store) Rulings() []domain.Ruling { return s.rulings }

// Get returns one ruling.
func (s *Store) Get(id string) (domain.Ruling, bool) {
	i, ok := s.byID[id]
	if !ok {
		return domain.Ruling{}, false
	}
	return s.rulings[i], true
}

// Label returns the gold label for a ruling, including rulings without a
// single origin that are not in the store.
func (s *Store) Label(id string) (Label, bool) {
	l, ok := s.labels[id]
	return l, ok
}

// Len is the number of rulings in the store.
func (s *Store) Len() int { return len(s.rulings) }
