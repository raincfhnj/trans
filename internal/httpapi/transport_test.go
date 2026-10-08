package httpapi_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"trans/internal/httpapi"
)

// draft is the body an adapter sends: small, and the same bytes on every ask.
const draft = `{"text":"Bitte behebe den Test"}`

// reply is what the stand-in service answers one ask with. The last reply is
// repeated for any ask past the end of the list, which is how a service that
// never recovers is written down.
type reply struct {
	status  int
	body    string
	retryIn string
}

// service stands in for one of the five endpoints: it answers what it was told
// to answer, and keeps every request it was sent so a test can say how many
// times it was asked and what arrived.
type service struct {
	*httptest.Server

	asked []string
}

func newService(t *testing.T, replies ...reply) *service {
	t.Helper()
	standing := &service{}
	standing.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		standing.asked = append(standing.asked, string(raw))

		answering := replies[min(len(standing.asked), len(replies))-1]
		if answering.retryIn != "" {
			w.Header().Set("Retry-After", answering.retryIn)
		}
		w.WriteHeader(answering.status)
		_, _ = io.WriteString(w, answering.body)
	}))
	t.Cleanup(standing.Close)
	return standing
}

// draftRequest builds the kind of request an adapter makes: a POST with a JSON
// body that net/http knows how to send again.
func draftRequest(t *testing.T, ctx context.Context, url string) *http.Request {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(draft))
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	return request
}

// sends asks and closes whatever came back. A test that is about the failure
// has no use for a body, and a response beside the error would be a failure of
// the transport rather than of the test.
func sends(t *testing.T, transport *httpapi.Transport, request *http.Request) error {
	t.Helper()
	response, err := transport.Do(request)
	if response != nil {
		response.Body.Close()
	}
	return err
}

// An answer that arrives is asked for once, and its body is the caller's to
// read: every adapter decodes it.
func TestAnAnswerComesBackWithItsBodyStillReadable(t *testing.T) {
	t.Parallel()
	server := newService(t, reply{status: http.StatusOK, body: `{"translated":"hello"}`})
	var waited []time.Duration
	transport := httpapi.Watching(httpapi.New(), &waited)

	response, err := transport.Do(draftRequest(t, context.Background(), server.URL))
	if err != nil {
		t.Fatalf("Do returned unexpected error: %v", err)
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("reading the answer: %v", err)
	}
	if string(raw) != `{"translated":"hello"}` {
		t.Errorf("the answer was %q, want what the service said", raw)
	}
	if len(server.asked) != 1 {
		t.Errorf("the service was asked %d times, want once", len(server.asked))
	}
	if len(waited) != 0 {
		t.Errorf("waited %v, want no wait for an answer", waited)
	}
}

// The whole point of the package: a 429 is not a sentence about a status, it is
// a service asking to be asked again in a moment — and the moment is its to
// name.
func TestARateLimitIsAskedAgainWhenTheServiceNamesTheWait(t *testing.T) {
	t.Parallel()
	server := newService(t,
		reply{status: http.StatusTooManyRequests, body: "quota exceeded", retryIn: "1"},
		reply{status: http.StatusOK, body: `{"translated":"hello"}`},
	)
	var waited []time.Duration
	transport := httpapi.Watching(httpapi.New(), &waited)

	response, err := transport.Do(draftRequest(t, context.Background(), server.URL))
	if err != nil {
		t.Fatalf("Do returned unexpected error: %v", err)
	}
	defer response.Body.Close()

	if len(server.asked) != 2 {
		t.Errorf("the service was asked %d times, want the retry and nothing more", len(server.asked))
	}
	if len(waited) != 1 || waited[0] != time.Second {
		t.Errorf("waited %v, want exactly the second the service named", waited)
	}
	// A retried request has to carry the same bytes: a retry that sent the
	// spent reader would arrive empty and be refused for another reason.
	if len(server.asked) == 2 && server.asked[0] != draft {
		t.Errorf("the first ask carried %q, want the draft", server.asked[0])
	}
	if len(server.asked) == 2 && server.asked[1] != draft {
		t.Errorf("the retry carried %q, want the draft again", server.asked[1])
	}
	answered, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("reading the answer: %v", err)
	}
	if string(answered) != `{"translated":"hello"}` {
		t.Errorf("the retry answered %q, want the translation", answered)
	}
}

