package translation_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"trans/internal/httpapi"
	"trans/internal/translation"
)

// Trouble turns what a service said into a sentence a person can act on. Every
// kind gets its own words, and the service's own explanation is carried along
// when there is one: the kind is this program's word for the failure, but what
// the service said is often the only thing that names the fix.
func TestTroubleSaysWhatKindOfFailureItWas(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "nothing went wrong at all",
			err:  nil,
			want: "",
		},
		{
			name: "a preview that was abandoned is passed on as it is",
			err:  context.Canceled,
			want: context.Canceled.Error(),
		},
		{
			name: "a service that took too long",
			err:  context.DeadlineExceeded,
			want: "deepl did not answer in time",
		},
		{
			name: "a key that was refused, with what the service said about it",
			err:  &httpapi.Auth{Status: 403, Body: "Authorization failed"},
			want: "deepl refused the API key: Authorization failed",
		},
		{
			name: "throttling, with the wait the service named",
			err:  &httpapi.RateLimited{Status: 429, Body: "Too many requests", RetryAfter: 30 * time.Second},
			want: "deepl is rate limiting us — try again in 30s: Too many requests",
		},
		{
			name: "throttling that named no wait",
			err:  &httpapi.RateLimited{Status: 429, Body: "Too many requests"},
			want: "deepl is rate limiting us: Too many requests",
		},
		{
			name: "a service that is unwell, with the wait it named",
			err:  &httpapi.Unavailable{Status: 503, Body: "Service Unavailable", RetryAfter: 1500 * time.Millisecond},
			want: "deepl is not answering right now — try again in 1.5s: Service Unavailable",
		},
		{
			name: "a service that is unwell and said nothing",
			err:  &httpapi.Unavailable{Status: 500, Body: "no details"},
			want: "deepl is not answering right now: no details",
		},
		{
			name: "a host that does not exist",
			err:  &net.DNSError{Err: "no such host", Name: "api.example"},
			want: "deepl could not be found — is there a network?",
		},
		{
			name: "a connection that never opened",
			err:  &net.OpError{Op: "dial", Err: errors.New("connection refused")},
			want: "deepl could not be reached",
		},
		{
			name: "anything else keeps what went wrong",
			err:  errors.New("the endpoint answered something unexpected"),
			want: "deepl could not be asked: the endpoint answered something unexpected",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			trouble := translation.Trouble("deepl", test.err)
			switch {
			case test.want == "":
				if trouble != nil {
					t.Errorf("Trouble returned %v for no failure at all", trouble)
				}
			case trouble == nil:
				t.Errorf("Trouble returned nothing, want %q", test.want)
			case trouble.Error() != test.want:
				t.Errorf("Trouble returned %q, want %q", trouble, test.want)
			}
		})
	}
}

// A cancelled translation is the one failure that is not the author's to read
// about: it is this program deciding to stop, so the error is handed back as it
// came in — the same error, not one wrapped around it — and every caller that
// waits on a preview can recognise it with errors.Is.
func TestACancelledTranslationIsNotDressedUpAsATrouble(t *testing.T) {
	t.Parallel()

	wrapped := fmt.Errorf("asking deepl: %w", context.Canceled)
	trouble := translation.Trouble("deepl", wrapped)
	if !errors.Is(trouble, context.Canceled) {
		t.Errorf("Trouble returned %v, want the cancellation still findable in it", trouble)
	}
	if trouble.Error() != wrapped.Error() {
		t.Errorf("Trouble returned %q, want what it was given back unchanged (%q)",
			trouble, wrapped)
	}
}
