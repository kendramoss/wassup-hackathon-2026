package engine

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/blob"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
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

// verRetr is a fake retriever over fixed rulings that reports a revision.
type verRetr struct {
	rulings []domain.Ruling
	rev     string
}

func (v verRetr) Search(_ context.Context, _ string, k int, allow func(domain.Ruling) bool) ([]retrieval.Hit, error) {
	var out []retrieval.Hit
	for _, r := range v.rulings {
		if allow(r) && len(out) < k {
			out = append(out, retrieval.Hit{Ruling: r})
		}
	}
	return out, nil
}
func (v verRetr) Revision() string { return v.rev }

// plainRetr has no Revision method.
type plainRetr struct{ verRetr }

func (plainRetr) Revision() {} // wrong signature on purpose: not Revisioned

func ruling(id, facts string) domain.Ruling {
	return domain.Ruling{ID: id, Date: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), Facts: facts, Origin: "Thailand"}
}

// citeFirst answers citing the first precedent shown to the model.
func citeFirst() *llm.Fake {
	return &llm.Fake{Respond: func(req llm.Request) (llm.Response, error) {
		id := rulingRef.FindString(req.Messages[0].Content)
		return llm.Response{Text: `{"origin_country":"Thailand","essential_character":"motor","decisive_step":"like ` + id + `","flip":null,"confidence":0.9,"reasoning":"see ` + id + `"}`}, nil
	}}
}

var evCase = domain.Case{ID: "case_ev", Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Facts: "motor made in Thailand"}

func TestCacheEvidenceRevision(t *testing.T) {
	base := []domain.Ruling{ruling("N100001", "motor"), ruling("N100002", "gearbox")}
	cases := []struct {
		name      string
		second    verRetr
		wantCalls int
		wantCite  string
	}{
		{"unchanged", verRetr{base, "r1"}, 1, "N100001"},
		{"added", verRetr{append([]domain.Ruling{ruling("N100000", "motor and gear")}, base...), "r2"}, 2, "N100000"},
		{"removed", verRetr{base[1:], "r3"}, 2, "N100002"},
		{"corrected", verRetr{[]domain.Ruling{ruling("N100001", "motor, corrected"), base[1]}, "r4"}, 2, "N100001"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := citeFirst()
			dir := blob.Dir{Dir: t.TempDir()}
			a := NewCache(New(f, verRetr{base, "r1"}, Options{Model: "m", K: 2}), dir)
			b := NewCache(New(f, tc.second, Options{Model: "m", K: 2}), dir) // restarted server over a changed corpus
			if _, err := a.Predict(context.Background(), evCase); err != nil {
				t.Fatal(err)
			}
			got, err := b.Predict(context.Background(), evCase)
			if err != nil || len(f.Calls()) != tc.wantCalls || got.Prediction.Trace.Cached != (tc.wantCalls == 1) {
				t.Fatalf("calls %d cached %v err %v", len(f.Calls()), got.Prediction.Trace.Cached, err)
			}
			if len(got.Prediction.CitedRulings) != 1 || got.Prediction.CitedRulings[0] != tc.wantCite {
				t.Errorf("cited %v, want %s", got.Prediction.CitedRulings, tc.wantCite)
			}
			_, peek, _ := b.Peek(context.Background(), evCase)
			if !peek {
				t.Error("answer not stored for the current revision")
			}
		})
	}
}

func TestCacheBypassesUnversionedRetriever(t *testing.T) {
	rs := []domain.Ruling{ruling("N100001", "motor")}
	for name, r := range map[string]retrieval.Retriever{
		"no Revision method": plainRetr{verRetr{rs, "x"}},
		"empty revision":     verRetr{rs, ""},
	} {
		t.Run(name, func(t *testing.T) {
			f := citeFirst()
			dir := blob.Dir{Dir: t.TempDir()}
			c := NewCache(New(f, r, Options{Model: "m", K: 1}), dir)
			c.Predict(context.Background(), evCase)
			again, _ := c.Predict(context.Background(), evCase)
			if len(f.Calls()) != 2 || again.Prediction.Trace.Cached {
				t.Errorf("calls %d: persisted cache used without a trustworthy revision", len(f.Calls()))
			}
			if _, ok, _ := c.Peek(context.Background(), evCase); ok {
				t.Error("Peek hit without a trustworthy revision")
			}
			if ents, _ := os.ReadDir(dir.Dir); len(ents) != 0 {
				t.Errorf("wrote %d cache entries", len(ents))
			}
		})
	}
}

func TestCacheFactsOnlyKeepsCaching(t *testing.T) { // no retriever = no evidence to go stale
	f := citeFirst()
	dir := blob.Dir{Dir: t.TempDir()}
	c := NewCache(New(f, nil, Options{Model: "m"}), dir)
	c.Predict(context.Background(), evCase)
	if r, _ := c.Predict(context.Background(), evCase); !r.Prediction.Trace.Cached || len(f.Calls()) != 1 {
		t.Error("facts-only engine lost its cache")
	}
}

// TestCacheProductionWrappers builds the retrievers as cmd/server does:
// BM25 over store rulings stamped with the store revision, and Hybrid over it.
func TestCacheProductionWrappers(t *testing.T) {
	rs := []domain.Ruling{ruling("N100001", "motor assembled"), ruling("N100002", "gearbox assembled")}
	corrected := []domain.Ruling{ruling("N100001", "motor assembled, corrected"), rs[1]}
	vecs := retrieval.NewVectors("emb", 2)
	vecs.Add("N100001", "h1", []float32{1, 0})
	vecs.Add("N100002", "h2", []float32{0, 1})
	vecs.Add("q", retrieval.TextHash(retrieval.EmbedText(evCase.Facts)), []float32{1, 0})
	build := map[string]func(rs []domain.Ruling, storeRev string) retrieval.Retriever{
		"bm25": func(rs []domain.Ruling, rev string) retrieval.Retriever {
			return retrieval.NewBM25(rs, PrecedentText).WithRevision(rev)
		},
		"hybrid": func(rs []domain.Ruling, rev string) retrieval.Retriever {
			return &retrieval.Hybrid{BM25: retrieval.NewBM25(rs, PrecedentText).WithRevision(rev), Vectors: vecs}
		},
		"bm25 unstamped": func(rs []domain.Ruling, _ string) retrieval.Retriever { return retrieval.NewBM25(rs, PrecedentText) },
		"hybrid unstamped": func(rs []domain.Ruling, _ string) retrieval.Retriever {
			return &retrieval.Hybrid{BM25: retrieval.NewBM25(rs, PrecedentText), Vectors: vecs}
		},
	}
	for name, mk := range build {
		t.Run(name, func(t *testing.T) {
			stamped := !strings.Contains(name, "unstamped")
			run := func(second []domain.Ruling, secondRev string) (calls int) {
				f := citeFirst()
				dir := blob.Dir{Dir: t.TempDir()}
				a := NewCache(New(f, mk(rs, "store-1"), Options{Model: "m", K: 2}), dir)
				b := NewCache(New(f, mk(second, secondRev), Options{Model: "m", K: 2}), dir)
				a.Predict(context.Background(), evCase)
				b.Predict(context.Background(), evCase)
				return len(f.Calls())
			}
			if got, want := run(rs, "store-1"), map[bool]int{true: 1, false: 2}[stamped]; got != want {
				t.Errorf("unchanged: %d calls, want %d", got, want)
			}
			if got := run(corrected, "store-2"); got != 2 {
				t.Errorf("corrected: %d calls, want 2", got)
			}
		})
	}
}
