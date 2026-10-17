package hts

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

func TestExtract(t *testing.T) {
	tests := []struct {
		name, text string
		want       []string
		source     string
	}{
		{"tariff no header, 10 digits, 9903 skipped", "N351889\r\nCATEGORY: Classification\r\nTARIFF NO.: 8204.20.0000; 8466.10.0175; 9903.02.69\r\nDear Mr. X", []string{"8204.20.00", "8466.10.01"}, SourceTariffNo},
		{"tariff no without colon, 8 digits", "TARIFF NO. 8476.89.00\n", []string{"8476.89.00"}, SourceTariffNo},
		{"duplicates collapse to one subheading", "TARIFF NO.: 8481.80.1095, 8481.80.1090\n", []string{"8481.80.10"}, SourceTariffNo},
		{"only special provisions in header: fall through to holding", "TARIFF NO.: 9802.00.5060\nHOLDING:\nThe ring is classified in subheading 7113.19.5000, HTSUS.", []string{"7113.19.50"}, SourceHolding},
		{"last holding section wins", "HOLDING:\nsee 1111.11.11\nlater text\nHOLDINGS:\nclassified in 8424.30.9000", []string{"8424.30.90"}, SourceHolding},
		{"applicable subheading phrase", "Facts.\n\nThe applicable subheading for the pressure washer (model 61255) is 8424.30.9000, Harmonized Tariff Schedule.", []string{"8424.30.90"}, SourceApplicable},
		{"part of a longer number is not a code", "Protest No. 12345.67.8901 and 1234.56.78.90.12\n", nil, ""},
		{"a 9-digit run is not a code", "applicable subheading is 8424.30.900", []string{"8424.30.90"}, SourceApplicable},
		{"nothing stated", "The country of origin of the heat gun is Thailand.", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, src := Extract(tt.text)
			if !reflect.DeepEqual(got, tt.want) || src != tt.source {
				t.Errorf("Extract = %v %q, want %v %q", got, src, tt.want, tt.source)
			}
		})
	}
}

