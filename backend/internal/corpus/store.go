package corpus

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
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
	rev     string
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
	s.rev = RevisionOf(s.rulings)
	return s, nil
}

// Revision identifies the evidence the store can put in front of the engine
// and retriever. It is computed once by Load, so it is immutable and costs
// nothing per request. It changes when a ruling is added, removed or
// corrected, or when the corpus order changes (BM25 breaks score ties by
// corpus position), and not with paths, mtimes or unused labels. Reasoning
// attached after Load (WithReasoning) is not included; use RevisionOf on the
// final ruling slice for that.
func (s *Store) Revision() string { return s.rev }

// RevisionOf is the hex sha256 over rulings in order. Each ruling
// contributes every field the retriever indexes or the prompt renders (ID,
// date, origin, holding, subject, facts, reasoning), length-prefixed so field
// boundaries cannot collide.
func RevisionOf(rulings []domain.Ruling) string {
	h := sha256.New()
	put := func(v string) {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(v)))
		h.Write(n[:])
		h.Write([]byte(v))
	}
	put("corpus-revision-v1")
	for _, r := range rulings {
		put(r.ID)
		put(r.Date.Format("2006-01-02"))
		put(r.Origin)
		put(r.Holding)
		put(r.Subject)
		put(r.Facts)
		put(r.Reasoning)
	}
	return hex.EncodeToString(h.Sum(nil))
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
