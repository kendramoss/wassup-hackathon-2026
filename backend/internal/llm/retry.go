package llm

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// WithRetry retries retryable failures (429, 5xx, transport errors) with
// exponential backoff. Bad responses and 4xx errors are returned at once.
func WithRetry(m Model, maxRetries int, baseDelay time.Duration) Model {
	return &retrying{next: m, maxRetries: maxRetries, base: baseDelay}
}

type retrying struct {
	next       Model
	maxRetries int
	base       time.Duration
}

func (r *retrying) Provider() string { return r.next.Provider() }

func (r *retrying) Generate(ctx context.Context, req Request) (Response, error) {
	var lastErr error
	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		if attempt > 0 {
			delay := r.base << (attempt - 1)
			select {
			case <-ctx.Done():
				return Response{}, fmt.Errorf("llm: retry %d: %w", attempt, ctx.Err())
			case <-time.After(delay):
			}
		}
		resp, err := r.next.Generate(ctx, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !retryable(err) || ctx.Err() != nil {
			return Response{}, err
		}
	}
	return Response{}, fmt.Errorf("llm: %w after %d attempts: %w", ErrUnavailable, r.maxRetries+1, lastErr)
}

func retryable(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Retryable()
	}
	return errors.Is(err, ErrUnavailable)
}

// Unavailable reports whether err means the provider was busy or down (a
// 429, a 5xx, a timeout, or retries exhausted), not that the request was bad.
func Unavailable(err error) bool {
	return retryable(err) || errors.Is(err, context.DeadlineExceeded)
}
