package gtranslate_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"trans/internal/gtranslate"
)

type capturedRequest struct {
	query url.Values
	body  string
}

func serverReturning(t *testing.T, status int, response string) (*httptest.Server, *capturedRequest) {
	t.Helper()
	captured := &capturedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		captured.query, captured.body = r.URL.Query(), string(raw)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, response)
	}))
	t.Cleanup(server.Close)
	return server, captured
}

// The endpoint answers with one list per sentence, each a list of alternatives.
const translated = `[[["Please fix the failing test","Bitte behebe den fehlschlagenden Test",null,null,10]],null,"de"]`

func TestTranslateReturnsTheEnglishTextForTheDraft(t *testing.T) {
	t.Parallel()
	server, captured := serverReturning(t, http.StatusOK, translated)
	client := gtranslate.New(gtranslate.WithEndpoint(server.URL))

	english, err := client.Translate(context.Background(), "Bitte behebe den fehlschlagenden Test")
	if err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	if english != "Please fix the failing test" {
		t.Errorf("Translate returned %q, want the translated text", english)
	}
	if got := captured.query.Get("q"); got != "Bitte behebe den fehlschlagenden Test" {
		t.Errorf("request sent %q, want the draft", got)
	}
}

func TestTheDraftIsSentWithTheTargetLanguageAndAutoDetection(t *testing.T) {
	t.Parallel()
	server, captured := serverReturning(t, http.StatusOK, translated)
	client := gtranslate.New(
		gtranslate.WithEndpoint(server.URL),
		gtranslate.WithTargetLanguage("en-GB"),
	)

	if _, err := client.Translate(context.Background(), "Bitte behebe den Test"); err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	query := captured.query
	if query.Get("tl") != "en" {
		t.Errorf("request asked for %q, want the plain language code", query.Get("tl"))
	}
	if query.Get("sl") != "auto" {
		t.Errorf("request named the source as %q, want it detected", query.Get("sl"))
	}
}

// A translation that did not arrive must read like a sentence, not like a Go
// value, and it must not be mistaken for a translation. A 429 is the endpoint
// saying it is being asked too often, which is a sentence of its own.
func TestAServiceThatRefusesIsReportedInFull(t *testing.T) {
	t.Parallel()
	server, _ := serverReturning(t, http.StatusTooManyRequests, "quota exceeded")
	client := gtranslate.New(gtranslate.WithEndpoint(server.URL))

	translated, err := client.Translate(context.Background(), "Bitte behebe es")
	if err == nil {
		t.Fatal("a refused draft came back translated:", translated)
	}
	if !strings.Contains(err.Error(), "quota exceeded") {
		t.Errorf("error is %v, want it to say what the service said", err)
	}
	if !strings.Contains(err.Error(), "google is rate limiting us") {
		t.Errorf("error is %v, want it to say the endpoint is throttling", err)
	}
}

// A 429 is not the end of the draft: the endpoint asks to be asked again, and
// the second ask is the one that carries the translation. This is the endpoint
// most likely to be throttled, so it is the one that most needs the retry.
func TestARateLimitIsAskedAgainRatherThanReported(t *testing.T) {
	t.Parallel()
	asked := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		asked++
		w.Header().Set("Content-Type", "application/json")
		if asked == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, "quota exceeded")
			return
		}
		_, _ = io.WriteString(w, translated)
	}))
	t.Cleanup(server.Close)

	english, err := gtranslate.New(gtranslate.WithEndpoint(server.URL)).
		Translate(context.Background(), "Bitte behebe den fehlschlagenden Test")
	if err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	if english != "Please fix the failing test" {
		t.Errorf("Translate returned %q, want the translation from the second ask", english)
	}
	if asked != 2 {
		t.Errorf("the endpoint was asked %d times, want the one retry a 429 earns", asked)
	}
}

// A throttle that does not lift carries the time it named, and the wait is left
// to the person: a popup is not a place to sit out thirty seconds.
func TestAPersistentRateLimitSaysWhenToComeBack(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, "quota exceeded")
	}))
	t.Cleanup(server.Close)

	started := time.Now()
	_, err := gtranslate.New(gtranslate.WithEndpoint(server.URL)).
		Translate(context.Background(), "Bitte behebe es")
	elapsed := time.Since(started)

	if err == nil {
		t.Fatal("Translate returned no error for an endpoint that keeps throttling")
	}
	for _, wanted := range []string{"google is rate limiting us", "30s", "quota exceeded"} {
		if !strings.Contains(err.Error(), wanted) {
			t.Errorf("error %q does not say %q", err, wanted)
		}
	}
	if elapsed > 5*time.Second {
		t.Errorf("Translate waited %s, want the wait the endpoint named left to the person", elapsed)
	}
}

func TestAnAnswerWithoutATranslationIsRefused(t *testing.T) {
	t.Parallel()
	server, _ := serverReturning(t, http.StatusOK, `[[["","x",null,null,0]],null,"de"]`)
	client := gtranslate.New(gtranslate.WithEndpoint(server.URL))

	if translated, err := client.Translate(context.Background(), "Bitte behebe es"); err == nil {
		t.Fatal("an empty answer was taken as a translation:", translated)
	}
}

// A draft longer than the query string allows travels in pieces, and the pieces
// are put back together in order.
func TestALongDraftIsSentInPiecesAndJoined(t *testing.T) {
	t.Parallel()
	server, _ := serverReturning(t, http.StatusOK, `[[["x","y",null,null,0]],null,"de"]`)
	client := gtranslate.New(gtranslate.WithEndpoint(server.URL))

	english, err := client.Translate(context.Background(), strings.Repeat("a", 2500))
	if err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	if english != "xxx" {
		t.Errorf("Translate returned %q, want the three pieces joined", english)
	}
}
