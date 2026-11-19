package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// A watch saved before the provenance fields existed still loads, and writes
// back without them; one with them round-trips through every store that can
// run offline.
func TestWatchProvenanceRoundTrip(t *testing.T) {
	var legacy Watch
	old := `{"checked_at":"2026-05-01T00:00:00Z","status":"unchanged","prediction":{"origin_country":"China"},"newer_rulings":[]}`
	if err := json.Unmarshal([]byte(old), &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.Basis != "" || legacy.Triage != nil || legacy.Regime != "" {
		t.Errorf("legacy watch got provenance: %+v", legacy)
	}
	b, _ := json.Marshal(legacy)
	for _, k := range []string{"basis", "regime", "original_regime", "scope", "specialist", "triage"} {
		if strings.Contains(string(b), `"`+k+`"`) {
			t.Errorf("legacy watch wrote %q: %s", k, b)
		}
	}

	f, err := NewFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	full := &Watch{CheckedAt: time.Now().UTC().Truncate(time.Second), Status: WatchChanged, Newer: []string{}, Basis: WatchBasisRegime,
		Regime: "pharma_api", OriginalRegime: "substantial_transformation", Scope: "confirmed", Specialist: "ok",
		Triage: &domain.Triage{Route: domain.RouteNeedsReview, Reason: "r"}}
	for _, s := range []Cases{NewMemory(), f} {
		for id, w := range map[string]*Watch{"with": full, "without": &legacy} {
			if err := s.Put(ctx, Case{ID: id, Watch: w}); err != nil {
				t.Fatal(err)
			}
			got, err := s.Get(ctx, id)
			if err != nil || got.Watch == nil || got.Watch.Basis != w.Basis || got.Watch.Regime != w.Regime || got.Watch.OriginalRegime != w.OriginalRegime ||
				got.Watch.Scope != w.Scope || got.Watch.Specialist != w.Specialist || (got.Watch.Triage == nil) != (w.Triage == nil) {
				t.Errorf("%T %s: got %+v, %v", s, id, got.Watch, err)
			}
		}
	}
}
