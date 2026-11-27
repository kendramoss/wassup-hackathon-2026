package adcvd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

func TestScopeFirstPromptsAreTheOnesMeasured(t *testing.T) {
	var got scopeFirstPrompts
	if err := json.Unmarshal(scopeFirstPromptsJSON, &got); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile("../../../eval/adcvd/scope-reader-prompts.json")
	if err != nil {
		t.Fatal(err)
	}
	var rd struct {
		Versions map[string]promptPair `json:"versions"`
	}
	if err := json.Unmarshal(b, &rd); err != nil {
		t.Fatal(err)
	}
	want := rd.Versions[ScopeReaderPromptVersion]
	if want.System == "" || got.Reader.System != want.System || got.Reader.User != want.User || got.Reader.Version != ScopeReaderPromptVersion {
		t.Errorf("the embedded reader prompt differs from %s in eval/adcvd/scope-reader-prompts.json", ScopeReaderPromptVersion)
	}
	b, err = os.ReadFile("../../../eval/adcvd/real/screen-prompts.json")
	if err != nil {
		t.Fatal(err)
	}
	var sc promptPair
	if err := json.Unmarshal(b, &sc); err != nil {
		t.Fatal(err)
	}
	if sc.System == "" || got.Screen.System != sc.System || got.Screen.User != sc.User || got.Screen.Version != ScreenPromptVersion || sc.Version != ScreenPromptVersion {
		t.Errorf("the embedded screen prompt differs from eval/adcvd/real/screen-prompts.json")
	}
}

// scriptedModel answers the screen with possible and the reader with reading, by the system prompt it is given.
type scriptedModel struct{ *llm.Fake }

func newScripted(possible, reading string, screenErr, readErr error) *scriptedModel {
	s := &scriptedModel{}
	s.Fake = &llm.Fake{Respond: func(req llm.Request) (llm.Response, error) {
		if strings.Contains(req.System, "screen which") {
			if screenErr != nil {
				return llm.Response{}, screenErr
			}
			return llm.Response{Text: possible}, nil
		}
		if readErr != nil {
			return llm.Response{}, readErr
		}
		return llm.Response{Text: reading}, nil
	}}
	return s
}

const (
	readIn         = `{"verdict":"in","lean":"in","quote":"steel wheels, in sizes from 12 to 16.5 inches","reason":"it is a steel wheel"}`
	readUnclearIn  = "```json\n" + `{"verdict":"unclear","lean":"in","quote":"finished or unfinished","reason":"material not stated"}` + "\n```"
	readUnclearOut = `{"verdict":"unclear","lean":"out","quote":"","reason":"size not stated"}`
	readOut        = `{"verdict":"out","lean":"out","quote":"Excluded from the scope are wheels made of aluminum","reason":"aluminum"}`
)

func wheelPart() Part {
	return Part{Name: "Steel wheel for a trailer, 15 inch", HTS: "8716.90.5035", Origin: "China"}
}

func checkOne(t *testing.T, m llm.Model, p Part) PartResult {
	t.Helper()
	sf, err := NewScopeFirst(m, "gemini-4-argon")
	if err != nil {
		t.Fatal(err)
	}
	return refineTable().CheckScopeFirst(Input{Parts: []Part{p}}, sf).Parts[0]
}

func TestScopeFirstFlagsInAndAsksWhatWasMeasured(t *testing.T) {
	m := newScripted(`{"possible":[1]}`, readIn, nil, nil)
	r := checkOne(t, m, wheelPart())
	if r.Status != StatusPossible || len(r.Matches) != 1 {
		t.Fatalf("result = %+v, want possible exposure with one match", r)
	}
	mt := r.Matches[0]
	if mt.Reading != "in" || !strings.Contains(mt.ScopeClause, "steel wheels") || strings.Join(mt.Cases, ",") != "A-570-082,C-570-083" || mt.Product != "Certain Steel Wheels" {
		t.Errorf("match = %+v", mt)
	}
	calls := m.Calls()
	if len(calls) != 2 {
		t.Fatalf("%d model calls, want one screen and one read", len(calls))
	}
	scr, rd := calls[0], calls[1]
	if scr.Model != "gemini-4-argon" || !strings.Contains(scr.Messages[0].Content, "1. Certain Steel Wheels (A-570-082 / C-570-083) [its listed HTS numbers include the product's code]\n   The merchandise covered by the order is steel wheels") || !strings.Contains(scr.Messages[0].Content, "Product (made in China): Steel wheel for a trailer, 15 inch\n\nOrders:\n") {
		t.Errorf("screen prompt: %q", scr.Messages[0].Content)
	}
	u := rd.Messages[0].Content
	if !strings.HasPrefix(u, "Order: Certain Steel Wheels from China\n\nWritten scope of the order:\nThe merchandise covered by the order is steel wheels") || !strings.HasSuffix(u, "\n\nProduct (made in China): Steel wheel for a trailer, 15 inch") {
		t.Errorf("reader prompt: %q", u)
	}
	if strings.Contains(scr.Messages[0].Content, "{") && strings.Contains(scr.Messages[0].Content, "{orders}") || strings.Contains(u, "{scope}") {
		t.Error("placeholders left in a prompt")
	}
}

