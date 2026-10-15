package retrieval

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"sort"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// EmbedRunes is how much of a ruling's facts is embedded (both precedents and
// queries, so similarity is symmetric).
const EmbedRunes = 2000

// EmbedText is the text embedded for a facts string.
func EmbedText(facts string) string { return corpus.Truncate(facts, EmbedRunes) }

// TextHash identifies an embedded text, so a query whose facts are already
// in the index (eval cases) reuses the stored vector.
func TextHash(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:16])
}

// Vectors is an embedding index: one unit vector per ruling.
type Vectors struct {
	Model  string
	Dims   int
	byID   map[string][]float32
	byHash map[string][]float32
}

// NewVectors returns an empty index.
func NewVectors(model string, dims int) *Vectors {
	return &Vectors{Model: model, Dims: dims, byID: map[string][]float32{}, byHash: map[string][]float32{}}
}

// Add stores a ruling's vector and the hash of the text it was made from.
func (v *Vectors) Add(id, hash string, vec []float32) {
	v.byID[id] = vec
	v.byHash[hash] = vec
}

// Get returns a ruling's vector.
func (v *Vectors) Get(id string) ([]float32, bool) { x, ok := v.byID[id]; return x, ok }

// Len is the number of vectors.
func (v *Vectors) Len() int { return len(v.byID) }

type vectorHeader struct {
	Model  string   `json:"model"`
	Dims   int      `json:"dims"`
	IDs    []string `json:"ids"`
	Hashes []string `json:"hashes"`
}

// Save writes the index: one JSON header line, then float32 little-endian
// vectors in header order.
func (v *Vectors) Save(path string) error {
	ids := make([]string, 0, len(v.byID))
	for id := range v.byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	hashes := make([]string, len(ids))
	rev := map[*float32]string{}
	for h, vec := range v.byHash {
		if len(vec) > 0 {
			rev[&vec[0]] = h
		}
	}
	for i, id := range ids {
		if vec := v.byID[id]; len(vec) > 0 {
			hashes[i] = rev[&vec[0]]
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	hdr, _ := json.Marshal(vectorHeader{Model: v.Model, Dims: v.Dims, IDs: ids, Hashes: hashes})
	if _, err := w.Write(append(hdr, '\n')); err != nil {
		return err
	}
	for _, id := range ids {
		if err := binary.Write(w, binary.LittleEndian, v.byID[id]); err != nil {
			return err
		}
	}
	return w.Flush()
}

// LoadVectors reads an index written by Save.
func LoadVectors(path string) (*Vectors, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("retrieval: open vectors: %w", err)
	}
	defer f.Close()
	r := bufio.NewReader(f)
	line, err := r.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("retrieval: vectors header: %w", err)
	}
	var h vectorHeader
	if err := json.Unmarshal(line, &h); err != nil {
		return nil, fmt.Errorf("retrieval: vectors header: %w", err)
	}
	v := NewVectors(h.Model, h.Dims)
	for i, id := range h.IDs {
		vec := make([]float32, h.Dims)
		if err := binary.Read(r, binary.LittleEndian, vec); err != nil {
			return nil, fmt.Errorf("retrieval: vector %s: %w", id, err)
		}
		v.Add(id, h.Hashes[i], vec)
	}
	if _, err := r.ReadByte(); err != io.EOF {
		return nil, fmt.Errorf("retrieval: vectors file has trailing data")
	}
	return v, nil
}

func dot(a, b []float32) float64 {
	var s float64
	for i := range a {
		s += float64(a[i]) * float64(b[i])
	}
	return s
}

// Hybrid retrieves BM25 candidates and re-ranks them by embedding
// similarity, so same-product rulings come ahead of rulings that merely share
// vocabulary. Candidates = 0 ranks every allowed ruling by embedding alone.
type Hybrid struct {
	BM25       *BM25
	Vectors    *Vectors
	Embedder   llm.Embedder
	Candidates int
}

// Search implements Retriever.
func (h *Hybrid) Search(ctx context.Context, query string, k int, allow func(domain.Ruling) bool) ([]Hit, error) {
	text := EmbedText(query)
	q, ok := h.Vectors.byHash[TextHash(text)]
	if !ok {
		vecs, _, err := h.Embedder.Embed(ctx, h.Vectors.Model, []string{text}, h.Vectors.Dims)
		if err != nil {
			return nil, fmt.Errorf("retrieval: embed query: %w", err)
		}
		q = vecs[0]
	}
	var pool []Hit
	if h.Candidates > 0 {
		pool = h.BM25.Top(query, h.Candidates, allow)
	} else {
		for _, r := range h.BM25.docs {
			if allow(r) {
				pool = append(pool, Hit{Ruling: r})
			}
		}
	}
	type scored struct {
		hit Hit
		sim float64
	}
	var ranked []scored
	for _, c := range pool {
		vec, ok := h.Vectors.Get(c.Ruling.ID)
		if !ok {
			continue
		}
		ranked = append(ranked, scored{c, dot(q, vec)})
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].sim > ranked[j].sim })
	if len(ranked) > k {
		ranked = ranked[:k]
	}
	out := make([]Hit, len(ranked))
	for i, s := range ranked {
		out[i] = Hit{Ruling: s.hit.Ruling, Score: math.Round(s.sim*1e4) / 1e4}
	}
	return out, nil
}

var _ Retriever = (*Hybrid)(nil)
