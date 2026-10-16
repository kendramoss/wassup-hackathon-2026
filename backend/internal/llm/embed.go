package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"

	"google.golang.org/genai"
)

// Embedder turns texts into vectors (gemini-embedding-001). Vectors are
// returned unit-length so a dot product is cosine similarity.
type Embedder interface {
	Embed(ctx context.Context, model string, texts []string, dims int) ([][]float32, Usage, error)
}

// Embed implements Embedder via POST {BaseURL}/v1/embeddings.
func (l *LiteLLM) Embed(ctx context.Context, model string, texts []string, dims int) ([][]float32, Usage, error) {
	body, _ := json.Marshal(map[string]any{"model": model, "input": texts, "dimensions": dims})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.baseURL+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, Usage{}, fmt.Errorf("litellm: embed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+l.apiKey)
	resp, err := l.client.Do(req)
	if err != nil {
		return nil, Usage{}, fmt.Errorf("litellm: embed: %w: %s", ErrUnavailable, l.scrub(err.Error()))
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if resp.StatusCode/100 != 2 {
		return nil, Usage{}, &APIError{Provider: "litellm", StatusCode: resp.StatusCode, Body: l.scrub(truncate(string(raw), 300))}
	}
	var out struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Usage struct {
			PromptTokens int `json:"prompt_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Data) != len(texts) {
		return nil, Usage{}, fmt.Errorf("litellm: embed: %w: got %d vectors for %d texts", ErrBadResponse, len(out.Data), len(texts))
	}
	vecs := make([][]float32, len(texts))
	for _, d := range out.Data {
		if d.Index < 0 || d.Index >= len(vecs) {
			return nil, Usage{}, fmt.Errorf("litellm: embed: %w: index %d", ErrBadResponse, d.Index)
		}
		vecs[d.Index] = Normalize(d.Embedding)
	}
	return vecs, Usage{InputTokens: out.Usage.PromptTokens, TotalTokens: out.Usage.PromptTokens}, nil
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

var (
	_ Embedder = (*LiteLLM)(nil)
	_ Embedder = (*Vertex)(nil)
)
