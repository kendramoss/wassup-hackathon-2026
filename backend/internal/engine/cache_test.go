package engine

import (
	"context"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/blob"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/triage"
)

func TestCacheReplaysPredictions(t *testing.T) {
	f := llm.FakeText(`{"origin_country":"Thailand","essential_character":"motor","decisive_step":"motor in Thailand","flip":null,"confidence":0.93,"reasoning":"r"}`)
	e := New(f, nil, Options{Model: "m"})
	dir := blob.Dir{Dir: t.TempDir()}
	a, b := NewCache(e, dir), NewCache(e, dir) // two instances sharing a store
	c := domain.Case{ID: "case_1", Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Facts: "motor made in Thailand", Exclude: []string{"N2", "N1"}}
	first, err := a.Predict(context.Background(), c)
	if err != nil || first.Prediction.Trace.Cached {
		t.Fatalf("first = %+v %v", first.Prediction.Trace, err)
	}
	c.ID, c.Exclude = "case_2", []string{"N1", "N2"} // another case with the same facts; exclusion order does not matter
	again, err := b.Predict(context.Background(), c)
	if err != nil || !again.Prediction.Trace.Cached || again.Prediction.Origin != "Thailand" || again.Prediction.Trace.InputTokens != 0 || len(f.Calls()) != 1 {
		t.Errorf("replay = %+v, calls %d", again.Prediction, len(f.Calls()))
	}
	c.Date = c.Date.AddDate(0, 1, 0) // a later date sees more precedents: a new prediction
	if r, _ := a.Predict(context.Background(), c); r.Prediction.Trace.Cached || len(f.Calls()) != 2 {
		t.Errorf("new date replayed: calls %d", len(f.Calls()))
	}
	other := NewCache(New(f, nil, Options{Model: "m", System: TextileSystem, Version: TextileVersion}), dir)
	if r, _ := other.Predict(context.Background(), c); r.Prediction.Trace.Cached {
		t.Error("a different prompt version reused the cache")
	}
	argonKey, flashKey := a.key("gemini-4-argon", c), a.key("tuned-flash", c)
	if argonKey == flashKey {
		t.Fatal("cache key ignored the model")
	}
	c.ID = "N350001" // an eval row: never cached
	before := len(f.Calls())
	a.Predict(context.Background(), c)
	a.Predict(context.Background(), c)
	if len(f.Calls()) != before+2 {
		t.Errorf("ruling-ID cases were cached")
	}
}

func TestCacheFallbackDoesNotBlockPrimary(t *testing.T) {
	var calls int
	f := &llm.Fake{Respond: func(req llm.Request) (llm.Response, error) {
		calls++
		origin, model := "Thailand", "tuned-flash"
		if calls > 1 {
			origin, model = "China", req.Model
		}
		return llm.Response{ModelID: model, RequestedModel: "gemini-4-argon", Fallback: calls == 1, Text: `{"origin_country":"` + origin + `","essential_character":"motor","decisive_step":"step","flip":null,"confidence":0.8,"reasoning":"r"}`}, nil
	}}
	dir := blob.Dir{Dir: t.TempDir()}
	primary := NewCache(New(f, nil, Options{Model: "gemini-4-argon"}), dir)
	c := domain.Case{ID: "case_fb", Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Facts: "motor made in Thailand"}
	first, err := primary.Predict(context.Background(), c)
	if err != nil || first.Prediction.Trace.ModelID != "tuned-flash" || first.Prediction.Origin != "Thailand" || first.Prediction.Trace.Cached {
		t.Fatalf("first = %+v %v", first.Prediction, err)
	}
	// The fallback answer is readable under the fallback model's key.
	flash := NewCache(New(f, nil, Options{Model: "tuned-flash"}), dir)
	replay, err := flash.Predict(context.Background(), c)
	if err != nil || !replay.Prediction.Trace.Cached || replay.Prediction.Origin != "Thailand" || calls != 1 {
		t.Fatalf("fallback replay = %+v calls %d err %v", replay.Prediction, calls, err)
	}
	// The primary key misses, so a later Argon call is not blocked.
	second, err := primary.Predict(context.Background(), c)
	if err != nil || second.Prediction.Trace.Cached || second.Prediction.Origin != "China" || second.Prediction.Trace.ModelID != "gemini-4-argon" {
		t.Fatalf("primary again = %+v err %v", second.Prediction, err)
	}
}

