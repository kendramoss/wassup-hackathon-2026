package ukorigin_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/euorigin"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/nporigin"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/ukorigin"
)

var (
	rePunct  = regexp.MustCompile(`\s+([,;.])`)
	reSpaces = regexp.MustCompile(`\s+`)
	// The UK writes "Goods Classification Table heading ex 04 07" where the
	// EU writes "HS heading ex 0407".
	reUKCode = regexp.MustCompile(`\b(\d{2}) (\d{2})\b`)
)

func norm(s string) string {
	s = strings.ReplaceAll(s, "Goods Classification Table", "HS")
	s = reUKCode.ReplaceAllString(s, "$1$2")
	s = rePunct.ReplaceAllString(s, "$1")
	return strings.TrimSuffix(strings.TrimSpace(reSpaces.ReplaceAllString(s, " ")), ".")
}

func f(v float64) *float64 { return &v }

// TestParity: where the EU and UK row text is the same, both checks give
// the same answer (D765). Rows whose answer turns on a difference in the
// framing (the UK's definition of value in a max-value rule) or in the
// chapter's residual rules are left out and counted.
func TestParity(t *testing.T) {
	eu, err := euorigin.Rules()
	if err != nil {
		t.Fatal(err)
	}
	uk, err := ukorigin.Rules()
	if err != nil {
		t.Fatal(err)
	}
	type rowIn struct {
		ch  nporigin.Chapter
		row nporigin.Row
	}
	index := func(r *nporigin.Rules) map[string]rowIn {
		m := map[string]rowIn{}
		for _, ch := range r.Table().Chapters {
			for _, row := range ch.Rows {
				m[row.Key] = rowIn{ch, row}
			}
		}
		return m
	}
	E, U := index(eu), index(uk)
	sameChapter := func(a, b nporigin.Chapter) bool {
		return a.Residual == b.Residual && (a.Mixture == "") == (b.Mixture == "") &&
			strings.Contains(a.Mixture, "dry matter") == strings.Contains(b.Mixture, "dry matter") &&
			(a.Fallback == "") == (b.Fallback == "") && len(a.ChapterRules) == len(b.ChapterRules)
	}
	compared, skipped := 0, 0
	for key, e := range E {
		u, ok := U[key]
		if !ok || norm(e.row.Text) != norm(u.row.Text) {
			continue
		}
		maxValue := false
		for _, a := range e.row.Alts {
			maxValue = maxValue || a.Kind == nporigin.KindMaxValue
		}
		if maxValue || !sameChapter(e.ch, u.ch) {
			skipped++
			continue
		}
		compared++
		good := e.row.Applies[0].Lo
		if len(good) == 4 {
			good += "00"
		}
		mats := [][]nporigin.Material{
			{{Name: "other chapter", HTS: "3926.90", Origin: "China", ValueShare: f(0.6), WeightShare: f(0.6)}, {Name: "local", HTS: "4819.10", Origin: "Vietnam", ValueShare: f(0.3), WeightShare: f(0.3)}},
			{{Name: "same heading", HTS: good[:4] + ".99", Origin: "China", ValueShare: f(0.6), WeightShare: f(0.6)}, {Name: "local", HTS: "4819.10", Origin: "Vietnam", ValueShare: f(0.3), WeightShare: f(0.3)}},
		}
		for mi, ms := range mats {
			for _, ans := range []bool{true, false} {
				answers := map[string]bool{}
				for _, a := range e.row.Alts {
					answers[a.Key], answers[a.Key+".c"] = ans, ans
					for i := range a.Except {
						answers[fmt.Sprintf("%s.x%d", a.Key, i+1)] = ans
					}
				}
				answers["ch"+e.ch.Chapter+".r"], answers["ch"+e.ch.Chapter+".mix"] = ans, ans
				in := nporigin.Input{GoodHTS: good, Country: "Vietnam", Row: key, Materials: ms, Answers: answers}
				re, ru := eu.Evaluate(in), uk.Evaluate(in)
				if re.Status != ru.Status || re.Origin != ru.Origin {
					t.Errorf("%s (materials %d, answers %v): EU %s %q, UK %s %q", key, mi, ans, re.Status, re.Origin, ru.Status, ru.Origin)
				}
			}
		}
	}
	// About 440 shared rows have the same text; the value rules are skipped.
	if compared != 441 || skipped != 3 {
		t.Errorf("compared %d rows, skipped %d; want 441 and 3 (D765)", compared, skipped)
	}
	t.Logf("parity: %d rows compared, %d skipped", compared, skipped)
}

// TestFramingDiffers: the cases where the EU and UK texts frame the same row
// differently give different answers, on purpose (D764).
func TestFramingDiffers(t *testing.T) {
	eu, err := euorigin.Rules()
	if err != nil {
		t.Fatal(err)
	}
	uk, err := ukorigin.Rules()
	if err != nil {
		t.Fatal(err)
	}
	yes := true
	mats := []nporigin.Material{{Name: "board", HTS: "8473.30", Origin: "China", ValueShare: f(0.8)}, {Name: "box", HTS: "4819.10", Origin: "Vietnam", ValueShare: f(0.1)}}
	for _, c := range []struct {
		name             string
		in               nporigin.Input
		euStatus, euOrig string
		ukStatus         string
	}{
		{"minimal operations, unlisted good: Art. 34 DA in the EU, a judgement in the UK",
			nporigin.Input{GoodHTS: "8471.30", Country: "Vietnam", MinimalOnly: &yes, Materials: mats}, nporigin.Decided, "China", nporigin.NeedsJudgement},
		{"minimal operations, listed good: EU residual rule, UK judgement",
			nporigin.Input{GoodHTS: "8527.13", Country: "Vietnam", MinimalOnly: &yes, Materials: mats}, nporigin.Decided, "China", nporigin.NeedsJudgement},
		{"value of the materials used: a question in the EU, decided in the UK",
			nporigin.Input{GoodHTS: "6307.20", Country: "Turkey", Materials: []nporigin.Material{{Name: "fabric", HTS: "5903.20", Origin: "India", ValueShare: f(0.35)}}}, nporigin.Undetermined, "", nporigin.Decided},
	} {
		re, ru := eu.Evaluate(c.in), uk.Evaluate(c.in)
		if re.Status != c.euStatus || re.Origin != c.euOrig || ru.Status != c.ukStatus {
			t.Errorf("%s: EU %s %q, UK %s %q", c.name, re.Status, re.Origin, ru.Status, ru.Origin)
		}
	}
}
