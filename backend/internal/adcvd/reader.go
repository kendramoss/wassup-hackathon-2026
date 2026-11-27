package adcvd

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

//go:embed data/reader-prompt.json
var readerPromptJSON []byte

// ReaderPromptVersion is the prompt of the scope reader that D942 measured.
const ReaderPromptVersion = "adcvd-reader-v3"

// Limits of one reader question.
const (
	readerCallTimeout = 25 * time.Second
	readerScopeRunes  = 14000
	readerDescRunes   = 600
	readerCacheMax    = 4096
)

type readerPrompt struct {
	Version string `json:"version"`
	System  string `json:"system"`
	User    string `json:"user"`
}

// PairKey names one question to the scope reader: the order's name, country
// and written scope and the product description. It is the key of the cache
// and of the recorded answers of the evaluation.
func PairKey(o Order, name string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s", o.Product, o.Country, o.ScopeText, name)
	return hex.EncodeToString(h.Sum(nil))[:24]
}

// LLMReader is a Reader that asks a model. It keeps answers in memory for the
// life of the process and never retries; an error is "no answer", which
// removes nothing.
type LLMReader struct {
	model   llm.Model
	modelID string
	prompt  readerPrompt

	mu    sync.Mutex
	cache map[string]Verdict
}

// NewLLMReader returns a reader that asks modelID of m.
func NewLLMReader(m llm.Model, modelID string) (*LLMReader, error) {
	r := &LLMReader{model: m, modelID: modelID, cache: map[string]Verdict{}}
	if err := json.Unmarshal(readerPromptJSON, &r.prompt); err != nil || r.prompt.Version != ReaderPromptVersion || r.prompt.System == "" || r.prompt.User == "" {
		return nil, fmt.Errorf("adcvd: reader prompt is not %s", ReaderPromptVersion)
	}
	return r, nil
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// scopeForReader keeps paragraph breaks and caps the length.
func scopeForReader(s string) string {
	if r := []rune(s); len(r) > readerScopeRunes {
		return string(r[:readerScopeRunes])
	}
	return s
}

// Read implements Reader.
func (r *LLMReader) Read(p Part, o Order) (Verdict, error) {
	name := clip(p.Name, readerDescRunes)
	if name == "" || strings.TrimSpace(o.ScopeText) == "" {
		return "", nil
	}
	key := PairKey(o, name)
	r.mu.Lock()
	v, ok := r.cache[key]
	r.mu.Unlock()
	if ok {
		return v, nil
	}
	user := strings.NewReplacer("{product}", o.Product, "{country}", o.Country, "{scope}", scopeForReader(o.ScopeText), "{description}", name).Replace(r.prompt.User)
	ctx, cancel := context.WithTimeout(context.Background(), readerCallTimeout)
	defer cancel()
	resp, err := r.model.Generate(ctx, llm.Request{Model: r.modelID, System: r.prompt.System,
		Messages: []llm.Message{{Role: llm.RoleUser, Content: user}}, MaxOutputTokens: 400})
	if err != nil {
		return "", err
	}
	var a struct {
		Verdict string `json:"verdict"`
	}
	if err := resp.DecodeJSON(&a); err != nil {
		return "", err
	}
	switch Verdict(strings.ToLower(strings.TrimSpace(a.Verdict))) {
	case VerdictIn:
		v = VerdictIn
	case VerdictOut:
		v = VerdictOut
	default:
		v = VerdictUnclear
	}
	r.mu.Lock()
	if len(r.cache) >= readerCacheMax {
		r.cache = map[string]Verdict{}
	}
	r.cache[key] = v
	r.mu.Unlock()
	return v, nil
}
