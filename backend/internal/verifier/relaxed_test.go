package verifier

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestRelaxationsDropCountriesAndSplit(t *testing.T) {
	got := relaxations("electric motor stator rotor laminations SMT PCBA Thailand China")
	want := []string{
		"electric motor stator rotor laminations SMT PCBA Thailand China",
		"electric motor stator rotor laminations SMT PCBA",
		"electric motor stator rotor",
		"laminations SMT PCBA",
		"electric motor",
	}
	if !slices.Equal(got, want) {
		t.Errorf("relaxations = %q\nwant %q", got, want)
	}
	if got := relaxations("hair dryer OR blower motor heating element"); got[1] != "hair dryer blower motor heating element" {
		t.Errorf("OR should be dropped: %q", got)
	}
	if got := relaxations("heat gun"); len(got) != 1 {
		t.Errorf("a short query has nothing to relax: %q", got)
	}
}

// wordSearch returns hits only when every query word is in a ruling's
// subject, as CROSS does.
type wordSearch struct {
	subjects map[string]string
	queries  []string
	err      error
}

func (w *wordSearch) Search(_ context.Context, q string, _ time.Time, exclude map[string]bool) ([]Hit, error) {
	w.queries = append(w.queries, q)
	if w.err != nil {
		return nil, w.err
	}
	var out []Hit
	for _, id := range []string{"N1", "N2", "N3"} {
		ok := !exclude[id]
		for _, word := range strings.Fields(strings.ToLower(q)) {
			ok = ok && strings.Contains(strings.ToLower(w.subjects[id]), word)
		}
		if ok {
			out = append(out, Hit{ID: id, Subject: w.subjects[id], Origin: "?"})
		}
	}
	return out, nil
}

func (w *wordSearch) Read(context.Context, string, time.Time, map[string]bool) (string, error) {
	return "cross text", nil
}

type fixedSearch []Hit

func (f fixedSearch) Search(context.Context, string, time.Time, map[string]bool) ([]Hit, error) {
	return f, nil
}
func (fixedSearch) Read(context.Context, string, time.Time, map[string]bool) (string, error) {
	return "local text", nil
}

func TestRelaxedFindsWhatTheFullQueryMisses(t *testing.T) {
	cross := &wordSearch{subjects: map[string]string{"N1": "heat gun with motor", "N2": "electric motor stator", "N3": "hair dryer"}}
	r := Relaxed{Primary: cross, Fill: fixedSearch{{ID: "N2"}, {ID: "L9", Origin: "Thailand"}}}
	hits, err := r.Search(context.Background(), "electric motor stator rotor Thailand China", time.Now(), map[string]bool{"N1": true})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, h := range hits {
		ids = append(ids, h.ID)
	}
	// The full query finds nothing; "electric motor stator rotor" neither;
	// "electric motor" finds N2; the corpus adds L9 (N2 is not repeated).
	if strings.Join(ids, ",") != "N2,L9" || len(cross.queries) < 2 || cross.queries[0] != "electric motor stator rotor Thailand China" {
		t.Errorf("hits %v after queries %q", ids, cross.queries)
	}
	if txt, _ := r.Read(context.Background(), "N2", time.Now(), nil); txt != "cross text" {
		t.Errorf("Read must go to CROSS, got %q", txt)
	}
}

func TestRelaxedStopsWhenFullAndSurvivesCrossDown(t *testing.T) {
	var eight fixedSearch
	for i := range SearchHits {
		eight = append(eight, Hit{ID: string(rune('A'+i)) + "1"})
	}
	if hits, _ := (Relaxed{Primary: eight, Fill: fixedSearch{{ID: "L9"}}}).Search(context.Background(), "a b c d e", time.Now(), nil); len(hits) != SearchHits || hits[SearchHits-1].ID == "L9" {
		t.Errorf("a full first page needs no relaxing or fill: %v", hits)
	}
	down := &wordSearch{err: errors.New("cross: HTTP 503")}
	hits, err := Relaxed{Primary: down, Fill: fixedSearch{{ID: "L9"}}}.Search(context.Background(), "heat gun motor", time.Now(), nil)
	if err != nil || len(hits) != 1 || hits[0].ID != "L9" {
		t.Errorf("CROSS down: the corpus should still answer: %v %v", hits, err)
	}
	if _, err := (Relaxed{Primary: down}).Search(context.Background(), "heat gun motor", time.Now(), nil); err == nil {
		t.Error("CROSS down and no fill must be an error (the verdict is then not cached, D58)")
	}
}
