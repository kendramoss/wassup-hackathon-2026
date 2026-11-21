package hts

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

func TestPartStatements(t *testing.T) {
	text := "The article consists of an MDF panel and aluminum pegs.\r\rThe aluminum pegs will be classified in subheading 8302.50, HTSUS. " +
		"The only non-originating component is the varistor, which as noted above, is classified in heading 8533. " +
		"The subject merchandise is classified in subheading 8302.50.0000, HTSUS. " +
		"The mesh jacket classifiable under subheading 8421.99.01, HTSUS, is made in China. " +
		"The country of origin of a good classified in heading 6301 is the country in which the fabric is woven. " +
		"The kit is classified under subheading 9802.00.80."
	got := PartStatements(text)
	want := []struct{ part, hts string }{{"aluminum pegs", "8302.50"}, {"varistor", "8533"}, {"mesh jacket", "8421.99.01"}}
	if len(got) != len(want) {
		t.Fatalf("statements = %+v", got)
	}
	for i, w := range want {
		if got[i].Part != w.part || got[i].HTS != w.hts || got[i].Sentence == "" {
			t.Errorf("%d = %+v, want %s %s", i, got[i], w.part, w.hts)
		}
	}
	if !strings.HasPrefix(got[0].Sentence, "The aluminum pegs will be classified") {
		t.Errorf("sentence = %q", got[0].Sentence)
	}
}

