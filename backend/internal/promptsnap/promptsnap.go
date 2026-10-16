// Package promptsnap fingerprints every model prompt together with its
// version constant (D72). snapshot.json holds a SHA-256 per prompt; the
// package test fails when a prompt's text changes while its version stays
// the same, so a prompt edit cannot ship under an old version name and every
// recorded result keeps meaning what its prompt_version says.
package promptsnap

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/caseagent"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/clarify"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/verifier"
)

// Prompt is one model instruction and the version it is recorded under.
type Prompt struct {
	Name, Version, Text string
}

// Fixed inputs for the prompts that are rendered by code (user-message
// templates): a change to the template changes the rendered text.
var (
	fixtureFacts = "The widget's motor is made in China and its housing in Vietnam; final assembly is in Vietnam."
	fixtureDate  = time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
)

// Prompts returns every prompt the backend sends to a model, with its JSON
// schema where it has one (a schema change changes the answer too).
func Prompts() []Prompt {
	prec := []domain.Ruling{{ID: "N300001", Date: fixtureDate, Facts: "A motor made in China is fitted in Vietnam.",
		Holding: "The country of origin is China.", Reasoning: "The motor imparts the essential character."}}
	answers, _ := clarify.AppendAnswers(fixtureFacts, []clarify.Answer{
		{Question: "Where is the motor wound?", Answer: "In China. The supplier did not say where the magnets are made."}})
	task := verifier.Task{FactsA: fixtureFacts, FactsB: fixtureFacts + " The motor moves to Thailand.",
		Target: "Thailand", Before: fixtureDate, KnownOriginA: "China", KnownRulingA: "N300001"}
	return []Prompt{
		{"engine.system", engine.PromptVersion, withSchema(engine.System, engine.Schema)},
		{"engine.user", engine.PromptVersion, engine.RenderUser(fixtureFacts, prec)},
		{"engine.rules", engine.RulesVersion, engine.Rules},
		{"engine.textile", engine.TextileVersion, engine.TextileSystem},
		{"engine.textile2", engine.TextileVersion2, engine.TextileSystem2},
		{"router.system", router.PromptVersion, withSchema(router.System, router.Schema)},
		{"clarify.system", clarify.PromptVersion, withSchema(clarify.System, clarify.Schema)},
		{"clarify.answers", clarify.AnswersVersion, answers},
		{"verifier.system", verifier.PromptVersion, verifier.System(6)},
		{"verifier.task", verifier.PromptVersion, task.UserMessage()},
		{"caseagent.system", caseagent.PromptVersion, caseagent.System},
	}
}

func withSchema(system string, schema map[string]any) string {
	b, err := json.Marshal(schema) // map keys are sorted, so this is stable
	if err != nil {
		panic(fmt.Sprintf("promptsnap: schema: %v", err))
	}
	return system + "\n\n[schema] " + string(b)
}

// Entry is one prompt's fingerprint.
type Entry struct {
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

// Snapshot is snapshot.json.
type Snapshot struct {
	Doc     string           `json:"_doc"`
	Prompts map[string]Entry `json:"prompts"`
}

const doc = "SHA-256 of every model prompt and the version it is recorded under (D72). " +
	"Regenerate with: cd backend && go test ./internal/promptsnap -update. " +
	"The test refuses to update while a prompt's text changed and its version did not."

// Take fingerprints the current prompts.
func Take() Snapshot {
	s := Snapshot{Doc: doc, Prompts: map[string]Entry{}}
	for _, p := range Prompts() {
		s.Prompts[p.Name] = Entry{Version: p.Version, SHA256: fingerprint(p.Text)}
	}
	return s
}

func fingerprint(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// Load reads a snapshot file.
func Load(path string) (Snapshot, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Snapshot{}, fmt.Errorf("promptsnap: %w", err)
	}
	var s Snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return Snapshot{}, fmt.Errorf("promptsnap: %s: %w", path, err)
	}
	return s, nil
}

// Write saves the snapshot as indented JSON.
func (s Snapshot) Write(path string) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// Kind classifies a difference between the committed and current snapshot.
type Kind int

const (
	// Unbumped: the text changed and the version did not. Never accepted.
	Unbumped Kind = iota + 1
	// Stale: the snapshot is behind (a version was bumped, a prompt was
	// added or removed). Accepted by regenerating the snapshot.
	Stale
)

// Problem is one difference.
type Problem struct {
	Name string
	Kind Kind
	Msg  string
}

// Diff compares the committed snapshot (old) with the current one (cur),
// in prompt-name order.
func Diff(old, cur Snapshot) []Problem {
	var out []Problem
	names := map[string]bool{}
	for n := range old.Prompts {
		names[n] = true
	}
	for n := range cur.Prompts {
		names[n] = true
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	slices.Sort(sorted)
	for _, n := range sorted {
		o, inOld := old.Prompts[n]
		c, inCur := cur.Prompts[n]
		switch {
		case !inOld:
			out = append(out, Problem{n, Stale, fmt.Sprintf("new prompt %s (%s) is not in the snapshot", n, c.Version)})
		case !inCur:
			out = append(out, Problem{n, Stale, fmt.Sprintf("prompt %s (%s) was removed", n, o.Version)})
		case o.SHA256 != c.SHA256 && o.Version == c.Version:
			out = append(out, Problem{n, Unbumped, fmt.Sprintf(
				"prompt %s changed but its version is still %q: bump the version constant, run the live slice "+
					"(cd backend && EVAL_SLICE=on go run ./cmd/eval slice, or the \"Eval slice\" workflow) "+
					"and record the result, then go test ./internal/promptsnap -update", n, c.Version)})
		case o.Version != c.Version:
			out = append(out, Problem{n, Stale, fmt.Sprintf("prompt %s moved from %s to %s", n, o.Version, c.Version)})
		}
	}
	return out
}
