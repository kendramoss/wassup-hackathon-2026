package engine

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/blob"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

const identityJSON = `{"origin_country":"Thailand","essential_character":"motor","decisive_step":"d","flip":null,"confidence":0.93,"reasoning":"r"}`

var identityCase = domain.Case{ID: "c", Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Facts: "motor made in Thailand"}

// #344: an alias answered with a resolved version is stored where Peek reads.
func TestCacheAliasResolvedVersionReplays(t *testing.T) {
	f := &llm.Fake{Respond: func(llm.Request) (llm.Response, error) {
		return llm.Response{Text: identityJSON, ModelID: "m-001"}, nil
	}}
	c := NewCache(New(f, nil, Options{Model: "m"}), blob.Dir{Dir: t.TempDir()})
	first, err := c.Predict(context.Background(), identityCase)
	if err != nil || first.Prediction.Trace.ModelID != "m-001" || first.Prediction.Trace.RequestedModel != "m" {
		t.Fatalf("first = %+v %v", first.Prediction.Trace, err)
	}
	again, err := c.Predict(context.Background(), identityCase)
	if err != nil || !again.Prediction.Trace.Cached || len(f.Calls()) != 1 {
		t.Errorf("repeat cached=%v calls=%d err=%v", again.Prediction.Trace.Cached, len(f.Calls()), err)
	}
	if again.Prediction.Trace.ModelID != "m-001" || again.Prediction.Trace.RequestedModel != "m" {
		t.Errorf("replayed trace lost identity: %+v", again.Prediction.Trace)
	}
}

// Back-compat: an entry written before requested_model existed (exact-name
// primary key, no requested_model) still replays, and reads as the primary.
func TestCacheLegacyEntryReplaysAsPrimary(t *testing.T) {
	f := llm.FakeText(identityJSON)
	dir := blob.Dir{Dir: t.TempDir()}
	c := NewCache(New(f, nil, Options{Model: "m"}), dir)
	old := cached{Prediction: domain.Prediction{Origin: "Thailand", Trace: domain.Trace{ModelID: "m"}}}
	b, _ := json.Marshal(old)
	if err := dir.Put(context.Background(), c.key("m", identityCase), b); err != nil {
		t.Fatal(err)
	}
	r, err := c.Predict(context.Background(), identityCase)
	if err != nil || !r.Prediction.Trace.Cached || len(f.Calls()) != 0 || r.Prediction.Trace.RequestedModel != "m" {
		t.Errorf("legacy replay = %+v calls %d err %v", r.Prediction.Trace, len(f.Calls()), err)
	}
}

// A fallback answer lands under the fallback's key, so Peek (primary) misses.
func TestCacheFallbackAnswerStaysInFallbackNamespace(t *testing.T) {
	f := &llm.Fake{Respond: func(llm.Request) (llm.Response, error) {
		return llm.Response{Text: identityJSON, ModelID: "flash-002", RequestedModel: "flash"}, nil
	}}
	c := NewCache(New(f, nil, Options{Model: "m"}), blob.Dir{Dir: t.TempDir()})
	if _, err := c.Predict(context.Background(), identityCase); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := c.Peek(context.Background(), identityCase); ok {
		t.Error("fallback answer satisfied a primary lookup")
	}
	if _, ok, _ := c.blobs.Get(context.Background(), c.key("flash", identityCase)); !ok {
		t.Error("fallback answer not stored under the fallback identity")
	}
}
