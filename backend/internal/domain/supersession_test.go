package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func day(s string) time.Time {
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return d
}

// N345891 (11 March 2025) was revoked by H346161 on 5 May 2025 (D320).
func TestSupersededAsOfRespectsTheCaseDate(t *testing.T) {
	cases := []struct {
		at   string
		want bool
	}{
		{"2025-05-04", false}, // still CBP's position: usable in a backtest
		{"2025-05-05", false}, // the revocation's own date: still in force before it was decided (D370)
		{"2025-05-06", true},  // the day after
		{"2026-01-01", true},
	}
	for _, tc := range cases {
		s, ok := SupersededAsOf("N345891", day(tc.at))
		if ok != tc.want {
			t.Errorf("at %s: superseded=%v, want %v", tc.at, ok, tc.want)
		}
		if ok && (s.By != "H346161" || s.Kind != "revoked" || s.Note() != "revoked by H346161 on 2025-05-05") {
			t.Errorf("at %s: %+v", tc.at, s)
		}
	}
	if _, ok := SupersededAsOf("N345891", time.Time{}); !ok {
		t.Error("a zero date means today: any known supersession counts")
	}
	if _, ok := SupersededAsOf("N357771", time.Time{}); ok {
		t.Error("N357771 was never revoked")
	}
}

func TestUsableAsPrecedentForSkipsSupersededRulings(t *testing.T) {
	r := Ruling{ID: "N345891", Date: day("2025-03-11")}
	if !r.UsableAsPrecedentFor("case", day("2025-05-04")) {
		t.Error("a ruling revoked after the case date must stay usable")
	}
	if !r.UsableAsPrecedentFor("case", day("2025-05-05")) {
		t.Error("a ruling revoked on the case date must stay usable (D370)")
	}
	if r.UsableAsPrecedentFor("case", day("2025-05-06")) || r.UsableAsPrecedentFor("case", day("2025-09-01")) {
		t.Error("a ruling revoked before the case date was usable")
	}
	plain := Ruling{ID: "N350001", Date: day("2025-03-11")}
	if !plain.UsableAsPrecedentFor("case", day("2025-09-01")) || plain.UsableAsPrecedentFor("N350001", day("2025-09-01")) {
		t.Error("date and self rules changed")
	}
}

// The earliest edge wins: N299096 was modified twice.
func TestParseSupersessionKeepsTheEarliestEdge(t *testing.T) {
	at, by, err := ParseSupersession([]byte(`{"edges":[
		{"ruling":"N1","superseded_by":"H2","date":"2018-11-06","kind":"modified"},
		{"ruling":"N1","superseded_by":"H1","date":"2018-09-13","kind":"modified"}]}`))
	if err != nil || by["N1"].By != "H1" || !at["N1"].Equal(day("2018-09-13")) {
		t.Fatalf("%v %v %v", at, by, err)
	}
	if _, _, err := ParseSupersession([]byte(`{"edges":[{"ruling":"N1","superseded_by":"H1","date":"2018-09-13","kind":"affirmed"}]}`)); err == nil {
		t.Error("an unknown kind was accepted")
	}
}

func TestEmbeddedSupersessionMapIsConsistent(t *testing.T) {
	var m SupersessionMap
	if err := json.Unmarshal(supersessionJSON, &m); err != nil {
		t.Fatal(err)
	}
	if m.Version == "" || m.Rule == "" || m.Counts["edges"] != len(m.Edges) || len(m.Edges) == 0 {
		t.Fatalf("version %q, counts %v, %d edges", m.Version, m.Counts, len(m.Edges))
	}
	for _, e := range m.Edges {
		if e.Quote == "" || e.SourceRuling == "" || !ValidRulingID(e.By) {
			t.Errorf("edge without provenance: %+v", e)
		}
	}
}

// #527, D370: when the case being backtested is itself the revoking ruling,
// its own change must not hide the ruling it revokes. H325607 (29 August
// 2025) revoked N331974; the guard starts the day after.
func TestACaseThatIsTheRevocationStillSeesTheRulingItRevokes(t *testing.T) {
	r := Ruling{ID: "N331974", Date: day("2023-03-01")}
	if s, ok := SupersededAsOf("N331974", time.Time{}); !ok || s.By != "H325607" || s.Date != "2025-08-29" {
		t.Fatalf("edge changed: %+v %v", s, ok)
	}
	if !r.UsableAsPrecedentFor("H325607", day("2025-08-29")) {
		t.Error("N331974 must be usable for case H325607 dated 2025-08-29")
	}
	if r.UsableAsPrecedentFor("other", day("2025-08-30")) {
		t.Error("N331974 must not be usable for a case dated 2025-08-30")
	}
	// Every ruling's earliest edge: usable on the revoking ruling's date,
	// not the day after.
	var m SupersessionMap
	if err := json.Unmarshal(supersessionJSON, &m); err != nil {
		t.Fatal(err)
	}
	for _, e := range m.Edges {
		if supersededBy[e.Ruling].By != e.By {
			continue // a later edge for a ruling already changed
		}
		d := day(e.Date)
		if _, ok := SupersededAsOf(e.Ruling, d); ok {
			t.Errorf("%s: hidden on its revocation date %s", e.Ruling, e.Date)
		}
		if _, ok := SupersededAsOf(e.Ruling, d.AddDate(0, 0, 1)); !ok {
			t.Errorf("%s: not hidden the day after %s", e.Ruling, e.Date)
		}
	}
}
