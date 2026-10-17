package nearmiss

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/blob"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/verifier"
)

// oneRuling serves N325810 before 2026 and nothing else.
type oneRuling struct{ reads int }

func (o *oneRuling) Search(context.Context, string, time.Time, map[string]bool) ([]verifier.Hit, error) {
	return nil, nil
}

func (o *oneRuling) Read(_ context.Context, id string, before time.Time, exclude map[string]bool) (string, error) {
	o.reads++
	if id != "N325810" || exclude[id] || !before.After(time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)) {
		return "NOT FOUND (only rulings dated before the case are available).", nil
	}
	return "[N325810 | 2021-12-01] Hair dryer\nFACTS: motor and PCBA made in Thailand\nCBP ANALYSIS AND HOLDING: the motor and PCBA impart the character; Thailand.", nil
}

const good = `{"difference":"In N325810 the PCBA was also made in Thailand; in your setup it stays in China.","would_match":"Make the PCBA in Thailand as well."}`

var task = Task{Facts: "Heat gun. Motor-fan assembly made in Thailand; PCBA from China.", Ruling: "N325810",
	Before: time.Date(2026, 1, 14, 0, 0, 0, 0, time.UTC), Exclude: []string{"N328954"}}

func TestAdviseReadsTheRulingAndAnswers(t *testing.T) {
	s := &oneRuling{}
	f := llm.FakeText(good)
	n, err := New(f, "m", s, 1500).Advise(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	if n.Ruling != "N325810" || !strings.Contains(n.Difference, "PCBA") || n.WouldMatch == "" || n.Trace.PromptVersion != PromptVersion {
		t.Errorf("advice = %+v", n)
	}
	c := f.Calls()
	if len(c) != 1 || c[0].System != System || c[0].SchemaName != "near_miss" ||
		!strings.Contains(c[0].Messages[0].Content, "CBP ANALYSIS AND HOLDING") || !strings.Contains(c[0].Messages[0].Content, "PCBA from China") {
		t.Errorf("request = %+v", c)
	}
}

func TestMapperKeepsOnlyKnownGroups(t *testing.T) {
	f := llm.FakeText(`{"groups":["pcba","made_up","pcba","switch_cord_minor_parts"]}`)
	groups := []domain.OperationGroup{
		{ID: "pcba", Name: "PCBA (SMT)", Operations: []string{"SMT placement"}, Components: []string{"printed circuit board assembly"}},
		{ID: "switch_cord_minor_parts", Name: "Switch, cord and minor parts", Operations: []string{"make the switch"}},
	}
	ids, u, err := NewMapper(f, "m").MapGroups(context.Background(), "Do what the earlier importer did.", groups)
	if err != nil || strings.Join(ids, ",") != "pcba,switch_cord_minor_parts" {
		t.Errorf("ids = %v %v", ids, err)
	}
	c := f.Calls()
	if len(c) != 1 || c[0].System != MapSystem || c[0].SchemaName != "near_miss_groups" || !strings.Contains(c[0].Messages[0].Content, "- pcba: PCBA (SMT); components: printed circuit board assembly") {
		t.Errorf("request = %+v", c)
	}
	_ = u
	if _, _, err := NewMapper(llm.FakeText(`not json`), "m").MapGroups(context.Background(), "x", groups); err == nil {
		t.Error("a bad answer must be an error")
	}
}

func TestAdviseRefusesRulingsItMayNotRead(t *testing.T) {
	f := llm.FakeText(good)
	a := New(f, "m", &oneRuling{}, 1500)
	for name, tk := range map[string]Task{
		"not a ruling": {Facts: "x", Ruling: "bogus", Before: task.Before},
		"excluded":     {Facts: "x", Ruling: "N325810", Before: task.Before, Exclude: []string{"N325810"}},
		"too late":     {Facts: "x", Ruling: "N325810", Before: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)},
	} {
		if _, err := a.Advise(context.Background(), tk); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	if len(f.Calls()) != 0 {
		t.Errorf("no model call should be made, got %d", len(f.Calls()))
	}
}

func TestAdviseRejectsAnEmptyAnswer(t *testing.T) {
	_, err := New(llm.FakeText(`{"difference":" ","would_match":"x"}`), "m", &oneRuling{}, 1500).Advise(context.Background(), task)
	if !errors.Is(err, llm.ErrBadResponse) {
		t.Errorf("err = %v, want ErrBadResponse", err)
	}
}

func TestCacheAnswersTheSameTaskWithoutAModelCall(t *testing.T) {
	f := llm.FakeText(good)
	c := NewCache(New(f, "m", &oneRuling{}, 1500), blob.Dir{Dir: t.TempDir()}, "m")
	first, err := c.Advise(context.Background(), task)
	if err != nil || first.Trace.Cached {
		t.Fatalf("first = %+v %v", first, err)
	}
	second, err := c.Advise(context.Background(), task)
	if err != nil || !second.Trace.Cached || second.Difference != first.Difference || second.Trace.InputTokens != 0 {
		t.Errorf("second = %+v %v", second, err)
	}
	other := task
	other.Facts += " The switch moves too."
	if _, err := c.Advise(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	if len(f.Calls()) != 2 {
		t.Errorf("model calls = %d, want 2 (other facts are another key)", len(f.Calls()))
	}
}
