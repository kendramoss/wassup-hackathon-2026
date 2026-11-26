package jpengine

import (
	"reflect"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

func texts(qs []domain.JapanQuestion) []string {
	out := make([]string, len(qs))
	for i, q := range qs {
		out[i] = q.Text
	}
	return out
}

func TestQuestionsPerPart(t *testing.T) {
	c := domain.Case{Product: &domain.Product{
		Components: []domain.Component{{Name: "insole", MadeIn: "Vietnam"}, {Name: "counter", MadeIn: ""}},
	}}
	qs := Questions(c, "CPTPP")
	want := []string{"Does insole have proof of origin under CPTPP?", "Where is counter made?", "What are the production steps, and in which country?"}
	if !reflect.DeepEqual(texts(qs), want) {
		t.Fatalf("got %q", texts(qs))
	}
	if !reflect.DeepEqual(qs[0].Options, YesNoUnsure) || qs[1].Options != nil || qs[0].ID != "originating:0" || qs[1].ID != "made_in:1" {
		t.Fatalf("options or ids: %+v", qs)
	}
}

func TestQuestionsNamesTheAgreement(t *testing.T) {
	c := domain.Case{Product: &domain.Product{Components: []domain.Component{{Name: "veneer", MadeIn: "Malaysia"}},
		Operations: []domain.Operation{{Name: "pressing"}}}}
	if got := texts(Questions(c, "JP-EU EPA")); len(got) != 1 || got[0] != "Does veneer have proof of origin under the Japan–EU agreement?" {
		t.Fatalf("got %q", got)
	}
}

func TestQuestionsSkipAnswered(t *testing.T) {
	c := domain.Case{Facts: "Shoes.\n\nQ: Does insole have proof of origin under CPTPP?\nA: Yes\n",
		Product: &domain.Product{Components: []domain.Component{{Name: "insole", MadeIn: "Vietnam"}, {Name: "outsole", MadeIn: "China"}},
			Operations: []domain.Operation{{Name: "lasting"}}}}
	if got := texts(Questions(c, "CPTPP")); len(got) != 1 || !strings.Contains(got[0], "outsole") {
		t.Fatalf("got %q", got)
	}
}

func TestQuestionsFreeTextAndGSP(t *testing.T) {
	if got := Questions(domain.Case{Facts: "Plywood from Vietnam."}, "RCEP"); len(got) != 1 || got[0].ID != "materials" || got[0].Options != nil {
		t.Fatalf("free text: %+v", got)
	}
	c := domain.Case{Product: &domain.Product{Components: []domain.Component{{Name: "a", MadeIn: "Cambodia"}}}}
	if got := Questions(c, "GSP"); got != nil {
		t.Fatalf("GSP: %+v", got)
	}
}

func TestQuestionsCapped(t *testing.T) {
	var parts []domain.Component
	for i := 0; i < 9; i++ {
		parts = append(parts, domain.Component{Name: string(rune('a' + i)), MadeIn: "China"})
	}
	if got := Questions(domain.Case{Product: &domain.Product{Components: parts}}, "RCEP"); len(got) != maxQuestions {
		t.Fatalf("got %d questions", len(got))
	}
}

func TestUndeterminedResultCarriesQuestions(t *testing.T) {
	res, _, err := avRun(t, avAnswer(Undetermined, "NONE", "", "", "needs facts"), func(c *domain.Case) {
		c.Product = &domain.Product{Components: []domain.Component{{Name: "insole", MadeIn: "Vietnam"}}, Operations: []domain.Operation{{Name: "lasting"}}}
	})
	if err != nil || len(res.Prediction.Japan.Questions) != 1 || res.Prediction.Japan.Questions[0].ID != "originating:0" {
		t.Fatalf("questions = %+v, %v", res.Prediction.Japan.Questions, err)
	}
}

func TestQualifyingResultHasNoQuestions(t *testing.T) {
	res, _, err := avRun(t, avAnswer(Qualifies, "CTH", "Vietnam", "CTH", "ok"), func(c *domain.Case) {
		c.Product = &domain.Product{Components: []domain.Component{{Name: "insole", MadeIn: "Vietnam"}}}
	})
	if err != nil || res.Prediction.Japan.Questions != nil {
		t.Fatalf("questions = %+v, %v", res.Prediction.Japan.Questions, err)
	}
}

func TestAnswersReachTheModel(t *testing.T) {
	_, m, err := avRun(t, avAnswer(Undetermined, "NONE", "", "", "needs facts"), func(c *domain.Case) {
		c.Facts += "\n\nQ: Does insole have proof of origin under CPTPP?\nA: Yes\n"
	})
	if err != nil {
		t.Fatal(err)
	}
	reqs := m.Calls()
	if len(reqs) != 1 || !strings.Contains(reqs[0].Messages[0].Content, "A: Yes") {
		t.Fatalf("answer not in the request")
	}
}
