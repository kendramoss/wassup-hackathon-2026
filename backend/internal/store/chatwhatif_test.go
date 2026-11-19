package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// An old record without provenance reads as unknown and not confirmed; a
// new one round-trips unchanged, through JSON and the file store.
func TestChatWhatIfProvenanceCompat(t *testing.T) {
	var old ChatWhatIf
	if err := json.Unmarshal([]byte(`{"change":"c","origin":"China","confidence":0.9,"decisive_step":"d"}`), &old); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(old)
	if !strings.Contains(string(b), `"regime":"unknown"`) || !strings.Contains(string(b), `"provisional":"`+LegacyProvisional+`"`) || strings.Contains(string(b), `"route"`) {
		t.Errorf("old record = %s", b)
	}
	n := ChatWhatIf{Origin: "India", Regime: "pharma_api", Specialist: "ok", Scope: "confirmed", Route: "auto", Reason: "r"}
	b, _ = json.Marshal(n)
	if strings.Contains(string(b), "provisional") || !strings.Contains(string(b), `"regime":"pharma_api"`) {
		t.Errorf("new confirmed record = %s", b)
	}
	f, err := NewFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p := ChatWhatIf{Origin: "China", Regime: "substantial_transformation", Specialist: "failed", Route: "needs_review", Provisional: "p"}
	if err := f.Put(context.Background(), Case{ID: "c1", Chat: []ChatTurn{{Question: "q", WhatIfs: []ChatWhatIf{p, old}}}}); err != nil {
		t.Fatal(err)
	}
	got, err := f.Get(context.Background(), "c1")
	if err != nil {
		t.Fatal(err)
	}
	w := got.Chat[0].WhatIfs
	if w[0] != p || w[1].Regime != "unknown" || w[1].Provisional != LegacyProvisional {
		t.Errorf("round trip = %+v", w)
	}
}
