package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/clarify"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jpcorpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jpengine"
)

// The answered splits (D921) measure the product after one round of the
// question card (D917). Each re-runs only the cases its base run left
// undetermined and for which the card asks a question, with the answer
// taken from the ruling's own statement of which materials are originating
// (eval/jp/answer_oracle.py) and appended exactly as the product appends it
// (clarify.AppendAnswers). Every other case keeps its base answer, as in the
// product, where the card shows only on an undetermined result.
const answersFile = "answers-d921.jsonl"

var answeredBase = map[string]struct{ split, run string }{
	"dev_eval_answered":   {"dev_eval", "jp-dev_eval-gemini-4-argon-jp-engine-v1-litellm-d909-dev"},
	"dev_neg_answered":    {"dev_neg", "jp-dev_neg-gemini-4-argon-jp-engine-v1-litellm-d913-neg-v1"},
	"fresh_1126_answered": {"fresh_1126", "jp-fresh_1126-gemini-4-argon-jp-engine-v1-litellm-d919-fresh"},
}

func isAnswered(split string) bool { _, ok := answeredBase[split]; return ok }

// baseSplit is the split whose cases and labels a split scores.
func baseSplit(split string) string {
	if b, ok := answeredBase[split]; ok {
		return b.split
	}
	return split
}

func loadAnswers() (map[string]string, error) {
	b, err := os.ReadFile(filepath.Join(outDir, answersFile))
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var r struct{ ID, Answer string }
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			return nil, err
		}
		out[r.ID] = r.Answer
	}
	return out, nil
}

// answeredCases returns the base split's cases that the card re-checks, with
// the answer appended to their facts.
func answeredCases(split string) ([]candidateInput, *jpcorpus.Pool, string, error) {
	b := answeredBase[split]
	items, pool, labelHash, err := loadCandidate(b.split)
	if err != nil {
		return nil, nil, "", err
	}
	rows, err := readCandidateRows(candidateRowsPath(b.run))
	if err != nil {
		return nil, nil, "", err
	}
	undetermined := map[string]bool{}
	for _, r := range rows {
		if r.Prediction.Japan != nil && r.Prediction.Japan.Outcome == jpengine.Undetermined {
			undetermined[r.ID] = true
		}
	}
	answers, err := loadAnswers()
	if err != nil {
		return nil, nil, "", err
	}
	var out []candidateInput
	for _, it := range items {
		id := it.label.ID
		if !undetermined[id] || it.caseValue.Japan == nil {
			continue
		}
		qs := jpengine.Questions(it.caseValue, it.caseValue.Japan.Agreement)
		a := answers[id]
		if len(qs) == 0 || a == "" {
			continue
		}
		facts, ok := clarify.AppendAnswers(it.caseValue.Facts, []clarify.Answer{{Question: qs[0].Text, Answer: a}})
		if !ok {
			return nil, nil, "", fmt.Errorf("answer for %s states no fact", id)
		}
		it.caseValue.Facts = facts
		out = append(out, it)
	}
	return out, pool, labelHash, nil
}
