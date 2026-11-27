package adcvd

import (
	"errors"
	"strings"
	"testing"
)

type fakeReader struct {
	verdict Verdict
	err     error
	asked   int
}

func (f *fakeReader) Read(Part, Order) (Verdict, error) {
	f.asked++
	return f.verdict, f.err
}

func refineTable() *Table {
	return NewTable("test-v1", "2026-11-27", []Order{
		{Case: "A-570-082", Type: "AD", Country: "China", Product: "Certain Steel Wheels", Status: "active",
			HTS:       []string{"8716.90.5035", "8708.70.4530"},
			ScopeText: "The merchandise covered is steel wheels, 12 to 16.5 inches in diameter, for trailers. Excluded from the scope are wheels made of aluminum and automobile tires."},
		{Case: "C-570-083", Type: "CVD", Country: "China", Product: "Certain Steel Wheels", Status: "active",
			HTS:       []string{"8716.90.5035"},
			ScopeText: "The merchandise covered is steel wheels, 12 to 16.5 inches in diameter, for trailers. Excluded from the scope are wheels made of aluminum and automobile tires."},
		{Case: "A-570-106", Type: "AD", Country: "China", Product: "Wooden Cabinets and Vanities", Status: "active",
			HTS: []string{"9403"}, ScopeText: "Wooden cabinets and vanities for kitchens and bathrooms."},
		{Case: "A-570-777", Type: "AD", Country: "China", Product: "Thing without scope text", Status: "active", HTS: []string{"7326.90.8688"}},
	})
}

func status(t *testing.T, tb *Table, p Part, cfg Config) PartResult {
	t.Helper()
	return tb.CheckWith(Input{Parts: []Part{p}}, cfg).Parts[0]
}

func TestZeroConfigIsTheBaselineCheck(t *testing.T) {
	tb := refineTable()
	in := Input{Parts: []Part{{Name: "aluminum trailer wheel", HTS: "8716.90.5035", Origin: "China"}, {Name: "cabinet", HTS: "9403.60.8081", Origin: "China"}, {Name: "", HTS: "7326.90.8688", Origin: "China"}}}
	a, b := tb.Check(in), tb.CheckWith(in, Config{})
	if len(a.Parts) != len(b.Parts) {
		t.Fatal("different lengths")
	}
	for i := range a.Parts {
		if a.Parts[i].Status != b.Parts[i].Status || len(a.Parts[i].Matches) != len(b.Parts[i].Matches) || a.Parts[i].Reason != b.Parts[i].Reason {
			t.Errorf("part %d differs between Check and CheckWith(Config{})", i)
		}
	}
}

func TestMinListedDigitsDropsHeadingLevelMatches(t *testing.T) {
	tb := refineTable()
	p := Part{Name: "wooden cabinet", HTS: "9403.60.8081", Origin: "China"}
	if got := status(t, tb, p, Config{}); got.Status != StatusPossible {
		t.Fatalf("baseline = %s, want possible", got.Status)
	}
	got := status(t, tb, p, Config{MinListedDigits: 6})
	if got.Status != StatusNone || len(got.Matches) != 0 {
		t.Fatalf("heading-level listing with MinListedDigits 6 = %s %v, want no_match", got.Status, got.Matches)
	}
	if !strings.Contains(got.Reason, "written scope") {
		t.Errorf("reason %q should say the written scope governs", got.Reason)
	}
	// a ten-digit listing agrees at ten digits and stays
	if got := status(t, tb, Part{Name: "trailer wheel", HTS: "8716.90.5035", Origin: "China"}, Config{MinListedDigits: 8}); got.Status != StatusPossible {
		t.Errorf("ten-digit listing dropped: %s", got.Status)
	}
}

func TestScopeTermsNeedAProductTermInCommon(t *testing.T) {
	tb := refineTable()
	cfg := Config{ScopeTerms: true}
	if got := status(t, tb, Part{Name: "Steel trailer wheels, model X", HTS: "8716.90.5035", Origin: "China"}, cfg); got.Status != StatusPossible {
		t.Errorf("a wheel against a wheel scope = %s, want possible", got.Status)
	}
	if got := status(t, tb, Part{Name: "Garlic cloves", HTS: "8716.90.5035", Origin: "China"}, cfg); got.Status != StatusNone {
		t.Errorf("garlic against a wheel scope = %s, want no_match", got.Status)
	}
}

