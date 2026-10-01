package httpapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"time"
)

// The sentinel errors say what kind of failure a call met, so a caller can
// decide what to do without reading a sentence and guessing. Everything this
// package turns into an error answers errors.Is for exactly one of them.
var (
	// ErrAuth says the service refused the credentials: a 401 or a 403. Asking
	// again will not help, and nothing will until the key is right, so the call
	// comes back on the first answer.
	ErrAuth = errors.New("the service refused the credentials")

	// ErrRateLimited says the service is throttling us: a 429 that was still a
	// 429 after every attempt this client was allowed.
	ErrRateLimited = errors.New("the service is rate limiting us")

	// ErrUnavailable says the service is unwell: a 5xx that asking again did
	// not fix, or one that asking again would not change.
	ErrUnavailable = errors.New("the service is not answering right now")

	// ErrNetwork says no answer came at all: the connection could not be made,
	// or it broke before the service spoke.
	ErrNetwork = errors.New("the service could not be reached")
)

// Auth is what comes back when the service refused the credentials. The body is
// kept so a sentence can still say what the service said about it.
type Auth struct {
	// Status is the code the service answered, 401 or 403 in practice.
	Status int
	// Body is what the service said, trimmed and no longer than the adapters
	// ever kept.
	Body string
}

func (e *Auth) Error() string { return statusLine(e.Status) + ": " + e.Body }

// Unwrap is what lets errors.Is find ErrAuth rather than make every caller
// match on the type.
func (e *Auth) Unwrap() error { return ErrAuth }

// RateLimited is what comes back when the service is throttling us, and it
// carries the one thing a class cannot: how long the service asked us to wait.
type RateLimited struct {
	// Status is the code the service answered, 429 in practice.
	Status int
	// Body is what the service said, trimmed and no longer than the adapters
	// ever kept.
	Body string
	// RetryAfter is what the service named, and zero when it named nothing.
	RetryAfter time.Duration
}

func (e *RateLimited) Error() string { return statusLine(e.Status) + ": " + e.Body }

func (e *RateLimited) Unwrap() error { return ErrRateLimited }

// Unavailable is what comes back when the service answered a 5xx: either every
// attempt was spent on it, or the status is one that asking again would not
// change.
type Unavailable struct {
	// Status is the code the service answered, 5xx in practice.
	Status int
	// Body is what the service said, trimmed and no longer than the adapters
	// ever kept.
	Body string
	// RetryAfter is what the service named, and zero when it named nothing. A
	// 503 is as entitled to say when to come back as a 429 is.
	RetryAfter time.Duration
}

func (e *Unavailable) Error() string { return statusLine(e.Status) + ": " + e.Body }

func (e *Unavailable) Unwrap() error { return ErrUnavailable }

// Network is what comes back when no answer arrived at all. The error the
// transport gave is kept with it: that is the only thing that tells a refused
// connection from a certificate that has expired, and whoever is reading a log
// will want it, while the sentence the reader sees leaves it out.
type Network struct {
	Cause error
}

func (e *Network) Error() string {
	return "the service could not be reached: " + e.Cause.Error()
}

// Unwrap answers with both the class and what the transport complained about,
// so errors.Is finds ErrNetwork and errors.As still reaches the cause.
func (e *Network) Unwrap() []error {
	return []error{ErrNetwork, e.Cause}
}

// Timeout reports whether the service took too long rather than refused. It is
// how os.IsTimeout asks, and it is what lets translation.Trouble tell a slow
// service from one that is not there.
func (e *Network) Timeout() bool {
	if errors.Is(e.Cause, context.DeadlineExceeded) {
		return true
	}

	var slow net.Error
	return errors.As(e.Cause, &slow) && slow.Timeout()
}

// statusLine says a status the way a person reads it: the number, and the words
// the standard library has for it.
func statusLine(status int) string {
	text := http.StatusText(status)
	if text == "" {
		return strconv.Itoa(status)
	}
	return strconv.Itoa(status) + " " + text
}
