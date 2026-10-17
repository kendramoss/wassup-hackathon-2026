package clarify

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

func precs() []domain.Ruling {
	d := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	return []domain.Ruling{{ID: "N351809", Date: d, Facts: "cabin filter, media China", Holding: "China"},
		{ID: "N352485", Date: d, Facts: "cabin filter, media Thailand", Holding: "Thailand"}}
}

func TestReviewValidatesCitations(t *testing.T) {
	tests := []struct {
		name, text string
		onPoint    bool
		conflict   bool
		needsInfo  bool
		questions  int
	}{
		{"on point with conflict", `{"on_point":true,"closest_ruling":"N351809","on_point_reason":"same filter","conflict":{"ruling_a":"N351809","holding_a":"China","ruling_b":"N352485","holding_b":"Thailand","deciding_difference":"where the media is made"},"questions":["Where is the filter media made?"]}`, true, true, false, 1},
		{"on point but unseen ruling is demoted", `{"on_point":true,"closest_ruling":"N999999","on_point_reason":"x","conflict":null,"questions":["q1"]}`, false, false, true, 1},
		{"conflict with unseen ruling dropped", `{"on_point":false,"closest_ruling":null,"on_point_reason":"x","conflict":{"ruling_a":"N351809","holding_a":"a","ruling_b":"N111111","holding_b":"b","deciding_difference":"d"},"questions":[]}`, false, false, false, 0},
		{"questions capped at 3", `{"on_point":false,"closest_ruling":null,"on_point_reason":"x","conflict":null,"questions":["a","b"," ","c","d"]}`, false, false, true, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := llm.FakeText(tt.text)
			r, err := New(f, "m", 2000).Review(context.Background(), "facts", domain.Prediction{Origin: "China", Confidence: 0.8}, precs())
			if err != nil {
				t.Fatal(err)
			}
			if r.OnPoint != tt.onPoint || (r.Conflict != nil) != tt.conflict || r.NeedsInfo() != tt.needsInfo || len(r.Questions) != tt.questions {
				t.Errorf("result = %+v", r)
			}
			if r.Trace.PromptVersion != PromptVersion {
				t.Error("missing prompt version")
			}
			sent := f.Calls()[0].Messages[0].Content
			if !strings.Contains(sent, "[N352485 | 2025-01-01]") {
				t.Error("precedents not in prompt")
			}
			if sent != RenderReview("facts", domain.Prediction{Origin: "China", Confidence: 0.8}, precs()) {
				t.Error("Review does not send RenderReview") // promptsnap hashes RenderReview
			}
		})
	}
}

func TestReviewConflictLeans(t *testing.T) {
	for _, tt := range []struct{ leans, want string }{{`"b"`, "b"}, {`"a"`, "a"}, {`null`, ""}, {`"c"`, ""}} {
		text := `{"on_point":false,"closest_ruling":null,"on_point_reason":"x","conflict":{"ruling_a":"N351809","holding_a":"China","ruling_b":"N352485","holding_b":"Thailand","deciding_difference":"where the media is made","leans":` + tt.leans + `},"questions":[]}`
		r, err := New(llm.FakeText(text), "m", 2000).Review(context.Background(), "facts", domain.Prediction{Origin: "China"}, precs())
		if err != nil || r.Conflict == nil {
			t.Fatalf("leans %s: %+v %v", tt.leans, r, err)
		}
		got := ""
		if r.Conflict.Leans != nil {
			got = *r.Conflict.Leans
		}
		if got != tt.want {
			t.Errorf("leans %s -> %q, want %q", tt.leans, got, tt.want)
		}
	}
}
