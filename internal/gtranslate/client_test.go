package gtranslate_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

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
// value, and it must not be mistaken for a translation.
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
