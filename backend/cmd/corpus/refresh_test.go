package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLastProbed(t *testing.T) {
	rows := []probeRow{
		{Ruling: "N366800", Found: true, Date: "2026-11-13"},
		{Ruling: "N366900", Found: false},
		{Ruling: "H362333", Found: true, Date: "2026-10-01"},
		{Ruling: "N366799", Found: true, Date: "2026-11-14"},
		{Ruling: "N367000", Error: "timeout"},
	}
	n, h, d := lastProbed(rows)
	if n != 366800 || h != 362333 || d != "2026-11-14" {
		t.Fatalf("got %d %d %s", n, h, d)
	}
}

func TestCheckSaved(t *testing.T) {
	dir := t.TempDir()
	write := func(id string, v any) {
		b, _ := json.Marshal(v)
		if err := os.WriteFile(filepath.Join(dir, id+".json"), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	text := "Dear Mr. Smith: This is in response to your letter requesting a country of origin determination. FACTS: The product is a widget assembled in Mexico from Chinese parts. ISSUE: What is the country of origin? LAW AND ANALYSIS: ..."
	write("N1", map[string]string{"rulingNumber": "N1", "rulingDate": "2026-11-20T00:00:00", "text": text, "subject": "Widget"})
	write("N2", map[string]string{"rulingNumber": "N2", "rulingDate": "", "text": text})
	os.WriteFile(filepath.Join(dir, "N3.json"), []byte("not json"), 0o644)
	write("N5", map[string]string{"rulingNumber": "N5", "rulingDate": "2026-11-20T00:00:00", "text": text})
	rows := []probeRow{
		{Ruling: "N1", Saved: true, Date: "2026-11-20"},
		{Ruling: "N2", Saved: true},
		{Ruling: "N3", Saved: true},
		{Ruling: "N4", Saved: true},
		{Ruling: "N5", Saved: true},
		{Ruling: "N6", Found: true},
	}
	items := checkSaved(rows, dir, map[string]bool{"N5": true})
	got := map[string]refreshItem{}
	for _, it := range items {
		got[it.Ruling] = it
	}
	if len(items) != 5 || !got["N1"].Valid || got["N1"].Subject != "Widget" {
		t.Fatalf("N1 should pass: %+v", items)
	}
	for _, id := range []string{"N2", "N3", "N4", "N5"} {
		if got[id].Valid || len(got[id].Reasons) == 0 {
			t.Errorf("%s should fail with a reason: %+v", id, got[id])
		}
	}
	var rep refreshReport
	summarise(&rep, rows, items)
	if rep.Tried != 6 || rep.Saved != 5 || rep.Valid != 1 || rep.DateMin != "2026-11-20" || rep.DateMax != "2026-11-20" {
		t.Fatalf("summary %+v", rep)
	}
}
