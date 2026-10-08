// Package httpapi is how this program asks a translation service for an answer:
// one client that asks again when asking again could plausibly help, that says
// which kind of failure it met rather than leaving a caller to read a status
// line, and that never keeps a popup waiting longer than a popup is worth.
package httpapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// defaultAttempts is the first ask and its three retries. A service that is
	// throttling, or that has lost a server for a moment, is normally back
	// within one of them; every attempt past that is time a person spends
	// watching a popup do nothing.
	defaultAttempts = 4

	// defaultBackoffBase is what the first wait starts at and what every wait
	// after it doubles from. It is short because the panel asks again on the
	// next keystroke anyway, and because the answer being waited for is
	// usually already on its way.
	defaultBackoffBase = 250 * time.Millisecond

	// defaultBackoffMaximum keeps the doubling from turning into a wait nobody
	// agreed to, whatever the attempt count becomes.
	defaultBackoffMaximum = 2 * time.Second

	// defaultRetryBudget bounds the whole call, measured from the first ask: an
	// attempt is only repeated when the wait before it fits in what is left. A
	// service that names a longer wait is not argued with — the error carries
	// what it asked for and the person decides whether to come back — and a
	// first attempt that took longer than this on its own is not followed by a
	// second, which is what keeps a slow service from being asked four times
	// over.
	defaultRetryBudget = 5 * time.Second

	// defaultTimeout bounds one attempt for an adapter that did not name its
	// own. A service that accepts a connection and then says nothing must not
	// hold a popup open for good.
	defaultTimeout = 15 * time.Second

	// defaultErrorBodyLimit is how much of a refusal is kept when an adapter
	// has not said otherwise. It matches the 2 KiB the adapters read
	// themselves: a complaint is a sentence, and a body is not a translation.
	defaultErrorBodyLimit = 2 << 10

	// maxNamedWait is the longest wait believed from a Retry-After header. A
	// day is past any window a service really means, and believing an
	// arbitrarily large number of seconds risks arithmetic that overflows.
	maxNamedWait = 24 * time.Hour
)

// Explainer says in a few words what a service said when it refused, given the
// body it refused with. Every adapter already has one, because only it knows
// whether its service complains in DeepL's shape or in an OpenAI-compatible
// one; taking it as an option is how a sentence ends up carrying the words the
// service used instead of the braces around them.
type Explainer func(body io.Reader) string

// Transport asks one service, and asks it again when that could help. It keeps
// nothing between calls, so an adapter builds one and keeps it for as long as
// it lives.
type Transport struct {
	client *http.Client

	// explain turns a refusal body into words; a transport without one keeps
	// the body as it found it, trimmed.
	explain   Explainer
	bodyLimit int64

	attempts int
	backoff  time.Duration
	maximum  time.Duration
	budget   time.Duration

	// wait is the one place a transport spends time. It is a field rather than
	// a plain call so a test can take the waiting out and watch what the policy
	// asked for instead of sitting through it.
	wait func(ctx context.Context, delay time.Duration) error
}

type Option func(*Transport)

// WithTimeout bounds a single attempt. Every adapter names its own — a model on
// a slow machine is not the same wait as a translation service — and a zero
// leaves the attempt unbounded, which only a caller that cancels for itself
// should ask for.
func WithTimeout(timeout time.Duration) Option {
	return func(t *Transport) { t.client.Timeout = timeout }
}

// WithCheckRedirect installs the guard that keeps a credential from following a
// redirect out of https. Go carries the request headers to the same host when
// it is redirected, a downgrade to plain http included, so an adapter that
// sends a key wants this; it is not a default because a refusal is a policy
// each service states for itself.
func WithCheckRedirect(refuse func(request *http.Request, via []*http.Request) error) Option {
	return func(t *Transport) { t.client.CheckRedirect = refuse }
}

// WithErrorBodyLimit is how much of a refusal is kept to be explained. It is
// the adapter's own 2 KiB by default, so an endpoint cannot answer a call with
// a megabyte of prose and have it all arrive in a popup.
func WithErrorBodyLimit(limit int64) Option {
	return func(t *Transport) { t.bodyLimit = limit }
}

// WithExplainer is the service's own error shape, asked to say in a few words
// what a refusal body means.
func WithExplainer(explain Explainer) Option {
	return func(t *Transport) { t.explain = explain }
}

// shared is the one pool of connections every adapter asks through. The five
// services are asked in the same session, and a transport each would hold a set
// of idle connections each for nothing; the standard library's is cloned so the
// proxy settings and the dial, TLS and handshake timeouts stay the ones it
// ships with rather than this package's guesses.
var shared = func() http.RoundTripper {
	if standard, ok := http.DefaultTransport.(*http.Transport); ok {
		return standard.Clone()
	}
	// A host that has replaced the default with a round tripper of its own
	// keeps it: this package is not the place to overrule that.
	return http.DefaultTransport
}()

