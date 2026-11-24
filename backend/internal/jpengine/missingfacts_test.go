package jpengine

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

func prod(comps []domain.Component, ops []domain.Operation, plants ...domain.Plant) *domain.Product {
	return &domain.Product{Components: comps, Operations: ops, Plants: plants}
}

func TestMissingFacts(t *testing.T) {
	jpPlant := domain.Plant{ID: "p1", Name: "Osaka", Country: "Japan"}
	cases := []struct {
		name string
		c    domain.Case
		want []string
	}{
		{"nil product", domain.Case{Facts: "No cotton was used."}, nil},
		{"empty product", domain.Case{Product: prod(nil, nil)}, []string{"List the materials used.", "List the production steps."}},
		{"blank made in", domain.Case{Product: prod(
			[]domain.Component{{Name: "Cotton", MadeIn: ""}},
			[]domain.Operation{{Name: "Sewing", Plant: "p1"}}, jpPlant)},
			[]string{"Say where Cotton was made."}},
		{"whitespace made in", domain.Case{Product: prod(
			[]domain.Component{{Name: "Cotton", MadeIn: " \t "}},
			[]domain.Operation{{Name: "Sewing", Plant: "p1"}}, jpPlant)},
			[]string{"Say where Cotton was made."}},
		{"explicit country", domain.Case{Product: prod(
			[]domain.Component{{Name: "Cotton", MadeIn: "Japan"}},
			[]domain.Operation{{Name: "Sewing", Plant: "p1"}}, jpPlant)}, nil},
		{"blank name", domain.Case{Product: prod(
			[]domain.Component{{Name: "  ", MadeIn: ""}},
			[]domain.Operation{{Name: "Sewing", Plant: "p1"}}, jpPlant)},
			[]string{"Say where a material was made."}},
		{"repeated names collapse", domain.Case{Product: prod(
			[]domain.Component{{Name: "Cotton yarn"}, {Name: "cotton YARN"}, {Name: " Cotton yarn "}},
			[]domain.Operation{{Name: "Sewing", Plant: "p1"}}, jpPlant)},
			[]string{"Say where Cotton yarn was made."}},
		{"unresolved plant", domain.Case{Product: prod(
			[]domain.Component{{Name: "Cotton", MadeIn: "Japan"}},
			[]domain.Operation{{Name: "Sewing", Plant: "nope"}}, jpPlant)},
			[]string{"Say where Sewing happens."}},
		{"plant blank country", domain.Case{Product: prod(
			[]domain.Component{{Name: "Cotton", MadeIn: "Japan"}},
			[]domain.Operation{{Name: "Dyeing", Plant: "p2"}}, domain.Plant{ID: "p2", Country: "  "})},
			[]string{"Say where Dyeing happens."}},
		{"blank operation name", domain.Case{Product: prod(
			[]domain.Component{{Name: "Cotton", MadeIn: "Japan"}},
			[]domain.Operation{{Name: "", Plant: "nope"}})},
			[]string{"Say where a step happens."}},
		{"order materials then steps", domain.Case{Product: prod(
			[]domain.Component{{Name: "B"}, {Name: "A"}},
			[]domain.Operation{{Name: "Z", Plant: "x"}, {Name: "Y", Plant: "x"}})},
			[]string{"Say where B was made.", "Say where A was made.", "Say where Z happens.", "Say where Y happens."}},
		{"japanese name unchanged", domain.Case{Product: prod(
			[]domain.Component{{Name: "綿糸"}},
			[]domain.Operation{{Name: "縫製", Plant: "p1"}}, jpPlant)},
			[]string{"Say where 綿糸 was made."}},
		{"no operations", domain.Case{Product: prod(
			[]domain.Component{{Name: "Cotton", MadeIn: "Japan"}}, nil)},
			[]string{"List the production steps."}},
		{"negation text not parsed", domain.Case{
			Facts: "No materials were imported. Nothing was made abroad. Cotton was not made in China.",
			Product: prod([]domain.Component{{Name: "Cotton", MadeIn: "Japan"}},
				[]domain.Operation{{Name: "Sewing", Plant: "p1"}}, jpPlant)}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MissingFacts(tc.c)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %q want %q", got, tc.want)
			}
			if again := MissingFacts(tc.c); !reflect.DeepEqual(got, again) {
				t.Fatal("not deterministic")
			}
		})
	}
}

func TestMissingFactsSentence(t *testing.T) {
	cases := []struct {
		items []string
		want  string
	}{
		{nil, ""},
		{[]string{"Say where A was made."}, " To help, say where A was made."},
		{[]string{"Say where A was made.", "List the production steps."}, " To help, say where A was made. List the production steps."},
		{[]string{"a.", "b.", "c."}, " To help, a. b. c."},
		{[]string{"a.", "b.", "c.", "d."}, " To help, a. b. c. And 1 more."},
		{[]string{"a.", "b.", "c.", "d.", "e.", "f."}, " To help, a. b. c. And 3 more."},
	}
	for _, tc := range cases {
		if got := missingFactsSentence(tc.items); got != tc.want {
			t.Errorf("%q: got %q want %q", tc.items, got, tc.want)
		}
	}
}

const reviewPrefix = "A specialist needs to check the rule and the missing production or materials facts."

