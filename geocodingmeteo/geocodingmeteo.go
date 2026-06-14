// Package geocodingmeteo is the library behind the geocodingmeteo command line:
// the HTTP client, request shaping, and the typed data models for the
// Open-Meteo Geocoding API.
//
// The Client sets a real User-Agent, paces requests so a busy session stays
// polite, and retries the transient failures (429 and 5xx) that any public API
// throws under load. Build your endpoint calls and JSON decoding on top of it.
package geocodingmeteo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// DefaultUserAgent identifies the client to Open-Meteo. A real, honest
// User-Agent is both polite and the thing most likely to keep you unblocked.
const DefaultUserAgent = "geocodingmeteo/dev (+https://github.com/tamnd/geocodingmeteo-cli)"

// Host is the geocoding API hostname, used as the URI driver scheme host.
const Host = "geocoding-api.open-meteo.com"

// Config holds per-client tuning knobs. DefaultConfig returns ready-to-use
// values; a host may override individual fields.
type Config struct {
	BaseURL string
	Rate    time.Duration
	Retries int
	Timeout time.Duration
}

// DefaultConfig returns production-safe defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL: "https://geocoding-api.open-meteo.com",
		Rate:    0,
		Retries: 3,
		Timeout: 10 * time.Second,
	}
}

// Location is one geocoding result: a city or named place returned by the
// Open-Meteo Geocoding API. The kit:"id" tag makes ID the resource address.
type Location struct {
	ID          int     `kit:"id" json:"id"`
	Name        string  `json:"name"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Elevation   float64 `json:"elevation"`
	Country     string  `json:"country"`
	CountryCode string  `json:"country_code"`
	Timezone    string  `json:"timezone"`
	Population  int     `json:"population"`
	Region      string  `json:"region"` // admin1
	FeatureCode string  `json:"feature_code"`
}

// Client talks to the Open-Meteo Geocoding API over HTTPS.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	BaseURL   string
	// Rate is the minimum gap between requests. Zero means no pacing.
	Rate    time.Duration
	Retries int

	last time.Time
}

// NewClient returns a Client with production-safe defaults derived from
// DefaultConfig.
func NewClient() *Client {
	cfg := DefaultConfig()
	return &Client{
		HTTP:      &http.Client{Timeout: cfg.Timeout},
		UserAgent: DefaultUserAgent,
		BaseURL:   cfg.BaseURL,
		Rate:      cfg.Rate,
		Retries:   cfg.Retries,
	}
}

// Search queries the geocoding API for places matching name. count controls
// the maximum number of results; language is an ISO 639-1 code (e.g. "en").
func (c *Client) Search(ctx context.Context, name string, count int, language string) ([]*Location, error) {
	if count <= 0 {
		count = 10
	}
	if language == "" {
		language = "en"
	}
	q := url.Values{}
	q.Set("name", name)
	q.Set("count", strconv.Itoa(count))
	q.Set("language", language)
	q.Set("format", "json")
	apiURL := c.BaseURL + "/v1/search?" + q.Encode()

	body, err := c.Get(ctx, apiURL)
	if err != nil {
		return nil, err
	}

	var wire wireSearchResponse
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}

	out := make([]*Location, 0, len(wire.Results))
	for _, w := range wire.Results {
		out = append(out, &Location{
			ID:          w.ID,
			Name:        w.Name,
			Latitude:    w.Latitude,
			Longitude:   w.Longitude,
			Elevation:   w.Elevation,
			Country:     w.Country,
			CountryCode: w.CountryCode,
			Timezone:    w.Timezone,
			Population:  w.Population,
			Region:      w.Admin1,
			FeatureCode: w.FeatureCode,
		})
	}
	return out, nil
}

// Get fetches url and returns the response body. It paces and retries
// according to the client's settings. The caller owns the returned bytes.
func (c *Client) Get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

// pace blocks until at least Rate has passed since the previous request.
func (c *Client) pace() {
	if c.Rate <= 0 {
		return
	}
	if wait := c.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := min(time.Duration(attempt)*500*time.Millisecond, 5*time.Second)
	return d
}

// --- wire types ---------------------------------------------------------------

type wireSearchResponse struct {
	Results []wireLocation `json:"results"`
}

type wireLocation struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Elevation   float64 `json:"elevation"`
	FeatureCode string  `json:"feature_code"`
	CountryCode string  `json:"country_code"`
	Timezone    string  `json:"timezone"`
	Population  int     `json:"population"`
	Country     string  `json:"country"`
	Admin1      string  `json:"admin1"`
}
