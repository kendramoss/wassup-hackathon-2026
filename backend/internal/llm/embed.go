package llm

import (
	"context"
	"fmt"
	"math"

	"google.golang.org/genai"
)

// Embedder turns texts into vectors (gemini-embedding-001). Vectors are
// returned unit-length so a dot product is cosine similarity.
type Embedder interface {
	Embed(ctx context.Context, model string, texts []string, dims int) ([][]float32, Usage, error)
}

// Embed implements Embedder via the Gen AI SDK on Vertex AI.
func (v *Vertex) Embed(ctx context.Context, model string, texts []string, dims int) ([][]float32, Usage, error) {
	contents := make([]*genai.Content, len(texts))
	for i, t := range texts {
		contents[i] = genai.NewContentFromText(t, genai.RoleUser)
	}
	d := int32(dims)
	resp, err := v.client.Models.EmbedContent(ctx, model, contents, &genai.EmbedContentConfig{OutputDimensionality: &d})
	if err != nil {
		return nil, Usage{}, mapVertexError(ctx, err)
	}
	if len(resp.Embeddings) != len(texts) {
		return nil, Usage{}, fmt.Errorf("vertex: embed: %w: got %d vectors for %d texts", ErrBadResponse, len(resp.Embeddings), len(texts))
	}
	vecs := make([][]float32, len(texts))
	for i, e := range resp.Embeddings {
		vecs[i] = Normalize(e.Values)
	}
	return vecs, Usage{}, nil
}

// Normalize returns v scaled to unit length (v unchanged if zero).
func Normalize(v []float32) []float32 {
	var s float64
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	if s == 0 {
		return v
	}
	n := float32(1 / math.Sqrt(s))
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = x * n
	}
	return out
}

// FakeEmbedder returns deterministic vectors for tests: Vector(text) decides.
type FakeEmbedder struct {
	Vector func(text string) []float32
}

// Embed implements Embedder.
func (f FakeEmbedder) Embed(_ context.Context, _ string, texts []string, _ int) ([][]float32, Usage, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = Normalize(f.Vector(t))
	}
	return out, Usage{}, nil
}

var _ Embedder = (*Vertex)(nil)
