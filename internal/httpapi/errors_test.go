package httpapi_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"trans/internal/httpapi"
)

// What a reader sees when a service refused: the number, the words the standard
// library has for it, and what the service said about it. Each kind is one
// sentence, and that sentence is what the popup shows.
func TestEveryRefusalSaysTheStatusAndWhatTheServiceSaid(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		err   error
		wants []string
		class error
	}{
		{
			name:  "credentials refused",
			err:   &httpapi.Auth{Status: http.StatusUnauthorized, Body: "the key is not valid"},
			wants: []string{"401", "Unauthorized", "the key is not valid"},
			class: httpapi.ErrAuth,
		},
		{
			name:  "throttled",
			err:   &httpapi.RateLimited{Status: http.StatusTooManyRequests, Body: "slow down"},
			wants: []string{"429", "Too Many Requests", "slow down"},
			class: httpapi.ErrRateLimited,
		},
		{
			name:  "service unavailable",
			err:   &httpapi.Unavailable{Status: http.StatusServiceUnavailable, Body: "down for maintenance"},
			wants: []string{"503", "Service Unavailable", "down for maintenance"},
			class: httpapi.ErrUnavailable,
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			for _, want := range test.wants {
				if !strings.Contains(test.err.Error(), want) {
					t.Errorf("the sentence reads %q, want it to hold %q", test.err.Error(), want)
				}
			}
			if !errors.Is(test.err, test.class) {
				t.Errorf("errors.Is did not find the class of %T", test.err)
			}
		})
	}
}

// A status the standard library has no words for is still worth saying: the
// number is what a person looks up, and it must not be dropped for a blank.
func TestAStatusWithNoWordsIsStillNamedByItsNumber(t *testing.T) {
	t.Parallel()

	refusal := &httpapi.Auth{Status: 499, Body: "the client closed the request"}
	if !strings.Contains(refusal.Error(), "499") {
		t.Errorf("the sentence reads %q, want the number in it", refusal.Error())
	}
}

// A service that was never reached keeps the transport's own complaint next to
// the class, so a log can tell a refused connection from an expired
// certificate while the reader's sentence stays plain.
func TestAnUnreachableServiceKeepsWhatTheTransportSaid(t *testing.T) {
	t.Parallel()

	cause := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	unreached := &httpapi.Network{Cause: cause}

	if !errors.Is(unreached, httpapi.ErrNetwork) {
		t.Error("errors.Is did not find the class of an unreachable service")
	}
	var found *net.OpError
	if !errors.As(unreached, &found) {
		t.Error("errors.As could not reach what the transport complained about")
	}
	if !strings.Contains(unreached.Error(), "connection refused") {
		t.Errorf("the sentence reads %q, want the transport's complaint in it", unreached.Error())
	}
	if unreached.Timeout() {
		t.Error("a refused connection is reported as a timeout")
	}
}

// A deadline is a timeout rather than a service that is not there, and the
// sentence says which: one is worth waiting out, the other is not.
func TestADeadlineIsReportedAsATimeout(t *testing.T) {
	t.Parallel()

	if !(&httpapi.Network{Cause: context.DeadlineExceeded}).Timeout() {
		t.Error("a deadline was not reported as a timeout")
	}
}

// A redirect is answered rather than followed when the transport was told to
// refuse them: a key must not travel to whichever host a service points at,
// which is the whole reason the option exists.
func TestARedirectIsRefusedRatherThanFollowed(t *testing.T) {
	t.Parallel()

	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("the transport followed the redirect")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(elsewhere.Close)

	pointing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL, http.StatusFound)
	}))
	t.Cleanup(pointing.Close)

	transport := httpapi.New(httpapi.WithCheckRedirect(func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}))

	response, err := transport.Do(draftRequest(t, context.Background(), pointing.URL))
	if err != nil {
		t.Fatalf("Do returned unexpected error: %v", err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusFound {
		t.Errorf("the response is a %d, want the redirect handed back untouched", response.StatusCode)
	}
}

// What a service said about a refusal is kept, but not all of it: a service
// that answers a book with a 429 must not have the book held in memory for a
// sentence nobody reads.
func TestTheBodyKeptFromARefusalIsCapped(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("sehr lang ", 200)
	standing := newService(t, reply{status: http.StatusTooManyRequests, body: long})

	refused := sends(t, httpapi.New(httpapi.WithErrorBodyLimit(16)),
		draftRequest(t, context.Background(), standing.URL))

	var throttled *httpapi.RateLimited
	if !errors.As(refused, &throttled) {
		t.Fatalf("the failure is %v, want a throttled service", refused)
	}
	if len(throttled.Body) > 16 {
		t.Errorf("the body kept is %d bytes, want no more than the 16 it was given", len(throttled.Body))
	}
	if throttled.Body == "" {
		t.Error("nothing was kept of what the service said")
	}
}