func TestScopeFirstAnswersAreCachedAndNotRepeated(t *testing.T) {
	m := newScripted(`{"possible":["1"]}`, readIn, nil, nil)
	sf, _ := NewScopeFirst(m, "gemini-4-argon")
	tb := refineTable()
	for i := 0; i < 3; i++ {
		if r := tb.CheckScopeFirst(Input{Parts: []Part{wheelPart()}}, sf).Parts[0]; r.Status != StatusPossible {
			t.Fatalf("run %d: %s", i, r.Status)
		}
	}
	if n := len(m.Calls()); n != 2 {
		t.Errorf("%d model calls for the same question three times, want 2", n)
	}
}

func TestScopeFirstPolicy(t *testing.T) {
	for _, c := range []struct {
		name, reading, status string
		flagged, review       int
		reading_              string
	}{
		{"in", readIn, StatusPossible, 1, 0, "in"},
		{"unclear with a lean of in is flagged, marked unclear", readUnclearIn, StatusPossible, 1, 0, "unclear"},
		{"unclear with a lean of out is a person's to check, not a flag", readUnclearOut, StatusPerson, 0, 1, ""},
		{"out", readOut, StatusNone, 0, 0, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := checkOne(t, newScripted(`{"possible":[1]}`, c.reading, nil, nil), wheelPart())
			if r.Status != c.status || len(r.Matches) != c.flagged || len(r.Review) != c.review {
				t.Fatalf("status %s, %d matches, %d to review; want %s, %d, %d (%s)", r.Status, len(r.Matches), len(r.Review), c.status, c.flagged, c.review, r.Reason)
			}
			if c.flagged == 1 && r.Matches[0].Reading != c.reading_ {
				t.Errorf("reading %q, want %q", r.Matches[0].Reading, c.reading_)
			}
			if c.reading == readUnclearIn && (r.Matches[0].Why != "material not stated" || !strings.Contains(r.Reason, "a person must check")) {
				t.Errorf("an unclear flag needs the reader's reason and a person's check: %+v", r)
			}
			if c.status == StatusPerson && !strings.Contains(r.Reason, "a person must check") {
				t.Errorf("reason = %q", r.Reason)
			}
		})
	}
}

func TestScopeFirstDropsWhatTheScreenDoesNotListAndReadsNothingElse(t *testing.T) {
	m := newScripted(`{"possible":[]}`, readIn, nil, nil)
	r := checkOne(t, m, wheelPart())
	if r.Status != StatusNone {
		t.Fatalf("status = %s, want no_match", r.Status)
	}
	if n := len(m.Calls()); n != 1 {
		t.Errorf("%d calls, want only the screen", n)
	}
}

func TestScopeFirstANoAnswerIsNotAClearAnswer(t *testing.T) {
	for _, c := range []struct {
		name string
		m    *scriptedModel
	}{
		{"screen fails", newScripted("", "", errors.New("boom"), nil)},
		{"screen is not JSON", newScripted("no idea", readIn, nil, nil)},
		{"reader fails", newScripted(`{"possible":[1]}`, "", nil, errors.New("boom"))},
		{"reader is not JSON", newScripted(`{"possible":[1]}`, "no idea", nil, nil)},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := checkOne(t, c.m, wheelPart())
			if r.Status != StatusNotChecked || len(r.Matches) != 0 || !strings.Contains(r.Reason, "did not answer") {
				t.Errorf("result = %+v, want not_checked", r)
			}
		})
	}
}

func TestScopeFirstNeedsANameAndAnOriginAndACountryWithOrders(t *testing.T) {
	m := newScripted(`{"possible":[1]}`, readIn, nil, nil)
	for _, c := range []struct {
		p      Part
		status string
	}{
		{Part{HTS: "8716.90.5035", Origin: "China"}, StatusNotChecked},
		{Part{Name: "steel wheel", HTS: "8716.90.5035"}, StatusNotChecked},
		{Part{Name: "steel wheel", HTS: "8716.90.5035", Origin: "Norway"}, StatusNone},
		{Part{Name: "zebra", Origin: "China"}, StatusNone},
	} {
		if r := checkOne(t, m, c.p); r.Status != c.status {
			t.Errorf("%+v: status %s, want %s", c.p, r.Status, c.status)
		}
	}
	if n := len(m.Calls()); n != 0 {
		t.Errorf("%d model calls for parts the check cannot or need not read, want 0", n)
	}
}

