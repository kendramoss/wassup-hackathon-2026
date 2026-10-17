package engine

import (
	"context"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/blob"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
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
		return llm.Response{ModelID: model, Text: `{"origin_country":"` + origin + `","essential_character":"motor","decisive_step":"step","flip":null,"confidence":0.8,"reasoning":"r"}`}, nil
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
