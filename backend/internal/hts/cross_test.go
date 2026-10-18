package hts

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/cross"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// fakeCROSS answers a search with hits whose subject contains every word of
// the query, as CROSS does.
type fakeCROSS struct {
	hits    []cross.SearchHit
	queries []string
	err     error
}

func (f *fakeCROSS) Search(_ context.Context, term string, _ time.Time, n int) ([]cross.SearchHit, error) {
	f.queries = append(f.queries, term)
	if f.err != nil {
		return nil, f.err
	}
	var out []cross.SearchHit
	for _, h := range f.hits {
		ok := true
		for _, w := range strings.Fields(term) {
			ok = ok && strings.Contains(strings.ToLower(h.Subject), w)
		}
		if ok && len(out) < n {
			out = append(out, h)
		}
	}
	return out, nil
}

func hit(id, date, subject, tariffs, category string) cross.SearchHit {
	return cross.SearchHit{RulingNumber: id, RulingDate: date + "T00:00:00", Subject: subject, Tariffs: tariffs, Categories: category}
}

var heatGunCROSS = []cross.SearchHit{
	hit("N328954", "2022-11-15", "The country of origin of a heat gun", "", "Origin"),
	hit("N289152", "2017-08-31", "The tariff classification of a heat gun", "8419.89.9585", "Classification"),
	hit("N014907", "2007-08-22", "The tariff classification of a heat gun from China", "8419.89.9585", "Classification"),
	hit("H317418", "2025-05-13", "Classification of certain models of heat guns", "8419.89.95", "Classification"),
	hit("960936", "1998-01-29", "Pistol shaped hot air guns, heat gun", "8419.89.90", "Classification"), // number since renumbered
	hit("952881", "1993-02-11", "Heat gun; subheading 8516.32.00", "8516.79.00", "Classification"),
	hit("N111111", "2020-01-01", "Party heat gun toy set", "9503.00.00, 9903.88.15", "Classification"),
}

func TestCrossCandidatesRanksClassificationRulings(t *testing.T) {
	f := &fakeCROSS{hits: heatGunCROSS}
	current := func(_ context.Context, sub string) bool { return sub != "8419.89.90" }
	cs, err := crossCandidates(context.Background(), f, current, "The merchandise is a heat gun with a heating element and fan.", "Kobalt 12.5-amp heat gun")
	if err != nil {
		t.Fatal(err)
	}
	var codes []string
	for _, c := range cs {
		codes = append(codes, c.HTS)
	}
	// 8419.89.95 has three classification rulings; the origin ruling has no
	// tariff; 8419.89.90 is not in today's HTS; 9903 is an additional duty.
	if len(codes) == 0 || codes[0] != "8419.89.95" || strings.Contains(strings.Join(codes, ","), "8419.89.90") || strings.Contains(strings.Join(codes, ","), "9903") {
		t.Errorf("candidates = %v", codes)
	}
	if len(cs[0].Supporting) != 3 || cs[0].Supporting[0].ID != "H317418" {
		t.Errorf("8419.89.95 support = %+v (want 3, newest first)", cs[0].Supporting)
	}
	if f.queries[0] != "heat gun" {
		t.Errorf("first query = %q, want the product name's words", f.queries[0])
	}
}

func TestCrossQueriesSkipTheRequestLetter(t *testing.T) {
	q := crossQueries("In your letter dated February 13, 2025, you requested a tariff classification ruling. In lieu of samples, literature was provided. The Saphira gaming chair is a swivel seat with a gas lift.", "")
	if len(q) == 0 || !strings.HasPrefix(q[0], "saphira gaming chair") {
		t.Errorf("queries = %q", q)
	}
}

func TestSubheadings(t *testing.T) {
	got := subheadings("8516.80.8000, 8516.80.8000; 9903.88.15, 9802.00.50, 8419.89.95")
	if strings.Join(got, ",") != "8516.80.80,8419.89.95" {
		t.Errorf("subheadings = %v", got)
	}
}

func TestSuggestMixesCROSSAndIndexAndFallsBack(t *testing.T) {
	m := &llm.Fake{Respond: func(r llm.Request) (llm.Response, error) {
		// rank the CROSS candidate first
		return llm.Response{Text: `{"candidates":[{"hts":"8419.89.95","reason":"heat guns","cited":["H317418"]}]}`}, nil
	}}
	s := newSuggester(t, m)
	s.WithCROSS(&fakeCROSS{hits: heatGunCROSS}, nil)
	res, err := s.SuggestNamed(context.Background(), "heat gun heating element fan", "heat gun")
	if err != nil || res.Source != SourceCROSS || res.Candidates[0].HTS != "8419.89.95" || res.Candidates[0].Reason != "heat guns" || len(res.Candidates) > MaxCandidates {
		t.Errorf("cross: %+v %v", res, err)
	}
	// CROSS down: the local index answers.
	s.WithCROSS(&fakeCROSS{err: errors.New("cross: HTTP 503")}, nil)
	res, _ = s.SuggestNamed(context.Background(), "heat gun heating element fan", "heat gun")
	if res.Source != SourceIndex || len(res.Candidates) == 0 {
		t.Errorf("fallback: %+v", res)
	}
}

func TestInterleaveTakesFromBoth(t *testing.T) {
	a := []Candidate{{HTS: "1"}, {HTS: "2"}, {HTS: "3"}, {HTS: "4"}}
	b := []Candidate{{HTS: "2"}, {HTS: "9"}, {HTS: "8"}}
	var got []string
	for _, c := range interleave(a, b, 5) {
		got = append(got, c.HTS)
	}
	if strings.Join(got, ",") != "1,2,9,3,8" {
		t.Errorf("interleave = %v", got)
	}
}
