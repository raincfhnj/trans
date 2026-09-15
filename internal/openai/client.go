// Package openai translates through any service that speaks the OpenAI chat
// completions API: the one it is named after, and the many that are compatible
// with it — DeepSeek, Qwen, Moonshot, a gateway, or a model running on this
// machine behind the same shape. It is one service behind the translation ports,
// and knows nothing about the overlay.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"trans/internal/translation"
)

const (
	defaultEndpoint       = "https://api.openai.com/v1"
	defaultModel          = "gpt-4o-mini"
	defaultTargetLanguage = "en"

	// A model on a slow machine or a long draft takes longer than a translation
	// service does; a command that hangs must still not hold the popup for good.
	defaultTimeout = 60 * time.Second

	maxErrorBodyBytes = 2 << 10
	maxResponseBytes  = 1 << 20
	// A draft is sent as one message; anything larger is not a prompt.
	maxRequestBytes = 256 << 10
)

type Client struct {
	httpClient     *http.Client
	endpoint       string
	apiKey         string
	model          string
	targetLanguage string
}

type Option func(*Client)

func WithEndpoint(endpoint string) Option {
	return func(c *Client) { c.endpoint = endpoint }
}

func WithModel(model string) Option {
	return func(c *Client) { c.model = model }
}

func WithTargetLanguage(language string) Option {
	return func(c *Client) { c.targetLanguage = language }
}

func New(apiKey string, options ...Option) *Client {
	client := &Client{
		httpClient:     &http.Client{Timeout: defaultTimeout},
		endpoint:       defaultEndpoint,
		apiKey:         apiKey,
		model:          defaultModel,
		targetLanguage: defaultTargetLanguage,
	}
	for _, option := range options {
		option(client)
	}
	// After the options, so no way of building a client can leave the key free to
	// follow a redirect into the clear.
	client.httpClient.CheckRedirect = refuseInsecureRedirect
	return client
}

func (c *Client) Endpoint() string { return c.endpoint }

func (c *Client) Translate(ctx context.Context, draft string) (string, error) {
	return c.TranslateWithContext(ctx, draft, "")
}

func (c *Client) TranslateWithContext(ctx context.Context, draft, surrounding string) (string, error) {
	body, err := c.requestBody(draft, surrounding)
	if err != nil {
		return "", err
	}
	if len(body) > maxRequestBytes {
		return "", fmt.Errorf("the draft is %d bytes, more than the %d the service takes at once",
			len(body), maxRequestBytes)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.completions(), bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("preparing the translation request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return "", translation.Trouble("the service", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("the service refused the draft: %s%s",
			response.Status, explanation(response.Body))
	}

	var decoded completion
	if err := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes)).Decode(&decoded); err != nil {
		return "", fmt.Errorf("reading the translation: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return "", errors.New("the service answered without a translation")
	}

	translated := strings.TrimSpace(decoded.Choices[0].Message.Content)
	if translated == "" {
		return "", errors.New("the service answered with an empty translation")
	}
	return translated, nil
}

type completion struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
	// Reasoning models refuse a temperature, so it is left out for them rather
	// than sent and refused.
	Temperature *float64 `json:"temperature,omitempty"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// The instruction is the service's whole job: translate, keep the code, and
// answer with nothing else. A draft written in the language it is already in
// comes back as it was, which is what the author asked for by writing it.
func (c *Client) requestBody(draft, surrounding string) ([]byte, error) {
	instruction := fmt.Sprintf(
		"Translate the user's message into %s. Answer with the translation only: "+
			"no quotes, no explanation, no notes. Keep code, commands, file paths, URLs, "+
			"markdown and technical terms exactly as they are.",
		c.targetLanguage)

	if strings.TrimSpace(surrounding) != "" {
		instruction += "\n\nThe text that comes before it, for meaning only — do not translate it:\n" +
			surrounding
	}

	request := chatRequest{
		Model: c.model,
		Messages: []message{
			{Role: "system", Content: instruction},
			{Role: "user", Content: draft},
		},
	}
	if !reasoningModel(c.model) {
		zero := 0.0
		request.Temperature = &zero
	}

	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encoding the translation request: %w", err)
	}
	return body, nil
}

func reasoningModel(model string) bool {
	for _, prefix := range []string{"o1", "o3", "o4", "gpt-5"} {
		if strings.HasPrefix(strings.ToLower(model), prefix) {
			return true
		}
	}
	return false
}

// completions takes whatever shape the endpoint was given: a base URL, a base URL
// with the path, or the full address.
func (c *Client) completions() string {
	endpoint := strings.TrimRight(c.endpoint, "/")
	if strings.HasSuffix(endpoint, "/chat/completions") {
		return endpoint
	}
	return endpoint + "/chat/completions"
}

// refuseInsecureRedirect keeps the key from following a redirect off https. Go
// carries the credential header when a host redirects to itself, downgrade
// included, so this is installed for every client rather than asked for.
func refuseInsecureRedirect(request *http.Request, _ []*http.Request) error {
	if request.URL.Scheme != "https" {
		return fmt.Errorf("refusing a redirect to %s: the API key travels with it",
			request.URL.Scheme)
	}
	return nil
}

func explanation(body io.Reader) string {
	raw, err := io.ReadAll(io.LimitReader(body, maxErrorBodyBytes))
	if err != nil || len(raw) == 0 {
		return ""
	}

	var refusal struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &refusal); err == nil && refusal.Error.Message != "" {
		return " — " + refusal.Error.Message
	}
	return " — " + strings.TrimSpace(string(raw))
}
