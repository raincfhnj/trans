// Package gtranslate translates through Google's public translate endpoint. It
// needs no key, which is what makes it the service a panel can fall back on when
// nothing has been configured yet. It is one service behind the translation
// ports, and knows nothing about the overlay.
package gtranslate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"trans/internal/translation"
)

const (
	defaultEndpoint       = "https://translate.googleapis.com/translate_a/single"
	defaultTargetLanguage = "en"
	defaultTimeout        = 15 * time.Second

	maxErrorBodyBytes = 2 << 10
	maxResponseBytes  = 1 << 20
	// The draft travels in the query string, which the network and the endpoint
	// both limit; a longer draft is sent in pieces. A piece that ends mid-sentence
	// costs a little quality, so the limit is generous.
	maxChunkRunes = 1200
)

type Client struct {
	httpClient     *http.Client
	endpoint       string
	targetLanguage string
}

type Option func(*Client)

func WithEndpoint(endpoint string) Option {
	return func(c *Client) { c.endpoint = endpoint }
}

func WithTargetLanguage(language string) Option {
	return func(c *Client) { c.targetLanguage = languageCode(language) }
}

func New(options ...Option) *Client {
	client := &Client{
		httpClient:     &http.Client{Timeout: defaultTimeout},
		endpoint:       defaultEndpoint,
		targetLanguage: defaultTargetLanguage,
	}
	for _, option := range options {
		option(client)
	}
	return client
}

func (c *Client) Endpoint() string { return c.endpoint }

func (c *Client) Translate(ctx context.Context, draft string) (string, error) {
	var translated strings.Builder

	for _, chunk := range chunks(draft, maxChunkRunes) {
		answer, err := c.translateOne(ctx, chunk)
		if err != nil {
			return "", err
		}
		translated.WriteString(answer)
	}
	return translated.String(), nil
}

func (c *Client) translateOne(ctx context.Context, draft string) (string, error) {
	query := url.Values{
		"client": {"gtx"},
		"sl":     {"auto"},
		"tl":     {c.targetLanguage},
		"dt":     {"t"},
		"q":      {draft},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"?"+query.Encode(), http.NoBody)
	if err != nil {
		return "", fmt.Errorf("preparing the translation request: %w", err)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return "", translation.Trouble("google", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("the service refused the draft: %s%s",
			response.Status, explanation(response.Body))
	}

	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return "", fmt.Errorf("reading the translation: %w", err)
	}
	return decode(raw)
}

// The answer is a list of sentences, each a list whose first entry is the
// translated one; the rest of the entries are alternatives and metadata.
func decode(raw []byte) (string, error) {
	var top []json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return "", fmt.Errorf("reading the translation: %w", err)
	}
	if len(top) == 0 {
		return "", fmt.Errorf("the service answered without a translation")
	}

	var segments [][]json.RawMessage
	if err := json.Unmarshal(top[0], &segments); err != nil {
		return "", fmt.Errorf("reading the translation: %w", err)
	}

	var whole strings.Builder
	for _, segment := range segments {
		if len(segment) == 0 {
			continue
		}
		var piece string
		if err := json.Unmarshal(segment[0], &piece); err != nil {
			return "", fmt.Errorf("reading the translation: %w", err)
		}
		whole.WriteString(piece)
	}
	if whole.Len() == 0 {
		return "", fmt.Errorf("the service answered without a translation")
	}
	return whole.String(), nil
}

// chunks cuts a draft at a rune boundary, so a multi-byte character is never
// split in two.
func chunks(draft string, limit int) []string {
	runes := []rune(draft)
	if len(runes) <= limit {
		return []string{draft}
	}

	var pieces []string
	for start := 0; start < len(runes); start += limit {
		pieces = append(pieces, string(runes[start:min(start+limit, len(runes))]))
	}
	return pieces
}

// languageCode says the language the way this endpoint does: a plain code.
func languageCode(language string) string {
	language = strings.TrimSpace(language)
	if language == "" {
		return defaultTargetLanguage
	}
	code, _, _ := strings.Cut(language, "-")
	return strings.ToLower(code)
}

func explanation(body io.Reader) string {
	raw, err := io.ReadAll(io.LimitReader(body, maxErrorBodyBytes))
	if err != nil || len(raw) == 0 {
		return ""
	}

	trimmed := strings.TrimSpace(string(raw))
	lowered := strings.ToLower(trimmed)
	// The endpoint answers a refusal with a web page, and a page says nothing a
	// sentence of ours could not; what it means is that the quota is watched.
	if strings.HasPrefix(lowered, "<!doctype") || strings.HasPrefix(lowered, "<html") {
		return " — the endpoint did not take the draft (rate limited, most likely)"
	}
	return " — " + trimmed
}
