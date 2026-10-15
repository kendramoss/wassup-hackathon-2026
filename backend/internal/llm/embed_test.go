package llm

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLiteLLMEmbed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &in)
		if r.URL.Path != "/v1/embeddings" || in["dimensions"] != 768.0 || in["model"] != "gemini-embedding-001" {
			t.Errorf("request %s %v", r.URL.Path, in)
		}
		_, _ = io.WriteString(w, `{"data":[{"index":1,"embedding":[0,2]},{"index":0,"embedding":[3,4]}],"usage":{"prompt_tokens":9}}`)
	}))
	defer srv.Close()
	vecs, u, err := NewLiteLLM(srv.URL, "k", time.Second).Embed(context.Background(), "gemini-embedding-001", []string{"a", "b"}, 768)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(float64(vecs[0][0])-0.6) > 1e-6 || vecs[1][1] != 1 || u.InputTokens != 9 {
		t.Errorf("vecs %v usage %+v (want order by index, unit length)", vecs, u)
	}
}

func TestNormalize(t *testing.T) {
	if v := Normalize([]float32{3, 4}); math.Abs(float64(v[0])-0.6) > 1e-6 {
		t.Errorf("Normalize = %v", v)
	}
	if v := Normalize([]float32{0, 0}); v[0] != 0 {
		t.Error("zero vector should stay zero")
	}
}
