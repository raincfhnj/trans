// Package translation defines what a translation service must offer and keeps
// the registry of the services this build knows about.
package translation

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"trans/internal/httpapi"
)

type Translator interface {
	Translate(ctx context.Context, draft string) (string, error)
}

// Options is the superset of settings; each service takes what applies and
// rejects what it cannot work without.
type Options struct {
	APIKey         string
	TargetLanguage string
	Endpoint       string
	// Model names what a service that is a model rather than a translator should
	// answer with. Services that translate by themselves ignore it.
	Model string
	// Command is a shell command line that translates what it is given on its
	// input, for a service that is a program on the machine rather than a host.
	Command string
}

type Provider interface {
	Name() string
	New(Options) (Translator, error)
}

type Usage struct {
	Used  int64
	Limit int64
}

// Not every service keeps count, so this is asked for rather than required.
type UsageReporter interface {
	Usage(ctx context.Context) (Usage, error)
}

// ErrNoUsage says the service does not report what it has spent.
var ErrNoUsage = errors.New("the service keeps no count")

// ReporterOf looks through anything that wraps a translator, so a service behind
// a cache still reports its allowance.
func ReporterOf(translator Translator) (UsageReporter, error) {
	for translator != nil {
		if reporter, keepsCount := translator.(UsageReporter); keepsCount {
			return reporter, nil
		}

		wrapper, wraps := translator.(interface{ Unwrap() Translator })
		if !wraps {
			break
		}
		translator = wrapper.Unwrap()
	}
	return nil, ErrNoUsage
}

// Trouble says in a sentence what went wrong on the way to a service. A transport
// error carries a URL, a Go type and a method name, none of which help the person
// looking at a popup.
func Trouble(service string, err error) error {
	// withWords puts what the service said after the sentence, when it said
	// anything at all: the kind of failure is this program's word for it, but
	// the service's own words are often the only thing that names the fix.
	withWords := func(sentence, said string) error {
		if said == "" {
			return errors.New(sentence)
		}
		return errors.New(sentence + ": " + said)
	}

	var refused *httpapi.Auth
	var limited *httpapi.RateLimited
	var down *httpapi.Unavailable
	var unreachable *httpapi.Network
	var dns *net.DNSError
	var connecting *net.OpError

	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled):
		return err
	case errors.Is(err, context.DeadlineExceeded), os.IsTimeout(err):
		return fmt.Errorf("%s did not answer in time", service)
	case errors.As(err, &refused):
		return withWords(service+" refused the API key", refused.Body)
	case errors.As(err, &limited):
		// How long the service asked to be left alone is worth passing on: it
		// is the difference between asking again in a moment and leaving the
		// panel alone for a while.
		if limited.RetryAfter > 0 {
			return withWords(
				service+" is rate limiting us — try again in "+
					limited.RetryAfter.Round(100*time.Millisecond).String(),
				limited.Body)
		}
		return withWords(service+" is rate limiting us", limited.Body)
	case errors.As(err, &down):
		if down.RetryAfter > 0 {
			return withWords(
				service+" is not answering right now — try again in "+
					down.RetryAfter.Round(100*time.Millisecond).String(),
				down.Body)
		}
		return withWords(service+" is not answering right now", down.Body)
	case errors.As(err, &dns):
		return fmt.Errorf("%s could not be found — is there a network?", service)
	case errors.As(err, &connecting):
		return fmt.Errorf("%s could not be reached", service)
	case errors.As(err, &unreachable):
		return fmt.Errorf("%s could not be reached", service)
	default:
		return fmt.Errorf("%s could not be asked: %w", service, err)
	}
}
