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
	c.ID = "N350001" // an eval row: never cached
	before := len(f.Calls())
	a.Predict(context.Background(), c)
	a.Predict(context.Background(), c)
	if len(f.Calls()) != before+2 {
		t.Errorf("ruling-ID cases were cached")
	}
}