// New builds the client one adapter talks through.
func New(options ...Option) *Transport {
	transport := &Transport{
		client: &http.Client{
			Transport: shared,
			Timeout:   defaultTimeout,
		},
		bodyLimit: defaultErrorBodyLimit,
		attempts:  defaultAttempts,
		backoff:   defaultBackoffBase,
		maximum:   defaultBackoffMaximum,
		budget:    defaultRetryBudget,
		wait:      waitForContext,
	}
	for _, option := range options {
		option(transport)
	}
	return transport
}

// Do sends the request, and sends it again while that could help. An answer
// this package does not know what to make of comes back with its body unread,
// so the caller reads its own service's refusals as it always has; an answer it
// does know — a refusal of the credentials, a rate limit, a service that is
// unwell — comes back as an error of that kind, with the body read, capped and
// closed.
func (t *Transport) Do(request *http.Request) (*http.Response, error) {
	ctx := request.Context()
	// The budget runs from here rather than from the first retry: what a person
	// is waiting for is the whole call, and the first attempt is most of it.
	deadline := time.Now().Add(t.budget)

	attempts := 1
	if replayable(request) {
		attempts = t.attempts
	}

	var failure error
	for attempt := range attempts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if attempt > 0 {
			delay := t.delayFor(failure, attempt)
			if time.Now().Add(delay).After(deadline) {
				// Waiting would hold the popup open for longer than the person
				// watching it will wait for a translation, so the answer so far
				// — a rate limit that says how long, most likely — is the
				// answer.
				break
			}
			if err := t.wait(ctx, delay); err != nil {
				return nil, err
			}
		}

		response, err := t.send(request)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				// The caller cancelled. That is what happened, whatever the
				// transport was in the middle of at the time.
				return nil, ctxErr
			}
			if !fromService(err) {
				// Not the service's doing — a body that would not rewind — so
				// it is handed back as it is rather than dressed up as a
				// network problem.
				return nil, err
			}
			failure = &Network{Cause: err}
			if !transient(err) {
				return nil, failure
			}
			continue
		}
		if refusal := t.classify(response); refusal != nil {
			failure = refusal
			if !retryable(refusal) {
				return nil, failure
			}
			continue
		}
		return response, nil
	}
	return nil, failure
}

// send makes one attempt. The request is copied rather than reused because the
// client closes the body it is handed, and the copy is given a fresh reader from
// GetBody when there is one: that is how the same bytes are sent twice instead
// of a spent reader being sent as nothing at all.
func (t *Transport) send(request *http.Request) (*http.Response, error) {
	sending := request.Clone(request.Context())
	if request.GetBody != nil {
		body, err := request.GetBody()
		if err != nil {
			return nil, fmt.Errorf("rewinding the request body: %w", err)
		}
		sending.Body = body
	}
	//nolint:gosec // the URL is the endpoint the person configured for this service, which is the one thing this package exists to call
	return t.client.Do(sending)
}

// replayable says whether a request can be sent again at all. A body that
// carries a GetBody can: net/http sets one for the readers the adapters use —
// bytes, strings, and no body at all. A caller's own stream cannot, and this
// package will not read it into memory to find out how long it is, nor send a
// reader that has already been spent; such a request is sent once and its
// failure is reported exactly as it is.
func replayable(request *http.Request) bool {
	return request.Body == nil || request.Body == http.NoBody || request.GetBody != nil
}

// fromService says whether the failure came from the service being talked to.
// The standard library promises that anything http.Client itself fails with is
// a *url.Error, so an error that is not one was this package's own doing — a
// body that would not rewind — and is not a network problem.
func fromService(err error) bool {
	var failed *url.Error
	return errors.As(err, &failed)
}

// transient says whether asking again could plausibly help. A timeout could:
// the service may be busy rather than away. So could a connection that broke
// while it was being read from or written to, which is what a reset or a broken
// pipe is — a service may have restarted between one preview and the next. A
// connection that could not be made at all is not worth another ask: nothing is
// listening, it will be refused just as fast the next time, and that is what a
// wrong port looks like. Neither is a name that does not resolve.
func transient(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	if dns, ok := errors.AsType[*net.DNSError](err); ok {
		// A lookup that timed out may answer on the next ask; one that says the
		// name is not there will say so however many times it is asked. The
		// timeout half is recorded rather than exercised: the standard library
		// offers no resolver hook that fails a lookup with IsTimeout set, and
		// a test that waited for a real resolver to time out would be measuring
		// the network rather than this branch.
		return dns.IsTimeout
	}

	var slow net.Error
	if errors.As(err, &slow) && slow.Timeout() {
		return true
	}

	// What the operation was says which side of the connection failed. A dial
	// that never got anywhere is a service that is not there; a read or a write
	// that broke halfway is a conversation that can be started again — and the
	// failing operation is read from the error rather than the platform's errno
	// because the errno for a reset is not the one this program's own syscall
	// package names it with on Windows.
	var broken *net.OpError
	return errors.As(err, &broken) && (broken.Op == "read" || broken.Op == "write")
}

