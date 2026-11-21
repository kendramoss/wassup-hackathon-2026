package tariffshift

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func mustDefault(t *testing.T) *Rules {
	t.Helper()
	r, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func rule(t *testing.T, hts string) Rule {
	t.Helper()
	for _, r := range mustDefault(t).Table().Rules {
		if r.HTS == hts {
			return r
		}
	}
	t.Fatalf("no rule %s", hts)
	return Rule{}
}

// The committed alternatives are exactly what the parser makes of the
// committed verbatim text, so rules.json cannot drift from the code.
func TestRulesJSONMatchesParser(t *testing.T) {
	tab := mustDefault(t).Table()
	re, err := Reparse(tab)
	if err != nil {
		t.Fatal(err)
	}
	for i := range re.Rules {
		if !reflect.DeepEqual(re.Rules[i], tab.Rules[i]) {
			a, _ := json.Marshal(re.Rules[i])
			b, _ := json.Marshal(tab.Rules[i])
			t.Fatalf("rule %s: parser gives\n%s\ncommitted\n%s\n(regenerate: go run ./cmd/tariffshift rules -reparse)", tab.Rules[i].HTS, a, b)
		}
	}
}

func TestTableShape(t *testing.T) {
	tab := mustDefault(t).Table()
	if !slices.Equal(tab.Chapters, ScopeChapters) {
		t.Fatalf("chapters %v, want %v", tab.Chapters, ScopeChapters)
	}
	if !strings.HasPrefix(tab.Source.URL, "https://www.ecfr.gov/") {
		t.Fatalf("source %q", tab.Source.URL)
	}
	perChapter := map[string]int{}
	for i, r := range tab.Rules {
		got, err := ParseRange(r.HTS)
		if err != nil || got != r.Applies {
			t.Fatalf("rule %s: applies %v, parsed %v (%v)", r.HTS, r.Applies, got, err)
		}
		perChapter[r.HTS[:2]]++
		if r.Text == "" || len(r.Alternatives) == 0 {
			t.Fatalf("rule %s has no text", r.HTS)
		}
		wantID := "19 CFR 102.20(" + r.Paragraph + "), " + r.HTS
		if r.ID != wantID {
			t.Fatalf("id %q, want %q", r.ID, wantID)
		}
		// Rows do not overlap.
		for _, o := range tab.Rules[i+1:] {
			if o.Applies.Level() == r.Applies.Level() && o.Applies.Lo <= r.Applies.Hi && r.Applies.Lo <= o.Applies.Hi {
				t.Fatalf("rules %s and %s overlap", r.HTS, o.HTS)
			}
		}
	}
	// Row counts read off the eCFR table by hand (as of 18 Nov 2026).
	for ch, want := range map[string]int{"82": 5, "94": 10} {
		if perChapter[ch] != want {
			t.Errorf("chapter %s: %d rules, want %d", ch, perChapter[ch], want)
		}
	}
	for _, ch := range ScopeChapters {
		if perChapter[ch] == 0 {
			t.Errorf("chapter %s has no rules", ch)
		}
	}
}

// Hand-checked encodings against the eCFR text of each row.
func TestHandCheckedRules(t *testing.T) {
	r8473 := rule(t, "8473")
	if r8473.Text != "A change to heading 8473 from any other heading, except from heading 8414, 8501, 8504, 8534, 8541, or 8542 when resulting from a simple assembly." {
		t.Fatalf("8473 text %q", r8473.Text)
	}
	if r8473.ID != "19 CFR 102.20(o), 8473" {
		t.Fatalf("8473 id %q", r8473.ID)
	}
	want := Alternative{
		Text:    "A change to heading 8473 from any other heading, except from heading 8414, 8501, 8504, 8534, 8541, or 8542 when resulting from a simple assembly",
		Encoded: true, To: []Range{{"8473", "8473"}}, From: 4,
		Except: []Exception{{
			From:      []Range{{"8414", "8414"}, {"8501", "8501"}, {"8504", "8504"}, {"8534", "8534"}, {"8541", "8541"}, {"8542", "8542"}},
			Condition: CondSimpleAssembly, Text: "heading 8414, 8501, 8504, 8534, 8541, or 8542",
		}},
	}
	if got := r8473.Alternatives; len(got) != 1 || !reflect.DeepEqual(got[0], want) {
		t.Fatalf("8473 parsed %+v", got)
	}

	a := rule(t, "8402.90").Alternatives[0]
	if a.From != 4 || len(a.Except) != 1 || a.Except[0].Condition != CondUnlessBending || len(a.Except[0].From) != 4 {
		t.Fatalf("8402.90 %+v", a)
	}

	g := rule(t, "7321.11-7321.89").Alternatives
	if len(g) != 2 || g[0].From != 4 || !g[1].NotGRI2a || g[1].From != 0 ||
		!reflect.DeepEqual(g[1].Also[0].From, []Range{{"732190", "732190"}}) {
		t.Fatalf("7321.11-7321.89 %+v", g)
	}

	b := rule(t, "8507.10-8507.80").Alternatives[0]
	if b.Group != "including" || !reflect.DeepEqual(b.Except[0].To, []Range{{"850780", "850780"}}) ||
		!reflect.DeepEqual(b.Except[0].From, []Range{{"850750", "850750"}, {"850760", "850760"}}) {
		t.Fatalf("8507 %+v", b)
	}

	p := rule(t, "8504.10-8504.50").Alternatives[0]
	if p.From != 6 || p.Group != "outside" || p.GroupImplied {
		t.Fatalf("8504.10-8504.50 %+v", p)
	}

	c := rule(t, "8544.11-8544.70").Alternatives
	if len(c) != 3 || !c[2].NotSimpleAssembly || c[2].Group != "including" || !c[0].Also[0].Described {
		t.Fatalf("8544 %+v", c)
	}

	l := rule(t, "9405.11-9405.69").Alternatives[0]
	if l.Group != "outside" || l.Except[0].Condition != CondGRI2a || !reflect.DeepEqual(l.Except[0].From, []Range{{"940591", "940599"}}) {
		t.Fatalf("9405 %+v", l)
	}

	// The paragraph (p) note bars changes to heading 8708 from four
	// subheadings; it becomes an exception on every encoded 8708 rule.
	v := rule(t, "8708.10")
	if len(v.Notes) != 1 || len(v.Alternatives[0].Except) != 1 ||
		!reflect.DeepEqual(v.Alternatives[0].Except[0].From, []Range{{"870990", "870990"}, {"871690", "871690"}, {"843120", "843120"}, {"843149", "843149"}}) {
		t.Fatalf("8708.10 %+v", v)
	}

	// A sentence by goods description is not encoded but keeps its codes.
	d := rule(t, "8470.10-8471.50").Alternatives
	if d[0].Encoded || !reflect.DeepEqual(d[0].To, []Range{{"847090", "847090"}}) || !d[4].Encoded || d[4].Group != "within" {
		t.Fatalf("8470.10-8471.50 %+v", d)
	}

	// The chapter 94 bedding note replaces 102.11(b).
	if !rule(t, "9404.30-9404.90").FallbackNote {
		t.Fatal("9404.30-9404.90 should carry the fallback note")
	}
}

func TestParseList(t *testing.T) {
	for _, c := range []struct {
		in        string
		want      []Range
		described bool
	}{
		{"heading 7303, 7304, 7305, or 7306", []Range{{"7303", "7303"}, {"7304", "7304"}, {"7305", "7305"}, {"7306", "7306"}}, false},
		{"subheading 8486.10 through 8486.40", []Range{{"848610", "848640"}}, false},
		{"chapter 16", []Range{{"16", "16"}}, false},
		{"subheading 8504.40 or from heading 8473", []Range{{"850440", "850440"}, {"8473", "8473"}}, false},
		{"parts of water-jet cutting machines of heading 8466", []Range{{"8466", "8466"}}, true},
	} {
		got, d, err := parseList(c.in)
		if err != nil || d != c.described || !reflect.DeepEqual(got, c.want) {
			t.Errorf("parseList(%q) = %v %v %v", c.in, got, d, err)
		}
	}
	if _, _, err := parseList("16"); err == nil {
		t.Error("a bare number without a level must not parse")
	}
}

func TestRangeContains(t *testing.T) {
	r := Range{"847010", "847150"}
	for code, want := range map[string]tri{"847130": yes, "8471300100": yes, "847160": no, "8471": maybe, "8470": maybe, "8472": no, "84": maybe} {
		if got := r.contains(code); got != want {
			t.Errorf("%v contains %s = %v, want %v", r, code, got, want)
		}
	}
}