// A service that would rather name a moment than a number of seconds names a
// date instead, and that is honoured the same way.
func TestARetryAfterDateIsHonoured(t *testing.T) {
	t.Parallel()
	server := newService(t,
		reply{
			status:  http.StatusTooManyRequests,
			body:    "slow down",
			retryIn: time.Now().Add(2 * time.Second).UTC().Format(http.TimeFormat),
		},
		reply{status: http.StatusOK, body: `{"translated":"hello"}`},
	)
	var waited []time.Duration
	transport := httpapi.Watching(httpapi.New(), &waited)

	if err := sends(t, transport, draftRequest(t, context.Background(), server.URL)); err != nil {
		t.Fatalf("Do returned unexpected error: %v", err)
	}

	if len(waited) != 1 {
		t.Fatalf("waited %v, want one wait", waited)
	}
	// The date is written to the second, so what is left of the two is a
	// little over one; the backoff this client would have taken on its own is
	// a quarter of that at the very most.
	if waited[0] < 500*time.Millisecond || waited[0] > 2*time.Second {
		t.Errorf("waited %s, want the wait the service named as a date", waited[0])
	}
}

// A service that is unwell is worth asking again, but not forever: once the
// attempts are spent, the answer that kept coming back is what the error says.
func TestAServiceThatStaysUnwellIsGivenUpOnAfterTheCap(t *testing.T) {
	t.Parallel()
	server := newService(t, reply{status: http.StatusInternalServerError, body: "we are down"})
	var waited []time.Duration
	transport := httpapi.Watching(httpapi.New(), &waited)

	err := sends(t, transport, draftRequest(t, context.Background(), server.URL))

	var down *httpapi.Unavailable
	if !errors.As(err, &down) {
		t.Fatalf("Do returned %v, want the service called unavailable", err)
	}
	if !errors.Is(err, httpapi.ErrUnavailable) {
		t.Error("the error does not answer errors.Is for ErrUnavailable")
	}
	if down.Status != http.StatusInternalServerError || down.Body != "we are down" {
		t.Errorf("the error carries %d %q, want the status and what the service said",
			down.Status, down.Body)
	}
	if len(server.asked) != httpapi.Attempts {
		t.Errorf("the service was asked %d times, want the %d attempts",
			len(server.asked), httpapi.Attempts)
	}
	if len(waited) != httpapi.Attempts-1 {
		t.Fatalf("waited %v, want one wait before each retry", waited)
	}
	// The backoff grows: a service that has been down for three answers is not
	// asked as eagerly as one that has just refused once.
	for attempt := 1; attempt < len(waited); attempt++ {
		if waited[attempt] < waited[attempt-1] {
			t.Errorf("the waits are %v, want each at least as long as the one before", waited)
			break
		}
	}
}

// A server error that asking again would not change — a 501 is a service saying
// it will never do this — is still the same kind of trouble, without a retry.
func TestAServerErrorThatAskingAgainWouldNotChangeIsNotAskedAgain(t *testing.T) {
	t.Parallel()
	server := newService(t, reply{status: http.StatusNotImplemented, body: "not implemented"})
	var waited []time.Duration
	transport := httpapi.Watching(httpapi.New(), &waited)

	err := sends(t, transport, draftRequest(t, context.Background(), server.URL))

	if !errors.Is(err, httpapi.ErrUnavailable) {
		t.Errorf("Do returned %v, want the service called unavailable", err)
	}
	if len(server.asked) != 1 {
		t.Errorf("the service was asked %d times, want a 501 left alone", len(server.asked))
	}
	if len(waited) != 0 {
		t.Errorf("waited %v, want nothing for a status that will not change", waited)
	}
}

// A draft the service will not take is not a service that is unwell: asking
// again would be refused again, and how to say so is the caller's business.
func TestARefusalIsLeftToTheCallerAndNotAskedAgain(t *testing.T) {
	t.Parallel()
	server := newService(t, reply{status: http.StatusBadRequest, body: "the draft is not text"})
	var waited []time.Duration
	transport := httpapi.Watching(httpapi.New(), &waited)

	response, err := transport.Do(draftRequest(t, context.Background(), server.URL))
	if err != nil {
		t.Fatalf("Do returned unexpected error: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusBadRequest {
		t.Errorf("the answer came back as %d, want the status the service sent", response.StatusCode)
	}
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("reading the refusal: %v", err)
	}
	if string(raw) != "the draft is not text" {
		t.Errorf("the refusal came back as %q, want what the service said", raw)
	}
	if len(server.asked) != 1 {
		t.Errorf("the service was asked %d times, want a 400 left alone", len(server.asked))
	}
	if len(waited) != 0 {
		t.Errorf("waited %v, want nothing for a refusal", waited)
	}
}