// #344: Vertex may answer the configured alias with a version string. That is
// still a primary answer: it must replay from the alias's cache key and must
// not be routed as fallback just because the strings differ.
func TestCacheReplaysVersionSuffixedPrimary(t *testing.T) {
	calls := 0
	f := &llm.Fake{Respond: func(req llm.Request) (llm.Response, error) {
		calls++
		return llm.Response{ModelID: "gemini-4-argon-001", RequestedModel: req.Model, Text: `{"origin_country":"Thailand","essential_character":"motor","decisive_step":"step","flip":null,"confidence":0.8,"reasoning":"r"}`}, nil
	}}
	dir := blob.Dir{Dir: t.TempDir()}
	a, b := NewCache(New(f, nil, Options{Model: "gemini-4-argon"}), dir), NewCache(New(f, nil, Options{Model: "gemini-4-argon"}), dir)
	c := domain.Case{ID: "case_alias", Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Facts: "motor made in Thailand"}
	first, err := a.Predict(context.Background(), c)
	if err != nil || first.Prediction.Trace.Cached {
		t.Fatalf("first = %+v %v", first.Prediction, err)
	}
	second, err := b.Predict(context.Background(), c)
	if err != nil || !second.Prediction.Trace.Cached || calls != 1 {
		t.Fatalf("replay = %+v calls %d err %v", second.Prediction, calls, err)
	}
	if got, want := second.Prediction.Trace.ModelID, "gemini-4-argon-001"; got != want {
		t.Errorf("resolved version = %q, want %q", got, want)
	}
	if got, want := second.Prediction.Trace.RequestedModel, "gemini-4-argon"; got != want {
		t.Errorf("requested model = %q, want %q", got, want)
	}
	if triage.AnsweredByFallback(second.Prediction.Trace, "gemini-4-argon") {
		t.Error("version-suffixed primary was routed as fallback")
	}
}

// #344: a real fallback answer is kept in its own key. It cannot satisfy a
// later primary lookup, so primary recovery still runs and then caches.
func TestGenuineFallbackDoesNotPoisonPrimaryCache(t *testing.T) {
	var primaryCalls, fallbackCalls int
	primary := &llm.Fake{Respond: func(llm.Request) (llm.Response, error) {
		primaryCalls++
		if primaryCalls == 1 {
			return llm.Response{}, &llm.APIError{Provider: "vertex", StatusCode: 429, Body: "Resource exhausted"}
		}
		return llm.Response{Text: `{"origin_country":"China","essential_character":"motor","decisive_step":"step","flip":null,"confidence":0.8,"reasoning":"r"}`}, nil
	}}
	fallback := &llm.Fake{Respond: func(llm.Request) (llm.Response, error) {
		fallbackCalls++
		return llm.Response{Text: `{"origin_country":"Thailand","essential_character":"motor","decisive_step":"step","flip":null,"confidence":0.8,"reasoning":"r"}`}, nil
	}}
	dir := blob.Dir{Dir: t.TempDir()}
	e := NewCache(New(llm.WithFallback(primary, fallback, "tuned-flash"), nil, Options{Model: "gemini-4-argon"}), dir)
	c := domain.Case{ID: "case_fb_recovery", Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Facts: "motor made in Thailand"}
	first, err := e.Predict(context.Background(), c)
	if err != nil || first.Prediction.Origin != "Thailand" || !first.Prediction.Trace.Fallback {
		t.Fatalf("fallback = %+v %v", first.Prediction, err)
	}
	second, err := e.Predict(context.Background(), c)
	if err != nil || second.Prediction.Trace.Cached || second.Prediction.Origin != "China" || second.Prediction.Trace.Fallback {
		t.Fatalf("primary recovery = %+v %v", second.Prediction, err)
	}
	third, err := e.Predict(context.Background(), c)
	if err != nil || !third.Prediction.Trace.Cached || third.Prediction.Origin != "China" {
		t.Fatalf("primary replay = %+v %v", third.Prediction, err)
	}
	if primaryCalls != 2 || fallbackCalls != 1 {
		t.Fatalf("calls: primary %d, fallback %d; want 2 and 1", primaryCalls, fallbackCalls)
	}
}
