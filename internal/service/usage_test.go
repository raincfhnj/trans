package service

import (
	"context"
	"errors"
	"testing"

	"trans/internal/promptflow"
	"trans/internal/translation"
)

// keepingCount is a service that reports what it has spent, in the vocabulary
// of the package that owns services.
type keepingCount struct {
	used, limit int64
	err         error
}

func (keepingCount) Translate(context.Context, string) (string, error) { return "EN", nil }

func (k keepingCount) Usage(context.Context) (translation.Usage, error) {
	return translation.Usage{Used: k.used, Limit: k.limit}, k.err
}

// silent is a service that keeps no count: the free endpoints do not, and the
// popup shows no allowance rather than a zero.
type silent struct{}

func (silent) Translate(context.Context, string) (string, error) { return "EN", nil }

// wrapper stands in for anything that decorates a translator — the code
// protector, the sentence cache — so the search for a reporter behind it can be
// checked without building one.
type wrapper struct{ inner translation.Translator }

func (w wrapper) Translate(ctx context.Context, text string) (string, error) {
	return w.inner.Translate(ctx, text)
}

func (w wrapper) Unwrap() translation.Translator { return w.inner }

// The count a service keeps is handed to the popup in the popup's own words:
// both vocabularies meet here and nowhere else.
func TestTheReporterHandsTheCountOverInThePopupsWords(t *testing.T) {
	t.Parallel()

	reporter, ok := UsageReporter(keepingCount{used: 4321, limit: 1_000_000})
	if !ok {
		t.Fatal("a service that keeps count was not found")
	}

	spent, err := reporter.Usage(context.Background())
	if err != nil {
		t.Fatalf("Usage returned unexpected error: %v", err)
	}
	if spent != (promptflow.Usage{Used: 4321, Limit: 1_000_000}) {
		t.Errorf("Usage answered %+v, want the count the service reported", spent)
	}
}

// A service that cannot say what it spent fails the same way it would fail a
// translation: the count is asked for, not required, so the error is passed on
// rather than swallowed into a zero.
func TestAReporterThatFailsSaysSo(t *testing.T) {
	t.Parallel()

	refused := errors.New("the service is not answering")
	reporter, ok := UsageReporter(keepingCount{err: refused})
	if !ok {
		t.Fatal("a service that keeps count was not found")
	}

	if _, err := reporter.Usage(context.Background()); !errors.Is(err, refused) {
		t.Errorf("Usage returned %v, want the service's own failure", err)
	}
}

// Nothing to ask is answered rather than faked: a service with no count hands
// back no reporter, which is what keeps the header from claiming an allowance.
func TestAServiceWithNoCountHasNoReporter(t *testing.T) {
	t.Parallel()

	if reporter, ok := UsageReporter(silent{}); ok || reporter != nil {
		t.Errorf("a service with no count answered a reporter: %v, %v", reporter, ok)
	}
}

// The count is looked for through whatever wraps the translator, so a service
// behind the cache still reports its allowance.
func TestTheCountIsFoundBehindAWrapper(t *testing.T) {
	t.Parallel()

	reporter, ok := UsageReporter(wrapper{inner: keepingCount{used: 12, limit: 100}})
	if !ok {
		t.Fatal("the reporter behind a wrapper was not found")
	}

	spent, err := reporter.Usage(context.Background())
	if err != nil {
		t.Fatalf("Usage returned unexpected error: %v", err)
	}
	if spent.Used != 12 || spent.Limit != 100 {
		t.Errorf("Usage answered %+v, want the count behind the wrapper", spent)
	}
}
