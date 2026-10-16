package engine

import (
	"context"
	"reflect"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

func TestPredictReportsStagesWithoutChangingTheResult(t *testing.T) {
	answer := `{"origin_country":"China","essential_character":"motor","decisive_step":"motor in China","flip":null,"confidence":0.97,"reasoning":"As N350001."}`
	c := domain.Case{ID: "case_1", Date: day("2026-01-14"), Facts: "A heat gun is assembled in Thailand from a Chinese motor and fan."}
	tests := []struct {
		name    string
		respond func(llm.Request) (llm.Response, error)
		want    []string
		wantErr bool
	}{
		{"retrieval then the model", func(llm.Request) (llm.Response, error) { return llm.Response{Text: answer}, nil },
			[]string{"precedents started", "precedents done", "engine started", "engine done"}, false},
		{"a failed model call reports no done", func(llm.Request) (llm.Response, error) { return llm.Response{}, llm.ErrUnavailable },
			[]string{"precedents started", "precedents done", "engine started"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			ctx := WithProgress(context.Background(), func(stage string, done bool) {
				s := stage + " started"
				if done {
					s = stage + " done"
				}
				got = append(got, s)
			})
			withHook, err := newEngine(&llm.Fake{Respond: tt.respond}).Predict(ctx, c)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("stages = %q, want %q", got, tt.want)
			}
			plain, _ := newEngine(&llm.Fake{Respond: tt.respond}).Predict(context.Background(), c)
			withHook.Prediction.Trace.LatencyMS, plain.Prediction.Trace.LatencyMS = 0, 0
			if !reflect.DeepEqual(withHook, plain) {
				t.Errorf("the hook changed the result:\n%+v\n%+v", withHook, plain)
			}
		})
	}
}