func TestScopeFirstWorksWithoutAnHTSCode(t *testing.T) {
	m := newScripted(`{"possible":[1]}`, readIn, nil, nil)
	r := checkOne(t, m, Part{Name: "steel wheel for a trailer", Origin: "China"})
	if r.Status != StatusPossible {
		t.Fatalf("status = %s, want possible: the code is only a hint", r.Status)
	}
	if u := m.Calls()[0].Messages[0].Content; strings.Contains(u, "[its listed HTS numbers") {
		t.Errorf("no code was given but the screen is told one matched: %q", u)
	}
}

func TestScopeFirstOneRequestIsCapped(t *testing.T) {
	m := newScripted(`{"possible":[]}`, readIn, nil, nil)
	sf, _ := NewScopeFirst(m, "gemini-4-argon")
	var in Input
	for i := 0; i < ScopeFirstMaxCalls+50; i++ {
		in.Parts = append(in.Parts, Part{Name: fmt.Sprintf("steel wheel number %d", i), Origin: "China"})
	}
	res := refineTable().CheckScopeFirst(in, sf)
	capped := 0
	for _, p := range res.Parts {
		if p.Status == StatusNotChecked {
			capped++
		}
	}
	if n := len(m.Calls()); n != ScopeFirstMaxCalls || capped != 50 {
		t.Errorf("%d calls and %d parts not checked, want %d and 50", n, capped, ScopeFirstMaxCalls)
	}
}

func TestScopeFirstResultSaysItIsNotAFinding(t *testing.T) {
	sf, _ := NewScopeFirst(newScripted(`{"possible":[1]}`, readIn, nil, nil), "gemini-4-argon")
	res := refineTable().CheckScopeFirst(Input{Parts: []Part{wheelPart()}}, sf)
	if !strings.Contains(res.Notice, "the written scope governs") || len(res.Limits) < 4 {
		t.Errorf("notice %q limits %d", res.Notice, len(res.Limits))
	}
	if res.Parts[0].Status != StatusPossible || !strings.Contains(res.Parts[0].Reason, "written scope governs") {
		t.Errorf("reason = %q", res.Parts[0].Reason)
	}
}

// The prompt the production check shows the name screen, and the key of a
// pair, are the ones the evaluation used: for 40 development queries
// (eval/adcvd/real/golden-screen.jsonl, written by screen_golden.py from the
// text label.py built) the pool, the list of orders and the keys are equal.
func TestScopeFirstScreenPromptIsTheEvaluationsOwn(t *testing.T) {
	tb, err := LoadFile("../../../eval/adcvd/orders-2026-11-27.v2.json")
	if err != nil {
		t.Skip("order list not present:", err)
	}
	b, err := os.ReadFile("../../../eval/adcvd/real/golden-screen.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	var p scopeFirstPrompts
	if err := json.Unmarshal(scopeFirstPromptsJSON, &p); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var g struct {
			ID, Description, Origin, HTS, User string
			ReaderKeyFirst                     string `json:"reader_key_first"`
		}
		if err := json.Unmarshal([]byte(line), &g); err != nil {
			t.Fatal(err)
		}
		pool := tb.Pool(Query{Description: g.Description, Country: g.Origin, HTS: g.HTS}, ScopeFirstK)
		if len(pool) == 0 {
			t.Fatalf("%s: empty pool", g.ID)
		}
		got := strings.NewReplacer("{country}", pool[0].Country, "{description}", g.Description, "{facts}", "", "{orders}", screenList(pool)).Replace(p.Screen.User)
		if got != g.User {
			t.Fatalf("%s: screen prompt differs from the evaluation's\n got  %q\n want %q", g.ID, got[:min(len(got), 400)], g.User[:min(len(g.User), 400)])
		}
		if k := PairKey(Order{Product: pool[0].Product, Country: pool[0].Country, ScopeText: pool[0].Scope}, g.Description); k != g.ReaderKeyFirst {
			t.Errorf("%s: pair key %s, want %s", g.ID, k, g.ReaderKeyFirst)
		}
		n++
	}
	if n != 40 {
		t.Errorf("%d golden rows, want 40", n)
	}
}
