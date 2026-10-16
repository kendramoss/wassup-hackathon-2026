package llm

import (
	"math"
	"testing"
)

func TestNormalize(t *testing.T) {
	if v := Normalize([]float32{3, 4}); math.Abs(float64(v[0])-0.6) > 1e-6 {
		t.Errorf("Normalize = %v", v)
	}
	if v := Normalize([]float32{0, 0}); v[0] != 0 {
		t.Error("zero vector should stay zero")
	}
}
