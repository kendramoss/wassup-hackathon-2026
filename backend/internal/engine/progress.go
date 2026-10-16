package engine

import "context"

// Stages Predict reports to a Progress in the context (streamed predict, D71).
const (
	// StagePrecedents is the retrieval of earlier rulings.
	StagePrecedents = "precedents"
	// StageEngine is the model call on facts plus precedents (and any votes).
	StageEngine = "engine"
)

// Progress is told when a stage starts (done false) and when it finishes.
// It only observes: the result is the same with or without it. A stage that
// fails reports no done.
type Progress func(stage string, done bool)

type progressKey struct{}

// WithProgress returns ctx carrying p, so Predict reports its stages to it.
// The engine sits behind the Predictor interface and the prediction cache,
// so the hook travels in the context, not the signature. A cached prediction
// reports nothing (no retrieval or model call happens).
func WithProgress(ctx context.Context, p Progress) context.Context {
	return context.WithValue(ctx, progressKey{}, p)
}

func report(ctx context.Context, stage string, done bool) {
	if p, ok := ctx.Value(progressKey{}).(Progress); ok && p != nil {
		p(stage, done)
	}
}