// A key that was refused will be refused again on the second ask, so the kind
// comes back at once — with the service's own words in it, put there by the
// explainer the adapter handed over, since only the adapter knows the shape its
// service complains in.
func TestARefusedCredentialIsNotAskedAgainAndKeepsWhatTheServiceSaid(t *testing.T) {
	t.Parallel()
	server := newService(t, reply{
		status: http.StatusForbidden,
		body:   `{"message":"Authorization failed"}`,
	})
	var (
		waited []time.Duration
		seen   string
	)
	transport := httpapi.Watching(httpapi.New(httpapi.WithExplainer(func(body io.Reader) string {
		raw, _ := io.ReadAll(body)
		seen = string(raw)
		return "Authorization failed"
	})), &waited)

	err := sends(t, transport, draftRequest(t, context.Background(), server.URL))

	var refused *httpapi.Auth
	if !errors.As(err, &refused) {
		t.Fatalf("Do returned %v, want the credentials called refused", err)
	}
	if !errors.Is(err, httpapi.ErrAuth) {
		t.Error("the error does not answer errors.Is for ErrAuth")
	}
	if refused.Body != "Authorization failed" {
		t.Errorf("the error says %q, want what the explainer made of the body", refused.Body)
	}
	if seen != `{"message":"Authorization failed"}` {
		t.Errorf("the explainer was given %q, want the body the service refused with", seen)
	}
	if len(server.asked) != 1 {
		t.Errorf("the service was asked %d times, want a refused key left alone", len(server.asked))
	}
	if len(waited) != 0 {
		t.Errorf("waited %v, want nothing before a refusal of the credentials", waited)
	}
}

// A rate limit that names a longer wait than anyone will hold a popup open is
// not waited out. The answer comes back at once and carries the time, so a
// sentence can tell the person to come back in a minute instead of leaving them
// watching a panel that looks frozen.
func TestARateLimitThatOutlastsTheBudgetIsNotWaitedOut(t *testing.T) {
	t.Parallel()
	server := newService(t, reply{
		status:  http.StatusTooManyRequests,
		body:    "quota exceeded",
		retryIn: "60",
	})
	// The real wait, which must never be entered: a minute of it is a minute
	// the test would spend, too.
	transport := httpapi.New()

	started := time.Now()
	err := sends(t, transport, draftRequest(t, context.Background(), server.URL))
	elapsed := time.Since(started)

	var limited *httpapi.RateLimited
	if !errors.As(err, &limited) {
		t.Fatalf("Do returned %v, want the service called rate limited", err)
	}
	if !errors.Is(err, httpapi.ErrRateLimited) {
		t.Error("the error does not answer errors.Is for ErrRateLimited")
	}
	if limited.RetryAfter != time.Minute {
		t.Errorf("the error carries a wait of %s, want the minute the service named", limited.RetryAfter)
	}
	if limited.Status != http.StatusTooManyRequests || limited.Body != "quota exceeded" {
		t.Errorf("the error carries %d %q, want the status and what the service said",
			limited.Status, limited.Body)
	}
	if elapsed > time.Second {
		t.Errorf("Do took %s, want the minute it was asked for left unwaited", elapsed)
	}
	if len(server.asked) != 1 {
		t.Errorf("the service was asked %d times, want the one ask", len(server.asked))
	}
}

// A preview cancelled on the next keystroke must come back at once, not after
// the wait the last answer asked for: the wait is a select on the context, and
// this is the test that uses a real clock to say so.
func TestACancelledPreviewDoesNotWaitOutTheBackoff(t *testing.T) {
	t.Parallel()
	asked := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "3")
		w.WriteHeader(http.StatusTooManyRequests)
		select {
		case asked <- struct{}{}:
		default:
		}
	}))
	t.Cleanup(server.Close)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	transport := httpapi.New(httpapi.WithTimeout(time.Second))
	go func() {
		<-asked
		// By now the transport is in the three seconds the service asked for.
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	started := time.Now()
	err := sends(t, transport, draftRequest(t, ctx, server.URL))
	elapsed := time.Since(started)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Do returned %v, want the cancellation the caller asked for", err)
	}
	if elapsed > time.Second {
		t.Errorf("Do took %s to come back after the cancellation, want it at once", elapsed)
	}
}

