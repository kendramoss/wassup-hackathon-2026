package spend

import (
	"context"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

func TestMeterCapsTheDayAndResets(t *testing.T) {
	s := NewMemory()
	m := New(s, 0.05)
	day := time.Date(2026, 10, 17, 23, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return day }
	ctx := context.Background()
	// gemini-4-argon promo: $2 in / $10 out per 1M; 10k in + 1k out = $0.03.
	f := &llm.Fake{Respond: func(llm.Request) (llm.Response, error) {
		return llm.Response{Text: "{}", Usage: llm.Usage{InputTokens: 10000, OutputTokens: 1000}}, nil
	}}
	model := m.Model(f)
	model.Generate(ctx, llm.Request{Model: "gemini-4-argon"})
	if over, total := m.Over(ctx); over || total != 0.03 {
		t.Fatalf("after one call: over %v, total %v", over, total)
	}
	model.Generate(ctx, llm.Request{Model: "gemini-4-argon"})
	if over, _ := m.Over(ctx); !over {
		t.Fatal("over the cap but not refused")
	}
	if got := m.UntilReset(); got != time.Hour {
		t.Errorf("until reset = %v, want 1h", got)
	}
	// Another instance sharing the store sees the same total.
	other := New(s, 0.05)
	other.now = m.now
	if over, _ := other.Over(ctx); !over {
		t.Error("a second instance does not see the shared total")
	}
	day = day.Add(2 * time.Hour) // next UTC day
	if over, total := m.Over(ctx); over || total != 0 {
		t.Errorf("new day: over %v, total %v", over, total)
	}
}

func TestNoCapNeverRefuses(t *testing.T) {
	m := New(NewMemory(), 0)
	m.Add(context.Background(), 1000)
	if over, _ := m.Over(context.Background()); over {
		t.Error("cap 0 refused")
	}
}
