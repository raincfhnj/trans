package httpapi

import (
	"context"
	"time"
)

// Attempts is how many times a retryable answer is asked for, so a test can
// assert the cap rather than repeat the number and drift from it.
const Attempts = defaultAttempts

// Watching takes the waiting out of a transport and collects what it would have
// waited for instead. A test of the retry policy must not spend the policy's
// time, and the wait itself is only ever a select on a context — the one thing
// about it that is worth a real clock is tested without this.
func Watching(t *Transport, waited *[]time.Duration) *Transport {
	t.wait = func(_ context.Context, delay time.Duration) error {
		*waited = append(*waited, delay)
		return nil
	}
	return t
}
