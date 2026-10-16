package retrieval

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// topicVec maps texts to 2-d vectors by topic so similarity is predictable.
func topicVec(text string) []float32 {
	text = strings.ToLower(text)
	switch {
	case strings.Contains(text, "heat gun"):
		return []float32{1, 0.1}
	case strings.Contains(text, "hair dryer"):
		return []float32{0.9, 0.3}
	default:
		return []float32{0, 1}
	}
}

func TestVectorsSaveLoad(t *testing.T) {
	v := NewVectors("m", 2)
	v.Add("N1", "h1", llm.Normalize([]float32{1, 2}))
	v.Add("N2", "h2", llm.Normalize([]float32{3, 0}))
	path := filepath.Join(t.TempDir(), "v.bin")
	if err := v.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := LoadVectors(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Len() != 2 || got.Model != "m" || got.Dims != 2 {
		t.Fatalf("loaded %+v", got)
	}
	if x, _ := got.Get("N2"); x[0] != 1 {
		t.Errorf("vector N2 = %v", x)
	}
	if _, ok := got.byHash["h1"]; !ok {
		t.Error("hash not restored")
	}
}

func TestHybridReranksByEmbedding(t *testing.T) {
	docs := sample()
	emb := llm.FakeEmbedder{Vector: topicVec}
	v := NewVectors("m", 2)
	for _, r := range docs {
		vec, _, _ := emb.Embed(context.Background(), "m", []string{EmbedText(r.Facts)}, 2)
		v.Add(r.ID, TextHash(EmbedText(r.Facts)), vec[0])
	}
	h := &Hybrid{BM25: NewBM25(docs, facts), Vectors: v, Embedder: emb, Candidates: 5}
	c := domain.Case{ID: "X", Date: day("2026-01-14")}
	// The query shares many words with the shrimp ruling but is about a heat gun.
	hits, err := h.Search(context.Background(), "heat gun peeled cooked Indonesia frozen", 2, PrecedentFilter(c))
	if err != nil {
		t.Fatal(err)
	}
	if hits[0].Ruling.ID != "N100001" {
		t.Errorf("top hit = %s, want the heat gun ruling", hits[0].Ruling.ID)
	}
	for _, x := range hits {
		if !x.Ruling.Date.Before(c.Date) {
			t.Errorf("date filter broken: %s", x.Ruling.ID)
		}
	}
	// Candidates = 0: pure embedding ranking over all allowed rulings.
	h.Candidates = 0
	hits, _ = h.Search(context.Background(), "hair dryer", 1, PrecedentFilter(c))
	if hits[0].Ruling.ID != "N100002" {
		t.Errorf("pure-vector top = %s", hits[0].Ruling.ID)
	}
}
