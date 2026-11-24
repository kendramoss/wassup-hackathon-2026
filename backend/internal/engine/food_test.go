package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

func TestFoodCase(t *testing.T) {
	for _, tc := range []struct {
		name, facts string
		want        bool
	}{
		{"seafood", "Whole frozen squid of Peruvian origin will be shipped to China, thawed, cleaned and cut into rings.", true},
		{"puree", "Frozen pineapple chunks are imported into France and made into a puree.", true},
		{"poultry", "U.S. chicken is deboned and grilled in China.", true},
		{"tea", "Black tea is blended in Hong Kong.", true},
		{"two weak words", "Wheat flour and sugar are mixed into a dough in Cambodia.", true},
		{"one weak word: rice cooker", "The rice cooker has an aluminum pot and a Chinese heating plate.", false},
		{"one weak word: milk frother", "A milk frother is assembled in Vietnam.", false},
		{"hex nut", "A steel hex nut is threaded in Taiwan.", false},
		{"clamshell", "A clamshell case is moulded in China.", false},
		{"hydraulic oil", "The jack uses hydraulic oil and is assembled in Thailand.", false},
		{"machinery", "A pump impeller is cast in India and machined in Germany.", false},
		{"empty", "", false},
	} {
		if got := FoodCase(domain.Case{Facts: tc.facts}); got != tc.want {
			t.Errorf("%s: FoodCase = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestFoodCaseRulesOnlyForFood(t *testing.T) {
	if FoodCaseRules(domain.Case{Facts: "Frozen scallops are shucked in China."}) != FoodRules {
		t.Error("a food case did not get FoodRules")
	}
	if FoodCaseRules(domain.Case{Facts: "A heat gun is assembled in Thailand."}) != "" {
		t.Error("a non-food case got FoodRules")
	}
}

func TestFoodRulesReachOnlyFoodCases(t *testing.T) {
	const answer = `{"origin_country":"Peru","essential_character":"squid","decisive_step":"x","flip":null,"confidence":0.9,"reasoning":"r"}`
	f := llm.FakeText(answer)
	eng := New(f, nil, Options{Model: "m", CaseRules: FoodCaseRules, CaseRulesVersion: FoodVersion})
	when := time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC)

	food, err := eng.Predict(context.Background(), domain.Case{ID: "N1", Date: when, Facts: "Frozen squid is cleaned in China."})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Calls()[0].System; !strings.HasSuffix(got, "\n\n"+FoodRules) || !strings.HasPrefix(got, System) {
		t.Errorf("food system prompt does not end with FoodRules")
	}
	if food.Prediction.Trace.PromptVersion != PromptVersion+"+"+FoodVersion {
		t.Errorf("food prompt version = %q", food.Prediction.Trace.PromptVersion)
	}

	other, err := eng.Predict(context.Background(), domain.Case{ID: "N2", Date: when, Facts: "A pump impeller is cast in India."})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Calls()[1].System; got != System {
		t.Errorf("a non-food case's system prompt changed: %q", got)
	}
	if other.Prediction.Trace.PromptVersion != PromptVersion {
		t.Errorf("non-food prompt version = %q", other.Prediction.Trace.PromptVersion)
	}
}

func TestFoodRulesStackAfterRules(t *testing.T) {
	f := llm.FakeText(`{"origin_country":"Peru","essential_character":"squid","decisive_step":"x","flip":null,"confidence":0.9,"reasoning":"r"}`)
	eng := New(f, nil, Options{Model: "m", Rules: "RULES TEXT", RulesVersion: "rules-v1", CaseRules: FoodCaseRules, CaseRulesVersion: FoodVersion})
	res, err := eng.Predict(context.Background(), domain.Case{ID: "N1", Date: time.Now(), Facts: "Frozen squid is cleaned in China."})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Calls()[0].System; !strings.HasSuffix(got, "RULES TEXT\n\n"+FoodRules) {
		t.Error("FoodRules must follow Rules")
	}
	if res.Prediction.Trace.PromptVersion != PromptVersion+"+rules-v1+"+FoodVersion {
		t.Errorf("prompt version = %q", res.Prediction.Trace.PromptVersion)
	}
}

func TestFoodRulesKeyTheCache(t *testing.T) {
	plain := NewCache(New(llm.FakeText(`{}`), nil, Options{Model: "m"}), nil)
	food := NewCache(New(llm.FakeText(`{}`), nil, Options{Model: "m", CaseRules: FoodCaseRules, CaseRulesVersion: FoodVersion}), nil)
	when := time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC)
	squid := domain.Case{ID: "case_a", Date: when, Facts: "Frozen squid is cleaned in China."}
	pump := domain.Case{ID: "case_b", Date: when, Facts: "A pump impeller is cast in India."}
	if plain.key("m", squid) == food.key("m", squid) {
		t.Error("a food case shares a cache key with and without FoodRules")
	}
	if plain.key("m", pump) != food.key("m", pump) {
		t.Error("a non-food case's cache key changed")
	}
}