// classify turns the answers this package knows what to make of into errors,
// and leaves every other answer to the caller, which knows what its own service
// means by a 400 or a 404 and what to say about it.
func (t *Transport) classify(response *http.Response) error {
	switch {
	case response.StatusCode == http.StatusUnauthorized,
		response.StatusCode == http.StatusForbidden:
		return &Auth{Status: response.StatusCode, Body: t.complaint(response)}
	case response.StatusCode == http.StatusTooManyRequests:
		return &RateLimited{
			Status:     response.StatusCode,
			Body:       t.complaint(response),
			RetryAfter: retryAfter(response),
		}
	case response.StatusCode >= 500:
		return &Unavailable{
			Status:     response.StatusCode,
			Body:       t.complaint(response),
			RetryAfter: retryAfter(response),
		}
	}
	return nil
}

// retryable says whether the answer is one that asking again could change. A
// rate limit is, and so is a service that has lost a server for a moment; a
// service that refuses the credentials will refuse them again, and a status it
// will never answer — 501, 505 — is not worth a second ask.
func retryable(refusal error) bool {
	if _, ok := errors.AsType[*RateLimited](refusal); ok {
		return true
	}

	if down, ok := errors.AsType[*Unavailable](refusal); ok {
		switch down.Status {
		case http.StatusInternalServerError, http.StatusBadGateway,
			http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		}
	}
	return false
}

// retryAfter reads how long the service asked to be left alone, and says zero
// when it asked for nothing this client can read — no header, a header in
// neither shape, or a time that has already gone by. The header comes as a
// number of seconds, and — from a service that wants to be precise, or that
// sits behind something opinionated — as a date.
func retryAfter(response *http.Response) time.Duration {
	header := strings.TrimSpace(response.Header.Get("Retry-After"))
	if header == "" {
		return 0
	}

	if seconds, err := strconv.Atoi(header); err == nil {
		if seconds < 0 {
			return 0
		}
		if seconds > int(maxNamedWait/time.Second) {
			seconds = int(maxNamedWait / time.Second)
		}
		return time.Duration(seconds) * time.Second
	}

	moment, err := http.ParseTime(header)
	if err != nil {
		return 0
	}
	if wait := time.Until(moment); wait > 0 {
		return wait
	}
	// A date already past names no wait at all, and the backoff this client
	// takes anyway is the smallest wait it makes.
	return 0
}

// delayFor says how long to wait before asking again. The wait is the longer of
// what the service asked for and what this client would have waited on its own:
// answering a Retry-After with less than it asked for is how a client earns a
// harder throttle, and answering one with exactly nothing is how five
// cancelled previews come back as one burst.
func (t *Transport) delayFor(cause error, retry int) time.Duration {
	backoff := t.jittered(retry)
	if named := namedWait(cause); named > backoff {
		return named
	}
	return backoff
}

// namedWait is what the service asked for, and zero when it asked for nothing.
func namedWait(cause error) time.Duration {
	if limited, ok := errors.AsType[*RateLimited](cause); ok {
		return limited.RetryAfter
	}

	if down, ok := errors.AsType[*Unavailable](cause); ok {
		return down.RetryAfter
	}
	return 0
}

// jittered is the wait this client would take on its own: the base doubled once
// per retry already spent, capped, then halved and shaken. The shaking matters
// more than it looks — a panel cancels and re-asks for every keystroke, and
// without it the answers to a burst of previews would all come back together
// and be throttled together.
func (t *Transport) jittered(retry int) time.Duration {
	grown := t.backoff
	for range retry - 1 {
		if grown >= t.maximum {
			break
		}
		grown *= 2
	}
	if grown > t.maximum {
		grown = t.maximum
	}

	half := grown / 2
	if half <= 0 {
		return grown
	}
	//nolint:gosec // a wait is not a secret: a shake a watcher could predict is still a shake
	return half + rand.N(half)
}

// complaint reads what the service said about the refusal — no further than the
// limit its adapter set — and closes the body. Nobody else is going to read a
// classified answer, and a body left open keeps its connection out of the pool.
func (t *Transport) complaint(response *http.Response) string {
	defer response.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(response.Body, t.bodyLimit))
	if err != nil || len(raw) == 0 {
		return ""
	}
	if t.explain != nil {
		return strings.TrimSpace(t.explain(bytes.NewReader(raw)))
	}
	return strings.TrimSpace(string(raw))
}

// waitForContext is the wait every transport starts with: a select rather than a
// sleep, so a context cancelled while the wait is running — the panel cancels a
// preview on the next keystroke — comes back at once instead of after the wait
// the last answer asked for.
func waitForContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
