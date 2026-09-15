package openai_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"trans/internal/openai"
	"trans/internal/translation"
)

func optionsWithEndpoint(endpoint string) translation.Options {
	return translation.Options{APIKey: "key-123", Endpoint: endpoint}
}

type capturedRequest struct {
	path          string
	authorization string
	body          map[string]any
}

func serverReturning(t *testing.T, status int, response string) (*httptest.Server, *capturedRequest) {
	t.Helper()
	captured := &capturedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.path = r.URL.Path
		captured.authorization = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &captured.body)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, response)
	}))
	t.Cleanup(server.Close)
	return server, captured
}

const answered = `{"choices":[{"message":{"role":"assistant","content":"Please fix the failing test"}}]}`

func TestTranslateReturnsTheEnglishTextForTheDraft(t *testing.T) {
	t.Parallel()
	server, captured := serverReturning(t, http.StatusOK, answered)
	client := openai.New("key-123", openai.WithEndpoint(server.URL), openai.WithModel("a-model"))

	english, err := client.Translate(context.Background(), "Bitte behebe den fehlschlagenden Test")
	if err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	if english != "Please fix the failing test" {
		t.Errorf("Translate returned %q, want the translated text", english)
	}
	if captured.path != "/chat/completions" {
		t.Errorf("request went to %q, want the completions path on the base URL", captured.path)
	}
	if captured.body["model"] != "a-model" {
		t.Errorf("request asked for model %v, want the configured one", captured.body["model"])
	}
}

func TestTheDraftIsSentAsTheOnlyThingToTranslate(t *testing.T) {
	t.Parallel()
	server, captured := serverReturning(t, http.StatusOK, answered)
	client := openai.New("key-123", openai.WithEndpoint(server.URL),
		openai.WithTargetLanguage("de"))

	if _, err := client.Translate(context.Background(), "Please fix the test"); err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}

	messages, _ := captured.body["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("request sent %d messages, want an instruction and the draft", len(messages))
	}
	first, _ := messages[0].(map[string]any)
	second, _ := messages[1].(map[string]any)
	if first["role"] != "system" || !strings.Contains(first["content"].(string), "de") {
		t.Errorf("the instruction is %v, want it to name the target language", first)
	}
	if second["role"] != "user" || second["content"] != "Please fix the test" {
		t.Errorf("the draft went as %v, want it alone", second)
	}
}

// A sentence is translated with the one before it in mind, without that sentence
// being translated or billed again.
func TestTranslateWithContextPassesTheSurroundingTextAlong(t *testing.T) {
	t.Parallel()
	server, captured := serverReturning(t, http.StatusOK, answered)
	client := openai.New("key-123", openai.WithEndpoint(server.URL),
		openai.WithTargetLanguage("en"))

	_, err := client.TranslateWithContext(context.Background(), "Und dann?", "Bitte behebe den Test.")
	if err != nil {
		t.Fatalf("TranslateWithContext returned unexpected error: %v", err)
	}

	messages, _ := captured.body["messages"].([]any)
	instruction, _ := messages[0].(map[string]any)
	if !strings.Contains(instruction["content"].(string), "Bitte behebe den Test.") {
		t.Errorf("the instruction is %v, want it to carry what came before", instruction)
	}
}

func TestTheKeyTravelsAsABearerToken(t *testing.T) {
	t.Parallel()
	server, captured := serverReturning(t, http.StatusOK, answered)
	client := openai.New("key-123", openai.WithEndpoint(server.URL))

	if _, err := client.Translate(context.Background(), "Bitte behebe es"); err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	if captured.authorization != "Bearer key-123" {
		t.Errorf("request carried %q, want the key as a bearer token", captured.authorization)
	}
}

func TestARefusalIsReportedWithWhatTheServiceSaid(t *testing.T) {
	t.Parallel()
	server, _ := serverReturning(t, http.StatusUnauthorized,
		`{"error":{"message":"Incorrect API key provided"}}`)
	client := openai.New("wrong", openai.WithEndpoint(server.URL))

	translated, err := client.Translate(context.Background(), "Bitte behebe es")
	if err == nil {
		t.Fatal("a refused draft came back translated:", translated)
	}
	if !strings.Contains(err.Error(), "Incorrect API key provided") {
		t.Errorf("error is %v, want it to say what the service said", err)
	}
}

// A local model is reached over plain http, which is why the loopback address is
// allowed while anything else must be https.
func TestAnEndpointThatWouldSendTheKeyInClearIsRefused(t *testing.T) {
	t.Parallel()
	provider := openai.Provider{}

	_, err := provider.New(optionsWithEndpoint("http://example.com/v1"))
	if err == nil {
		t.Fatal("an insecure endpoint was accepted for a key")
	}
	if _, err := provider.New(optionsWithEndpoint("http://127.0.0.1:8080/v1")); err != nil {
		t.Errorf("a model on this machine was refused: %v", err)
	}
	if _, err := provider.New(optionsWithEndpoint("https://example.com/v1")); err != nil {
		t.Errorf("a secure endpoint was refused: %v", err)
	}
}
