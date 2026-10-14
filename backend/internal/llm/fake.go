package llm

import (
	"context"
	"fmt"
	"sync"
)

// Fake is a scripted Model for tests. Respond decides each answer; Calls
// records every request.
type Fake struct {
	Respond func(req Request) (Response, error)

	mu    sync.Mutex
	calls []Request
}

// FakeText returns a Fake that always answers with text.
func FakeText(text string) *Fake {
	return &Fake{Respond: func(Request) (Response, error) { return Response{Text: text}, nil }}
}

// Provider implements Model.
func (f *Fake) Provider() string { return "fake" }

// Generate implements Model.
func (f *Fake) Generate(ctx context.Context, req Request) (Response, error) {
	f.mu.Lock()
	f.calls = append(f.calls, req)
	f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}
	if f.Respond == nil {
		return Response{}, fmt.Errorf("fake: no Respond func")
	}
	resp, err := f.Respond(req)
	if resp.ModelID == "" {
		resp.ModelID = req.Model
	}
	resp.Provider = "fake"
	return resp, err
}

// Calls returns a copy of the requests received so far.
func (f *Fake) Calls() []Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Request(nil), f.calls...)
}
