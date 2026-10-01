package mymemory_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"trans/internal/mymemory"
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

const translated = `{"responseData":{"translatedText":"Please fix the failing test"},"responseStatus":200}`

func TestTranslateReturnsTheEnglishTextForTheDraft(t *testing.T) {
	t.Parallel()
	server, captured := serverReturning(t, http.StatusOK, translated)
	client := mymemory.New(mymemory.WithEndpoint(server.URL))

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

// The service wants both languages in one parameter, and the source is left to
// it: the panel does not know what the author is writing in.
func TestTheLanguagePairIsSentWithAnAutodetectedSource(t *testing.T) {
	t.Parallel()
	server, captured := serverReturning(t, http.StatusOK, translated)
	client := mymemory.New(
		mymemory.WithEndpoint(server.URL),
		mymemory.WithTargetLanguage("EN-US"),
	)

	if _, err := client.Translate(context.Background(), "Bitte behebe den Test"); err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	if pair := captured.query.Get("langpair"); pair != "Autodetect|en" {
		t.Errorf("request asked for %q, want the source detected and the plain target code", pair)
	}
}

func TestAnAnswerWithAServiceErrorIsRefused(t *testing.T) {
	t.Parallel()
	server, _ := serverReturning(t, http.StatusOK,
		`{"responseData":{"translatedText":""},"responseStatus":403}`)
	client := mymemory.New(mymemory.WithEndpoint(server.URL))

	translated, err := client.Translate(context.Background(), "Bitte behebe es")
	if err == nil {
		t.Fatal("an answer without a translation was taken as one:", translated)
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error is %v, want it to say what the service answered", err)
	}
}

// The service escapes entities even for plain text sent to it.
func TestEntitiesInTheAnswerAreUnescaped(t *testing.T) {
	t.Parallel()
	server, _ := serverReturning(t, http.StatusOK,
		`{"responseData":{"translatedText":"if (a &amp;&amp; b) { fix(); }"},"responseStatus":200}`)
	client := mymemory.New(mymemory.WithEndpoint(server.URL))

	english, err := client.Translate(context.Background(), "wenn (a und b) repariere")
	if err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	if english != "if (a && b) { fix(); }" {
		t.Errorf("Translate returned %q, want the entities decoded", english)
	}
}

func TestALongDraftIsSentInPiecesAndJoined(t *testing.T) {
	t.Parallel()
	server, _ := serverReturning(t, http.StatusOK, translated)
	client := mymemory.New(mymemory.WithEndpoint(server.URL))

	english, err := client.Translate(context.Background(), strings.Repeat("a", 1000))
	if err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	if len(english) != 3*len("Please fix the failing test") {
		t.Errorf("Translate returned %q, want three answers joined", english)
	}
}

// A 429 is MyMemory asking to be asked again rather than a refusal of the
// draft, and the second ask is the one that carries the translation. A free
// endpoint is the one most likely to be throttled, so it is the one that most
// needs the retry.
func TestARateLimitIsAskedAgainRatherThanReported(t *testing.T) {
	t.Parallel()
	asked := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		asked++
		w.Header().Set("Content-Type", "application/json")
		if asked == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"responseStatus":429,"responseDetails":"MYMEMORY WARNING: YOU USED ALL AVAILABLE FREE TRANSLATIONS FOR TODAY"}`)
			return
		}
		_, _ = io.WriteString(w, translated)
	}))
	t.Cleanup(server.Close)

	english, err := mymemory.New(mymemory.WithEndpoint(server.URL)).
		Translate(context.Background(), "Bitte behebe den fehlschlagenden Test")
	if err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	if english != "Please fix the failing test" {
		t.Errorf("Translate returned %q, want the translation from the second ask", english)
	}
	if asked != 2 {
		t.Errorf("the service was asked %d times, want the one retry a 429 earns", asked)
	}
}

// A throttle that does not lift carries the time it named, and the wait is left
// to the person: a popup is not a place to sit out thirty seconds.
func TestAPersistentRateLimitSaysWhenToComeBack(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, "MYMEMORY WARNING: YOU USED ALL AVAILABLE FREE TRANSLATIONS FOR TODAY")
	}))
	t.Cleanup(server.Close)

	started := time.Now()
	_, err := mymemory.New(mymemory.WithEndpoint(server.URL)).
		Translate(context.Background(), "Bitte behebe es")
	elapsed := time.Since(started)

	if err == nil {
		t.Fatal("Translate returned no error for a service that keeps throttling")
	}
	for _, wanted := range []string{"mymemory is rate limiting us", "30s", "MYMEMORY WARNING"} {
		if !strings.Contains(err.Error(), wanted) {
			t.Errorf("error %q does not say %q", err, wanted)
		}
	}
	if elapsed > 5*time.Second {
		t.Errorf("Translate waited %s, want the wait the service named left to the person", elapsed)
	}
}
