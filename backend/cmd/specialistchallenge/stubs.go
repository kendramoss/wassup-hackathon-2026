package main

import (
	"context"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
)

// IntegrityOptions tunes CheckIntegrity.
type IntegrityOptions struct{ MinTest int }

func NormalizeFacts(s string) string                                                      { return "" }
func CheckIntegrity(dev, test []Case, o IntegrityOptions) []string                        { return nil }
func Leaks(c Case) []string                                                               { return nil }
func Grade(c Case, r Row) Row                                                             { return r }
func Complete(cases []Case, rows []Row) error                                             { return nil }
func SummarizeSplit(cases []Case, rows []Row) (SplitSummary, error)                       { return SplitSummary{}, nil }
func Gate(rows []Row) error                                                               { return nil }
func Percentile(v []int64, p float64) int64                                               { return 0 }
func FrozenGuard(git func(args ...string) (string, error), sha, resultsPath string) error { return nil }

// Candidate is the frozen set of engines under test.
type Candidate struct{}

func RunCase(ctx context.Context, cand *Candidate, c Case) Row { return Row{} }

// Pools are the precedent rulings each specialist retrieves from.
type Pools struct{ Textile, Pharma, Part102 []domain.Ruling }

func NewCandidate(m llm.Model, modelID string, p Pools) (*Candidate, error) { return &Candidate{}, nil }

func (c *Candidate) WrapRetrievers(f func(retrieval.Retriever) retrieval.Retriever) {}
func LoadAll(dir string) (dev, test []Case, err error)                              { return nil, nil, nil }
