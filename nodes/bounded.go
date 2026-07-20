package nodes

import (
	"context"
	"fmt"
	"time"
)

// computeBudget is the wall-clock ceiling for a single node invocation's
// algorithm. Every input bound in this package is calibrated to finish far
// inside it; the budget exists so that a bound which turns out to be
// mis-calibrated for some input shape degrades into a structured error instead
// of a hang. It sits below the platform's synchronous invocation budget so the
// caller gets this package's message rather than an opaque gateway timeout.
const computeBudget = 20 * time.Second

// runBounded runs fn on a worker goroutine and returns as soon as EITHER the
// work finishes, the caller's context is cancelled, or the budget expires.
//
// gonum's algorithms take no context and cannot be interrupted, so an abandoned
// computation does keep running to completion — this is what bounds the
// CALLER's wait, not the worker's. It is the backstop; the input bounds are the
// primary defence, and the two are meant to be read together.
func runBounded[T any](ctx context.Context, what string, fn func() T) (T, error) {
	var zero T
	done := make(chan T, 1) // buffered: an abandoned worker must never block
	go func() { done <- fn() }()

	timer := time.NewTimer(computeBudget)
	defer timer.Stop()

	select {
	case v := <-done:
		return v, nil
	case <-ctx.Done():
		return zero, fmt.Errorf("%s was cancelled: %w", what, ctx.Err())
	case <-timer.C:
		return zero, fmt.Errorf("%s exceeded the %s compute budget; reduce the size of the graph",
			what, computeBudget)
	}
}