func TestFormatCode(t *testing.T) {
	for in, want := range map[string]string{"8302.50": "8302.50", "8302.50.0000": "8302.50.00", "8533": "8533", "85334": "8533", "85": "", "8473.30.5100": "8473.30.51"} {
		if got := FormatCode(in); got != want {
			t.Errorf("FormatCode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShippedPartIndexLoads(t *testing.T) {
	x, err := LoadPartIndex(filepath.Join("..", "..", "data", "part_hts_index.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(x.Entries) < 500 {
		t.Errorf("entries = %d", len(x.Entries))
	}
}

func testIndex() *PartIndex {
	return &PartIndex{Entries: []PartEntry{
		{Ruling: "N100001", Date: "2023-01-10", Statement: Statement{Part: "electric motor", HTS: "8501.31", Sentence: "The electric motor is classified in subheading 8501.31, HTSUS."}},
		{Ruling: "N100002", Date: "2024-05-01", Statement: Statement{Part: "dc motor", HTS: "8501.31.40", Sentence: "The DC motor is classified in subheading 8501.31.40."}},
		{Ruling: "N100003", Date: "2025-02-01", Statement: Statement{Part: "plastic housing", HTS: "3926.90", Sentence: "The plastic housing is classified in subheading 3926.90."}},
	}}
}

// answerFake answers with codes per item key and counts calls.
func answerFake(t *testing.T, ans map[string][]map[string]any) *llm.Fake {
	return &llm.Fake{Respond: func(req llm.Request) (llm.Response, error) {
		var items []map[string]any
		for k, cs := range ans {
			items = append(items, map[string]any{"key": k, "candidates": cs})
		}
		b, _ := json.Marshal(map[string]any{"items": items})
		return llm.Response{Text: string(b), ModelID: "gemini-3.8-flash"}, nil
	}}
}

func product() domain.Product {
	return domain.Product{Name: "heat gun", Description: "Hand-held tool that blows hot air.",
		Components: []domain.Component{{Name: "Electric motor", MadeIn: "China"}, {Name: " "}, {Name: "Plastic housing", MadeIn: "Vietnam"}, {Name: "widget"}}}
}

func TestPartsSuggestLabelsSources(t *testing.T) {
	m := answerFake(t, map[string][]map[string]any{
		"p0": {{"hts": "8501.31", "reason": "Motor like N100001.", "cited": []string{"N100001", "N100003"}}, {"hts": "8501.31", "reason": "dup", "cited": []string{}}, {"hts": "9801.00", "reason": "no", "cited": []string{}}},
		"p2": {{"hts": "3926.90.99", "reason": "Plastic article.", "cited": []string{"N100003"}}, {"hts": "3926.90", "reason": "Housing like N100003.", "cited": []string{"N100003"}}},
	})
	checked := []string{}
	p := NewParts(m, "gemini-3.8-flash", testIndex(), nil, func(_ context.Context, sub string) bool {
		checked = append(checked, sub)
		return true
	})
	res, err := p.Suggest(context.Background(), product(), PartsOptions{Scope: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Calls()) != 1 || res.Trace == nil || res.Trace.PromptVersion != PartsPromptVersion || res.Note != UnverifiedNote || res.Good != nil {
		t.Fatalf("calls=%d res=%+v", len(m.Calls()), res)
	}
	if len(res.Parts) != 3 || res.Parts[0].Index != 0 || res.Parts[1].Index != 2 || res.Parts[2].Index != 3 {
		t.Fatalf("parts = %+v (the unnamed part is skipped)", res.Parts)
	}
	motor := res.Parts[0].Candidates
	if len(motor) != 1 || motor[0].HTS != "8501.31" || motor[0].Source != SourceRulings || len(motor[0].Cited) != 1 || motor[0].Cited[0] != "N100001" {
		t.Errorf("motor = %+v (a duplicate and a chapter 98 code are dropped; only its own rulings are cited)", motor)
	}
	housing := res.Parts[1].Candidates
	if len(housing) != 2 || housing[0].Source != SourceModel || len(housing[0].Supporting) != 0 || len(housing[0].Cited) != 0 || housing[1].Source != SourceRulings {
		t.Errorf("housing = %+v (a code no ruling lists is the model's)", housing)
	}
	if len(checked) != 1 || checked[0] != "3926.90.99" {
		t.Errorf("checked against today's HTS = %v, want only the model's 8-digit code", checked)
	}
	if len(res.Parts[2].Candidates) != 0 {
		t.Errorf("widget = %+v", res.Parts[2])
	}
	prompt := m.Calls()[0].Messages[0].Content
	for _, want := range []string{"PRODUCT: heat gun", `ITEM p0: part "Electric motor" (made in China)`, "N100001 (2023-01-10) classified \"electric motor\" in 8501.31", `ITEM p3: part "widget"`, "No earlier ruling in the index"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt lacks %q:\n%s", want, prompt)
		}
	}

	// Again for the same caller: every part from the cache, no model call.
	res2, err := p.Suggest(context.Background(), product(), PartsOptions{Scope: "a"})
	if err != nil || len(m.Calls()) != 1 || !res2.Parts[0].Cached || res2.Parts[0].Candidates[0].HTS != "8501.31" {
		t.Errorf("cached = %+v %v calls=%d", res2.Parts, err, len(m.Calls()))
	}
}

func TestPartsSuggestDropsCodesNotInTodaysHTS(t *testing.T) {
	m := answerFake(t, map[string][]map[string]any{"p0": {{"hts": "8501.10.99", "reason": "Old number.", "cited": []string{}}, {"hts": "8501.31", "reason": "Motor.", "cited": []string{"N100001"}}}})
	p := NewParts(m, "x", testIndex(), nil, func(_ context.Context, sub string) bool { return sub != "8501.10.99" })
	res, _ := p.Suggest(context.Background(), domain.Product{Name: "fan", Components: []domain.Component{{Name: "motor"}}}, PartsOptions{})
	if cs := res.Parts[0].Candidates; len(cs) != 1 || cs[0].HTS != "8501.31" {
		t.Errorf("candidates = %+v", cs)
	}
}

func TestPartsSuggestModelDownShowsRulingCandidates(t *testing.T) {
	m := &llm.Fake{Respond: func(llm.Request) (llm.Response, error) { return llm.Response{}, llm.ErrUnavailable }}
	p := NewParts(m, "x", testIndex(), nil, nil)
	res, err := p.Suggest(context.Background(), product(), PartsOptions{Scope: "a"})
	if err == nil || !res.ReasonsUnavailable {
		t.Fatalf("err=%v res=%+v", err, res)
	}
	if cs := res.Parts[0].Candidates; len(cs) == 0 || cs[0].Source != SourceRulings || cs[0].Reason != "" {
		t.Errorf("motor = %+v", cs)
	}
	// A failed answer is not cached.
	m.Respond = func(llm.Request) (llm.Response, error) { return llm.Response{Text: `{"items":[]}`}, nil }
	if res, _ := p.Suggest(context.Background(), product(), PartsOptions{Scope: "a"}); res.Parts[0].Cached {
		t.Errorf("a failed answer was cached")
	}
}

func TestPartsSuggestBeforeHidesLaterAndOwnRulings(t *testing.T) {
	p := NewParts(llm.FakeText(`{"items":[]}`), "x", testIndex(), nil, nil)
	cs := p.partCandidates("dc motor", time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC), "")
	for _, c := range cs {
		for _, r := range c.Supporting {
			if r.ID == "N100002" {
				t.Errorf("a ruling dated on the bound was used: %+v", cs)
			}
		}
	}
	cs = p.partCandidates("electric motor", time.Time{}, "N100001")
	for _, c := range cs {
		for _, r := range c.Supporting {
			if r.ID == "N100001" {
				t.Errorf("the excluded ruling was used: %+v", cs)
			}
		}
	}
	// With Before set nothing is cached.
	res, _ := p.Suggest(context.Background(), product(), PartsOptions{Before: time.Now(), Scope: "a"})
	res2, _ := p.Suggest(context.Background(), product(), PartsOptions{Scope: "a"})
	if res.Parts[0].Cached || res2.Parts[0].Cached {
		t.Errorf("evaluation answers were cached")
	}
}

func TestPartsSuggestDeduplicatesAndCachesPerProduct(t *testing.T) {
	m := answerFake(t, map[string][]map[string]any{"p0": {{"hts": "8501.31", "reason": "Motor.", "cited": []string{"N100001"}}}})
	p := NewParts(m, "x", testIndex(), nil, nil)
	prod := domain.Product{Name: "Fan", Components: []domain.Component{{Name: "Motor"}, {Name: "motor."}}}
	res, _ := p.Suggest(context.Background(), prod, PartsOptions{Scope: "a"})
	if len(res.Parts) != 2 || res.Parts[1].Candidates[0].HTS != "8501.31" {
		t.Fatalf("parts = %+v", res.Parts)
	}
	if !strings.Contains(m.Calls()[0].Messages[0].Content, "ITEM p0") || strings.Contains(m.Calls()[0].Messages[0].Content, "ITEM p1") {
		t.Errorf("the same part was asked twice")
	}
	fan := domain.Product{Name: "Fan", Description: "A desk fan."}
	if CacheKey("a", "Motor", "China", fan) != CacheKey("a", " motor. ", "china", domain.Product{Name: "fan", Description: "a desk  fan"}) {
		t.Errorf("cache key does not normalise")
	}
	for name, other := range map[string]string{
		"scope":       CacheKey("b", "motor", "China", fan),
		"product":     CacheKey("a", "motor", "China", domain.Product{Name: "Drill", Description: "A desk fan."}),
		"description": CacheKey("a", "motor", "China", domain.Product{Name: "Fan", Description: "A ceiling fan."}),
		"made in":     CacheKey("a", "motor", "Vietnam", fan),
	} {
		if other == CacheKey("a", "motor", "China", fan) {
			t.Errorf("cache key ignores the %s", name)
		}
	}
	// A different product asks again.
	prod.Name = "Drill"
	if res, _ := p.Suggest(context.Background(), prod, PartsOptions{Scope: "a"}); res.Parts[0].Cached || len(m.Calls()) != 2 {
		t.Errorf("another product's part came from the cache")
	}
	// No scope: nothing is cached or served from the cache.
	if res, _ := p.Suggest(context.Background(), prod, PartsOptions{}); res.Parts[0].Cached || len(m.Calls()) != 3 {
		t.Errorf("an unscoped request used the cache")
	}
}

// #577: a cached answer, and the reason the model wrote from one caller's
// description, never reach another caller or another description.
func TestPartsCacheNeverCrossesCallersOrDescriptions(t *testing.T) {
	m := &llm.Fake{Respond: func(req llm.Request) (llm.Response, error) {
		desc := ""
		for _, line := range strings.Split(req.Messages[0].Content, "\n") {
			if d, ok := strings.CutPrefix(line, "DESCRIPTION: "); ok {
				desc = d
			}
		}
		b, _ := json.Marshal(map[string]any{"items": []any{map[string]any{"key": "p0", "candidates": []any{
			map[string]any{"hts": "8538.90", "reason": "Housing of " + desc, "cited": []string{}}}}}})
		return llm.Response{Text: string(b)}, nil
	}}
	p := NewParts(m, "x", testIndex(), nil, nil)
	prod := func(desc string) domain.Product {
		return domain.Product{Name: "Controller", Description: desc, Components: []domain.Component{{Name: "housing", MadeIn: "China"}}}
	}
	secret := "ALPHA-SECRET: lidar controller rev 9, customer Foo"
	a, _ := p.Suggest(context.Background(), prod(secret), PartsOptions{Scope: "alice"})
	if !strings.Contains(a.Parts[0].Candidates[0].Reason, "ALPHA-SECRET") {
		t.Fatalf("alice = %+v", a.Parts)
	}
	for _, tc := range []struct{ name, scope, desc string }{
		{"another caller, same product words", "bob", secret},
		{"another caller, another description", "bob", "Garden irrigation controller"},
		{"same caller, another description", "alice", "Garden irrigation controller"},
	} {
		calls := len(m.Calls())
		res, _ := p.Suggest(context.Background(), prod(tc.desc), PartsOptions{Scope: tc.scope})
		if res.Parts[0].Cached || len(m.Calls()) != calls+1 {
			t.Errorf("%s: served from the cache", tc.name)
		}
		if tc.desc != secret && strings.Contains(res.Parts[0].Candidates[0].Reason, "ALPHA-SECRET") {
			t.Errorf("%s: reason carries another description: %q", tc.name, res.Parts[0].Candidates[0].Reason)
		}
	}
	// The same caller asking the same thing again is served from the cache.
	if res, _ := p.Suggest(context.Background(), prod(secret), PartsOptions{Scope: "alice"}); !res.Parts[0].Cached {
		t.Errorf("the same caller's repeat was not cached")
	}
}

func TestPartsCacheIsBounded(t *testing.T) {
	p := NewParts(llm.FakeText(`{}`), "x", testIndex(), nil, nil)
	now := time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)
	p.now = func() time.Time { return now }
	key := func(prod string) string { return CacheKey("a", "part", "", domain.Product{Name: prod}) }
	for i := range cacheMax + 10 {
		now = now.Add(time.Second)
		p.store(key(string(rune('a'+i%26))+strings.Repeat("x", i)), nil)
	}
	if len(p.cache) > cacheMax {
		t.Errorf("cache = %d entries", len(p.cache))
	}
	if _, ok := p.cached(key("a")); ok {
		t.Errorf("the oldest entry survived")
	}
	now = now.Add(cacheTTL + time.Minute)
	if _, ok := p.cached(key("k" + strings.Repeat("x", cacheMax+9))); ok {
		t.Errorf("an expired entry was served")
	}
}

func TestPartsSuggestGood(t *testing.T) {
	x, err := LoadIndex(filepath.Join("..", "..", "data", "hts_index.json"))
	if err != nil {
		t.Fatal(err)
	}
	m := &llm.Fake{Respond: func(req llm.Request) (llm.Response, error) {
		if !strings.Contains(req.Messages[0].Content, "ITEM good: the finished product") {
			t.Errorf("good not asked:\n%s", req.Messages[0].Content)
		}
		return llm.Response{Text: `{"items":[{"key":"good","candidates":[{"hts":"8516.79.00","reason":"Electro-thermic appliance.","cited":[]}]}]}`}, nil
	}}
	s, err := New(m, "x", x, filepath.Join("..", "..", "data", "facts"), nil)
	if err != nil {
		t.Fatal(err)
	}
	p := NewParts(m, "x", testIndex(), s, nil)
	res, err := p.Suggest(context.Background(), product(), PartsOptions{Good: true})
	if err != nil || res.Good == nil || res.Good.Source != SourceIndex || len(res.Good.Candidates) != 1 || res.Good.Candidates[0].HTS != "8516.79.00" {
		t.Fatalf("good = %+v %v", res.Good, err)
	}
}

func TestSimulatedPartsAnswer(t *testing.T) {
	var got string
	m := &llm.Fake{Respond: func(req llm.Request) (llm.Response, error) {
		if req.SchemaName != PartsSchemaName {
			t.Errorf("schema = %q", req.SchemaName)
		}
		got = SimulatedPartsAnswer(req.Messages[0].Content)
		return llm.Response{Text: got}, nil
	}}
	p := NewParts(m, "x", testIndex(), nil, nil)
	res, err := p.Suggest(context.Background(), product(), PartsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	motor := res.Parts[0].Candidates
	if len(motor) != 1 || motor[0].Source != SourceRulings || !strings.Contains(motor[0].Reason, "not judged") || len(motor[0].Cited) != 1 {
		t.Errorf("motor = %+v (answer %s)", motor, got)
	}
	if len(res.Parts[2].Candidates) != 0 {
		t.Errorf("widget, with no ruling, got %+v", res.Parts[2].Candidates)
	}
}
