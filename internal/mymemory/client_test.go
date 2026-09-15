package mymemory_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

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