func writeIndex(t *testing.T, x Index) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "hts_index.json")
	b, _ := json.Marshal(x)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadIndexChecksSubheadings(t *testing.T) {
	ok := Index{Rulings: []Entry{{Ruling: "N100001", HTS: []string{"8467.29.00"}}, {Ruling: "N100002", HTS: []string{"8467.29.00", "8516.31.00"}}}}
	x, err := LoadIndex(writeIndex(t, ok))
	if err != nil || len(x.Rulings) != 2 || x.Subheadings() != 2 {
		t.Fatalf("LoadIndex = %+v %v", x, err)
	}
	for name, bad := range map[string]Index{
		"empty":         {},
		"no subheading": {Rulings: []Entry{{Ruling: "N1", HTS: []string{}}}},
		"10 digits":     {Rulings: []Entry{{Ruling: "N1", HTS: []string{"8467.29.0010"}}}},
	} {
		if _, err := LoadIndex(writeIndex(t, bad)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

// fixture: three heat-gun-like rulings in two subheadings, a hair dryer and
// an unrelated shrimp ruling; N100006 has no facts file.
func fixture(t *testing.T) (*Index, string) {
	t.Helper()
	dir := t.TempDir()
	facts := map[string]string{
		"N100001": "The merchandise is a heat gun with a motor, fan, heating element and housing, used to strip paint.",
		"N100002": "A cordless heat gun with a heating element, motor and fan; it strips paint and shrinks tubing.",
		"N100003": "A heat gun kit: the heat gun, nozzles and a scraper packed in a case.",
		"N100004": "A hand-held hair dryer with a motor, fan and heating element for drying hair.",
		"N100005": "Frozen shrimp, peeled and deveined, cooked in Indonesia.",
	}
	for id, f := range facts {
		if err := os.WriteFile(filepath.Join(dir, id+".txt"), []byte(f+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return &Index{Rulings: []Entry{
		{Ruling: "N100001", Date: "2024-01-10", Subject: "The tariff classification and country of origin of a heat gun", HTS: []string{"8419.89.95"}},
		{Ruling: "N100002", Date: "2025-03-01", Subject: "The tariff classification of a cordless heat gun", HTS: []string{"8419.89.95"}},
		{Ruling: "N100003", Date: "2025-06-01", Subject: "The tariff classification of a heat gun kit", HTS: []string{"8467.29.00", "8205.51.30"}},
		{Ruling: "N100004", Date: "2025-12-01", Subject: "The tariff classification of a hair dryer", HTS: []string{"8516.31.00"}},
		{Ruling: "N100005", Date: "2026-01-01", Subject: "The tariff classification of frozen shrimp", HTS: []string{"0306.17.00"}},
		{Ruling: "N100006", Date: "2026-02-01", Subject: "heat gun heat gun heat gun", HTS: []string{"8516.79.00"}},
	}}, dir
}

func newSuggester(t *testing.T, m llm.Model) *Suggester {
	t.Helper()
	x, dir := fixture(t)
	s, err := New(m, "flash", x, dir, func(code string) bool { return code == "8419.89.95" })
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCandidatesGroupRulingsBySubheading(t *testing.T) {
	s := newSuggester(t, llm.FakeText("{}"))
	if n, subs := s.Coverage(); n != 5 || subs != 5 {
		t.Errorf("coverage = %d rulings %d subheadings, want 5 and 5 (N100006 and its 8516.79.00 have no facts)", n, subs)
	}
	cs := s.Candidates("A heat gun with a heating element and fan for stripping paint.")
	if len(cs) != MaxCandidates {
		t.Fatalf("candidates = %+v", cs)
	}
	if cs[0].HTS != "8419.89.95" || len(cs[0].Supporting) != 2 || !cs[0].DutyRates {
		t.Errorf("best = %+v, want 8419.89.95 with two rulings and duty rates", cs[0])
	}
	ref := cs[0].Supporting[0]
	if ref.Date == "" || !strings.Contains(ref.Subject, "heat gun") || ref.URL != domain.RulingURL(ref.ID) {
		t.Errorf("ref = %+v", ref)
	}
	for _, c := range cs {
		if c.HTS == "0306.17.00" || c.HTS == "8516.79.00" {
			t.Errorf("unrelated or factless subheading suggested: %+v", c)
		}
		if c.HTS != "8419.89.95" && c.DutyRates {
			t.Errorf("%s marked with duty rates", c.HTS)
		}
	}
	if got := s.Candidates("zzz qqq"); len(got) != 0 {
		t.Errorf("no shared term still suggested %+v", got)
	}
}

func TestSuggestReasonsCiteOnlyRetrievedRulings(t *testing.T) {
	reply := `{"candidates":[
	 {"hts":"8419.89.95","reason":" Both rulings are heat guns for stripping paint. ","cited":["N100001","N100005","N100001"]},
	 {"hts":"8467.29.00","reason":"A kit, not a single tool.","cited":["N100003"]},
	 {"hts":"9999.99.99","reason":"invented","cited":["N100004"]}]}`
	f := llm.FakeText(reply)
	s := newSuggester(t, f)
	res, err := s.Suggest(context.Background(), "A heat gun with a heating element and fan for stripping paint.")
	if err != nil {
		t.Fatal(err)
	}
	if res.Note != Note || res.ReasonsUnavailable || res.Trace == nil || res.Trace.PromptVersion != PromptVersion || res.Trace.ModelID != "flash" {
		t.Fatalf("result = %+v", res)
	}
	best := res.Candidates[0]
	if best.Reason != "Both rulings are heat guns for stripping paint." || !reflect.DeepEqual(best.Cited, []string{"N100001"}) {
		t.Errorf("best = %+v (N100005 is not retrieved for it; duplicates dropped)", best)
	}
	for _, c := range res.Candidates {
		if c.HTS == "9999.99.99" {
			t.Errorf("model's invented subheading returned")
		}
		if c.Cited == nil {
			t.Errorf("%s: cited is nil, want []", c.HTS)
		}
	}
	calls := f.Calls()
	if len(calls) != 1 {
		t.Fatalf("model calls = %d, want 1", len(calls))
	}
	req := calls[0]
	if req.SchemaName != "hts_suggestion" || req.Model != "flash" || !strings.Contains(req.Messages[0].Content, "CANDIDATE 1: 8419.89.95") ||
		!strings.Contains(req.Messages[0].Content, "strip paint") {
		t.Errorf("request = %+v", req)
	}
	items := req.JSONSchema["properties"].(map[string]any)["candidates"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	if enum := items["hts"].(map[string]any)["enum"].([]string); len(enum) != len(res.Candidates) || enum[0] != "8419.89.95" {
		t.Errorf("hts enum = %v", enum)
	}
	cited := items["cited"].(map[string]any)["items"].(map[string]any)["enum"].([]string)
	for _, id := range cited {
		if id == "N100005" || id == "N100006" {
			t.Errorf("cited enum offers %s, which was not retrieved", id)
		}
	}
}

func TestSuggestKeepsCandidatesWhenTheModelFails(t *testing.T) {
	for name, m := range map[string]*llm.Fake{
		"unavailable":  {Respond: func(llm.Request) (llm.Response, error) { return llm.Response{}, llm.ErrUnavailable }},
		"bad response": llm.FakeText("no json here"),
	} {
		t.Run(name, func(t *testing.T) {
			res, err := newSuggester(t, m).Suggest(context.Background(), "heat gun heating element fan")
			if err == nil || !res.ReasonsUnavailable || len(res.Candidates) == 0 || res.Candidates[0].Reason != "" || res.Note != Note {
				t.Errorf("res = %+v err = %v", res, err)
			}
		})
	}
	m := &llm.Fake{Respond: func(llm.Request) (llm.Response, error) { return llm.Response{}, errors.New("should not be called") }}
	res, err := newSuggester(t, m).Suggest(context.Background(), "zzz")
	if err != nil || len(res.Candidates) != 0 || len(m.Calls()) != 0 || res.Note != Note {
		t.Errorf("no candidates: res = %+v err = %v calls = %d", res, err, len(m.Calls()))
	}
}

func TestProductText(t *testing.T) {
	p := domain.Product{Name: "heat gun", Description: "Strips paint.",
		Components: []domain.Component{{Name: "motor", MadeIn: "China"}, {Name: "heating element", MadeIn: "Thailand"}},
		Operations: []domain.Operation{{Name: "final assembly", Plant: "th"}}}
	want := "The merchandise is a heat gun. Strips paint. It consists of motor, heating element. Processing: final assembly."
	if got := ProductText(p); got != want {
		t.Errorf("ProductText = %q", got)
	}
}

// TestShippedIndex checks data/hts_index.json loads, and that every ruling in
// it has a facts file, so the server serves all of it.
func TestShippedIndex(t *testing.T) {
	x, err := LoadIndex(filepath.Join("..", "..", "data", "hts_index.json"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(llm.FakeText("{}"), "m", x, filepath.Join("..", "..", "data", "facts"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if n, subs := s.Coverage(); n != len(x.Rulings) || subs != x.Subheadings() || n < 800 {
		t.Errorf("coverage = %d rulings %d subheadings of %d / %d", n, subs, len(x.Rulings), x.Subheadings())
	}
	cs := s.Candidates("Multilayer wood flooring with a face veneer of red oak, a core of pine blocks and a eucalyptus back ply.")
	if len(cs) == 0 || !strings.HasPrefix(cs[0].HTS, "4412.") {
		t.Errorf("wood flooring candidates = %+v, want heading 4412 first", cs)
	}
}