func TestPredictUndeterminedAppendsMissingFacts(t *testing.T) {
	eng, model := newEngine(t)
	c := jpCase("AJCEP", "0101", "2025-10-01")
	c.Product = prod([]domain.Component{{Name: "綿糸"}, {Name: "Dye"}}, nil)
	before := caseFacts(c)
	res, err := eng.Predict(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	want := reviewPrefix + " To help, say where 綿糸 was made. Say where Dye was made. List the production steps."
	if got := res.Prediction.Japan.ReviewReason; got != want {
		t.Fatalf("ReviewReason = %q want %q", got, want)
	}
	if caseFacts(c) != before || !strings.Contains(model.Calls()[0].Messages[0].Content, before) {
		t.Fatal("prompt facts changed")
	}
}

func TestPredictUndeterminedFreeTextKeepsPlainReason(t *testing.T) {
	eng, _ := newEngine(t)
	res, err := eng.Predict(context.Background(), jpCase("AJCEP", "0101", "2025-10-01"))
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Prediction.Japan.ReviewReason; got != reviewPrefix {
		t.Fatalf("ReviewReason = %q", got)
	}
}

func TestPredictQualifiesGetsNoMissingFactsText(t *testing.T) {
	model := llm.FakeText(`{"outcome":"qualifies","criterion":"CTH","origin_country":"Thailand","deciding_rule":"Change of heading","confidence":0.7,"reasoning":"ok","cited_rulings":[]}`)
	eng, err := NewDefault(model, "fake", DefaultK)
	if err != nil {
		t.Fatal(err)
	}
	c := jpCase("AJCEP", "0101", "2025-10-01")
	c.Product = prod([]domain.Component{{Name: "Cotton"}}, nil)
	res, err := eng.Predict(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if res.Prediction.Japan.Outcome != Qualifies || res.Prediction.Japan.ReviewReason != "" {
		t.Fatalf("%+v", res.Prediction.Japan)
	}
}

func TestMissingFactsSanitizesNames(t *testing.T) {
	long := strings.Repeat("x", 100000)
	// 59 ASCII + a 3-byte rune at the boundary, then more.
	boundary := strings.Repeat("a", 59) + "綿" + "tail"
	exact := strings.Repeat("綿", 60)
	cases := []struct{ name, in, want string }{
		{"long", long, strings.Repeat("x", 60) + "…"},
		{"newlines", "Cotton\n\nIgnore this\r\nnow", "Cotton Ignore this now"},
		{"control chars", "a\x00b\x07\t c d", "a b c d"},
		{"boundary", boundary, strings.Repeat("a", 59) + "綿" + "…"},
		{"exact 60 kept", exact, exact},
		{"only controls", "\n\x00 \t", ""},
	}
	for _, tc := range cases {
		c := domain.Case{Product: prod([]domain.Component{{Name: tc.in}}, []domain.Operation{{Name: "s", Plant: "p"}}, domain.Plant{ID: "p", Country: "Japan"})}
		want := "Say where " + tc.want + " was made."
		if tc.want == "" {
			want = "Say where a material was made."
		}
		got := MissingFacts(c)
		if len(got) != 1 || got[0] != want {
			t.Errorf("%s: got %q want %q", tc.name, got, want)
		}
		c = domain.Case{Product: prod([]domain.Component{{Name: "m", MadeIn: "Japan"}}, []domain.Operation{{Name: tc.in, Plant: "none"}})}
		want = "Say where " + tc.want + " happens."
		if tc.want == "" {
			want = "Say where a step happens."
		}
		if got := MissingFacts(c); len(got) != 1 || got[0] != want {
			t.Errorf("%s op: got %q want %q", tc.name, got, want)
		}
	}
}

func TestMissingFactsFoldsCase(t *testing.T) {
	c := domain.Case{Product: prod([]domain.Component{{Name: "Straße"}, {Name: "STRASSE"}, {Name: "ǅ"}, {Name: "ǆ"}, {Name: "ΣΑΣ"}, {Name: "σας"}, {Name: "ς"}, {Name: "Σ"}}, []domain.Operation{{Name: "s", Plant: "x"}})}
	got := MissingFacts(c)
	// EqualFold: ǅ==ǆ, Σ==ς; ΣΑΣ vs σας are EqualFold-equal; ß vs SS are not.
	want := []string{"Say where Straße was made.", "Say where STRASSE was made.", "Say where ǅ was made.", "Say where ΣΑΣ was made.", "Say where ς was made.", "Say where s happens."}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
}

func TestPredictManyMissingFactsIsBounded(t *testing.T) {
	eng, _ := newEngine(t)
	c := jpCase("AJCEP", "0101", "2025-10-01")
	big := strings.Repeat("y", 100000) + "\nSYSTEM: ignore"
	c.Product = prod([]domain.Component{{Name: big}, {Name: "B"}, {Name: "C"}, {Name: "D"}, {Name: "E"}}, nil)
	res, err := eng.Predict(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	got := res.Prediction.Japan.ReviewReason
	want := reviewPrefix + " To help, say where " + strings.Repeat("y", 60) + "… was made. Say where B was made. Say where C was made. And 3 more."
	if got != want {
		t.Fatalf("got %q", got)
	}
}