func TestRefinementsNeverRemoveWhatTheyCannotRead(t *testing.T) {
	tb := refineTable()
	all := Config{TitleTerms: true, ScopeTerms: true, Exclusions: true, Reader: &fakeReader{verdict: VerdictOut}, ReaderLean: true}
	// no part name: nothing to read against the scope
	if got := status(t, tb, Part{HTS: "7326.90.8688", Origin: "China"}, all); got.Status != StatusPossible {
		t.Errorf("part without a name = %s, want possible", got.Status)
	}
	// an order whose notice carried no scope text
	r := &fakeReader{verdict: VerdictOut}
	if got := status(t, tb, Part{Name: "steel bracket", HTS: "7326.90.8688", Origin: "China"}, Config{ScopeTerms: true, Reader: r}); got.Status != StatusPossible || r.asked != 0 {
		t.Errorf("order without scope text = %s, reader asked %d times, want possible and 0", got.Status, r.asked)
	}
}

func TestExclusionSentenceNamingTheProduct(t *testing.T) {
	tb := refineTable()
	cfg := Config{Exclusions: true}
	if got := status(t, tb, Part{Name: "Aluminum wheels for trailers", HTS: "8716.90.5035", Origin: "China"}, cfg); got.Status != StatusNone {
		t.Errorf("aluminum wheel named in an exclusion = %s, want no_match", got.Status)
	}
	if got := status(t, tb, Part{Name: "Steel trailer wheels", HTS: "8716.90.5035", Origin: "China"}, cfg); got.Status != StatusPossible {
		t.Errorf("steel wheel = %s, want possible", got.Status)
	}
	// one shared word is not enough
	if got := status(t, tb, Part{Name: "Tires for aircraft", HTS: "8716.90.5035", Origin: "China"}, cfg); got.Status != StatusPossible {
		t.Errorf("one shared term removed a match: %s", got.Status)
	}
}

func TestReaderOnlyOutRemoves(t *testing.T) {
	tb := refineTable()
	p := Part{Name: "steel trailer wheels", HTS: "8716.90.5035", Origin: "China"}
	for v, want := range map[Verdict]string{VerdictIn: StatusPossible, VerdictUnclear: StatusPossible, VerdictUnclearIn: StatusPossible, VerdictUnclearOut: StatusPossible, VerdictOut: StatusNone, "": StatusPossible} {
		if got := status(t, tb, p, Config{Reader: &fakeReader{verdict: v}}); got.Status != want {
			t.Errorf("verdict %q = %s, want %s", v, got.Status, want)
		}
	}
	if got := status(t, tb, p, Config{Reader: &fakeReader{verdict: VerdictUnclearOut}, ReaderLean: true}); got.Status != StatusNone {
		t.Errorf("unclear_out with ReaderLean = %s, want no_match", got.Status)
	}
	if got := status(t, tb, p, Config{Reader: &fakeReader{err: errors.New("down")}}); got.Status != StatusPossible {
		t.Errorf("reader error removed a match: %s", got.Status)
	}
}

func TestReaderAskedOncePerOrderGroup(t *testing.T) {
	tb := refineTable()
	r := &fakeReader{verdict: VerdictIn}
	got := status(t, tb, Part{Name: "steel trailer wheels", HTS: "8716.90.5035", Origin: "China"}, Config{Reader: r})
	if len(got.Matches) != 1 || len(got.Matches[0].Cases) != 2 {
		t.Fatalf("matches = %+v, want one group of the AD and the CVD order", got.Matches)
	}
	if r.asked != 1 {
		t.Errorf("reader asked %d times for the AD/CVD pair, want 1", r.asked)
	}
}

func TestTermsDropGenericWordsAndPlurals(t *testing.T) {
	a := Terms("Certain Steel Wheels from China")
	if !a["steel"] || !a["wheel"] || a["certain"] || a["china"] || a["from"] {
		t.Errorf("Terms = %v", a)
	}
	if b := Terms("Model X-45282 wheel"); !b["wheel"] || len(b) != 1 {
		t.Errorf("model numbers are not terms: %v", b)
	}
	if !Terms("batteries")["battery"] || !Terms("glasses")["glass"] || !Terms("buses")["bus"] {
		t.Errorf("plural endings: %v %v %v", Terms("batteries"), Terms("glasses"), Terms("buses"))
	}
}
