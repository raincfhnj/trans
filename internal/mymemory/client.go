// Package mymemory translates through MyMemory, a free service that needs no key
// and answers a limited number of characters a day. It is the second service a
// panel can fall back on when nothing has been configured. It is one service
// behind the translation ports, and knows nothing about the overlay.
package mymemory

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"trans/internal/translation"
)

const (
	defaultEndpoint = "https://api.mymemory.translated.net/get"
	// The service detects the language itself when it is not named, which is what
	// lets one panel serve authors writing in any language.
	autodetect = "Autodetect"
	// The free endpoint takes the draft in the query string and is meant for
	// short texts; a longer draft is sent in pieces.
	maxChunkRunes = 450

	defaultTimeout    = 15 * time.Second
	maxErrorBodyBytes = 2 << 10
	maxResponseBytes  = 1 << 20
)

type Client struct {
	httpClient     *http.Client
	endpoint       string
	targetLanguage string
	email          string
}

type Option func(*Client)

func WithEndpoint(endpoint string) Option {
	return func(c *Client) { c.endpoint = endpoint }
}

func WithTargetLanguage(language string) Option {
	return func(c *Client) { c.targetLanguage = languageCode(language) }
}

// WithEmail adds the address the service asks for when a lot is translated; it
// raises the daily allowance rather than being used for anything else.
func WithEmail(email string) Option {
	return func(c *Client) { c.email = email }
}

func New(options ...Option) *Client {
	client := &Client{
		httpClient:     &http.Client{Timeout: defaultTimeout},
		endpoint:       defaultEndpoint,
		targetLanguage: "en",
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

type answer struct {
	Data struct {
		Translated string `json:"translatedText"`
	} `json:"responseData"`
	Status any `json:"responseStatus"`
}

func (c *Client) translateOne(ctx context.Context, draft string) (string, error) {
	query := url.Values{
		"q":        {draft},
		"langpair": {autodetect + "|" + c.targetLanguage},
	}
	if c.email != "" {
		query.Set("de", c.email)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"?"+query.Encode(), http.NoBody)
	if err != nil {
		return "", fmt.Errorf("preparing the translation request: %w", err)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return "", translation.Trouble("mymemory", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("the service refused the draft: %s%s",
			response.Status, explanation(response.Body))
	}

	var decoded answer
	if err := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes)).Decode(&decoded); err != nil {
		return "", fmt.Errorf("reading the translation: %w", err)
	}
	if strings.TrimSpace(decoded.Data.Translated) == "" {
		return "", fmt.Errorf("the service answered without a translation: %v", decoded.Status)
	}
	// Entities come back for text the service took the draft to hold.
	return html.UnescapeString(decoded.Data.Translated), nil
}

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

// languageCode says the language the way this endpoint does: a plain code, with
// the region kept only where the region is the point.
func languageCode(language string) string {
	language = strings.TrimSpace(language)
	if language == "" {
		return "en"
	}

	code, region, hasRegion := strings.Cut(language, "-")
	code = strings.ToLower(code)
	if !hasRegion {
		return code
	}

	region = strings.ToUpper(region)
	if regionMatters[code+"-"+region] {
		return code + "-" + region
	}
	return code
}

var regionMatters = map[string]bool{
	"en-GB": true,
	"pt-BR": true,
	"pt-PT": true,
	"zh-CN": true,
	"zh-TW": true,
	"fr-CA": true,
	"es-MX": true,
}

func explanation(body io.Reader) string {
	raw, err := io.ReadAll(io.LimitReader(body, maxErrorBodyBytes))
	if err != nil || len(raw) == 0 {
		return ""
	}
	return " — " + strings.TrimSpace(string(raw))
}