// A connection nothing is listening on is not worth asking again — it will be
// refused just as fast the next time — but it is still a failure of the
// network rather than of the service, and a caller has to be able to say so.
func TestAServiceThatCannotBeReachedIsANetworkFailure(t *testing.T) {
	t.Parallel()
	closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	address := closed.URL
	closed.Close()
	var waited []time.Duration
	transport := httpapi.Watching(httpapi.New(), &waited)

	err := sends(t, transport, draftRequest(t, context.Background(), address))

	var unreachable *httpapi.Network
	if !errors.As(err, &unreachable) {
		t.Fatalf("Do returned %v, want a network failure", err)
	}
	if !errors.Is(err, httpapi.ErrNetwork) {
		t.Error("the error does not answer errors.Is for ErrNetwork")
	}
	if unreachable.Cause == nil {
		t.Error("the network failure kept no cause, so a log could not say what went wrong")
	}
	if unreachable.Timeout() {
		t.Error("a refused connection was reported as a service that took too long")
	}
	if len(waited) != 0 {
		t.Errorf("waited %v, want a refused connection left alone", waited)
	}
}

// A connection that breaks while it is being read from is worth another ask: a
// service may have restarted between one preview and the next. The listener
// here takes the connection, reads part of the request, and then drops it with
// a reset — the failure under test is a conversation cut off mid-flight, not a
// dial that never got anywhere, and reading first is what makes that the only
// thing this can look like from the client end.
func TestAConnectionThatBreaksMidAnswerIsAskedAgain(t *testing.T) {
	t.Parallel()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	accepting := make(chan struct{})
	go func() {
		close(accepting)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			// Read what the client sent before going: a reset that arrives
			// before any of the request has been read can surface as a dial
			// failure on some platforms, which is not the break under test.
			_, _ = io.ReadAtLeast(conn, make([]byte, 1), 1)
			// Linger zero makes the close a reset rather than a polite end.
			if tcp, ok := conn.(*net.TCPConn); ok {
				_ = tcp.SetLinger(0)
			}
			conn.Close()
		}
	}()
	<-accepting

	var waited []time.Duration
	transport := httpapi.Watching(httpapi.New(httpapi.WithTimeout(time.Second)), &waited)

	err = sends(t, transport, draftRequest(t, context.Background(), "http://"+listener.Addr().String()+"/"))

	if !errors.Is(err, httpapi.ErrNetwork) {
		t.Errorf("Do returned %v, want a network failure", err)
	}
	if len(waited) != httpapi.Attempts-1 {
		t.Errorf("waited %v, want a broken connection asked again up to the attempt cap", waited)
	}
}

// A service that took too long is a timeout and not a service that is unwell,
// which is the difference between the two sentences a person can read.
func TestAServiceThatTookTooLongIsATimeout(t *testing.T) {
	t.Parallel()
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(slow.Close)

	var waited []time.Duration
	transport := httpapi.Watching(httpapi.New(httpapi.WithTimeout(40*time.Millisecond)), &waited)

	err := sends(t, transport, draftRequest(t, context.Background(), slow.URL))

	var unreachable *httpapi.Network
	if !errors.As(err, &unreachable) {
		t.Fatalf("Do returned %v, want a network failure", err)
	}
	if !unreachable.Timeout() || !os.IsTimeout(err) {
		t.Error("an attempt that ran out of time was not reported as a timeout")
	}
}

// A body that cannot be sent twice is not sent twice. This package will not
// read a caller's stream into memory to find out how long it is, and it will
// not send a reader that has already been spent; such a request is sent once
// and its answer reported exactly as it came.
func TestARequestWhoseBodyCannotBeReplayedIsSentOnce(t *testing.T) {
	t.Parallel()
	server := newService(t,
		reply{status: http.StatusTooManyRequests, body: "quota exceeded", retryIn: "1"},
		reply{status: http.StatusOK, body: `{"translated":"hello"}`},
	)
	var waited []time.Duration
	transport := httpapi.Watching(httpapi.New(), &waited)

	// A caller's own stream gets no GetBody from net/http, which is the case
	// being tested; the readers the adapters use all get one.
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL,
		io.NopCloser(strings.NewReader(draft)))
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	if request.GetBody != nil {
		t.Fatal("the request was given a GetBody, so there is nothing under test")
	}

	err = sends(t, transport, request)

	if !errors.Is(err, httpapi.ErrRateLimited) {
		t.Errorf("Do returned %v, want the rate limit reported as it came", err)
	}
	if len(server.asked) != 1 {
		t.Errorf("the service was asked %d times, want a body that cannot be replayed sent once",
			len(server.asked))
	}
	if len(waited) != 0 {
		t.Errorf("waited %v, want nothing before a request that is not repeated", waited)
	}
}
