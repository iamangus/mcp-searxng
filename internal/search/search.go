package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/iamangus/mcp-searxng/internal/config"
	mcperrors "github.com/iamangus/mcp-searxng/internal/errors"
	"github.com/iamangus/mcp-searxng/internal/logging"
	"github.com/iamangus/mcp-searxng/internal/netclient"
	"github.com/iamangus/mcp-searxng/internal/proxy"
)

type SearXNGResult struct {
	Title   string  `json:"title"`
	Content string  `json:"content"`
	URL     string  `json:"url"`
	Score   float64 `json:"score"`
}

type SearXNGResponse struct {
	Results             []SearXNGResult   `json:"results"`
	Suggestions         []string          `json:"suggestions,omitempty"`
	Answers             []json.RawMessage `json:"answers,omitempty"`
	UnresponsiveEngines []json.RawMessage `json:"unresponsive_engines,omitempty"`
	Truncated           bool              `json:"truncated"`
	PageNo              int               `json:"page"`
}

type SearchParams struct {
	Query      string
	PageNo     int
	TimeRange  string
	Language   string
	SafeSearch int
}

type Client struct {
	cfg  *config.Config
	http *http.Client
}

func NewClient(cfg *config.Config) *Client {
	return &Client{cfg: cfg, http: netclient.NewTrusted(proxy.NewFunc(cfg, proxy.Search))}
}

func (c *Client) Close() { c.http.CloseIdleConnections() }

func (c *Client) Search(ctx context.Context, params SearchParams) (*SearXNGResponse, error) {
	cfg := c.cfg
	startTime := time.Now()
	if strings.TrimSpace(params.Query) == "" || len(params.Query) > 8192 || params.PageNo < 1 || params.PageNo > 100 || params.SafeSearch < 0 || params.SafeSearch > 2 || (params.TimeRange != "" && !isValidTimeRange(params.TimeRange)) {
		return nil, fmt.Errorf("invalid search parameters")
	}
	ctx, cancel := context.WithTimeout(ctx, netclient.Timeout)
	defer cancel()

	searchParts := []string{
		fmt.Sprintf("page %d", params.PageNo),
		fmt.Sprintf("lang: %s", params.Language),
	}
	if params.TimeRange != "" {
		searchParts = append(searchParts, fmt.Sprintf("time: %s", params.TimeRange))
	}
	if params.SafeSearch > 0 {
		searchParts = append(searchParts, fmt.Sprintf("safesearch: %d", params.SafeSearch))
	}

	logging.LogMessage(logging.Info,
		fmt.Sprintf("Starting web search (%s)", strings.Join(searchParts, ", ")))

	searxngURL := cfg.SearXNGURL
	if searxngURL == "" {
		return nil, mcperrors.NewConfigurationError("SEARXNG_URL not set")
	}

	baseURL, err := url.Parse(searxngURL)
	if err != nil {
		return nil, mcperrors.NewConfigurationError("Invalid SEARXNG_URL")
	}

	searchURL := baseURL.JoinPath("search")
	if searchURL == nil {
		return nil, mcperrors.NewConfigurationError("Invalid SEARXNG_URL")
	}

	q := searchURL.Query()
	q.Set("q", params.Query)
	q.Set("format", "json")
	q.Set("pageno", strconv.Itoa(params.PageNo))

	if params.TimeRange != "" && isValidTimeRange(params.TimeRange) {
		q.Set("time_range", params.TimeRange)
	}
	if params.Language != "" && params.Language != "all" {
		q.Set("language", params.Language)
	}
	if params.SafeSearch >= 0 && params.SafeSearch <= 2 {
		q.Set("safesearch", strconv.Itoa(params.SafeSearch))
	}
	searchURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("cannot create search request")
	}

	if cfg.HasAuth() {
		req.SetBasicAuth(cfg.AuthUsername, cfg.AuthPassword)
	}

	if cfg.UserAgent != "" {
		req.Header.Set("User-Agent", cfg.UserAgent)
	}

	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request failed (network, TLS, redirect or timeout)")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, mcperrors.NewServerError(resp.StatusCode, http.StatusText(resp.StatusCode), "", searxngURL)
	}

	body, err := netclient.ReadBody(resp.Body)
	if err != nil {
		return nil, err
	}

	var data SearXNGResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, mcperrors.NewJSONError("")
	}

	if data.Results == nil {
		return nil, mcperrors.NewDataError()
	}

	duration := time.Since(startTime)
	logging.LogMessage(logging.Info,
		fmt.Sprintf("Search completed: %d results in %dms", len(data.Results), duration.Milliseconds()))
	data.PageNo = params.PageNo
	// Bound structured output as well as the text fallback.
	data.Truncated = len(data.Results) > 20
	if len(data.Results) > 20 {
		data.Results = data.Results[:20]
	}
	for i := range data.Results {
		r := &data.Results[i]
		r.Title = truncate(r.Title, 500, &data.Truncated)
		r.Content = truncate(r.Content, 1000, &data.Truncated)
		r.URL = truncate(r.URL, 2048, &data.Truncated)
	}
	// These fields are useful diagnostics but must not bypass the output cap.
	if len(data.Suggestions) > 10 {
		data.Suggestions = data.Suggestions[:10]
		data.Truncated = true
	}
	for i := range data.Suggestions {
		data.Suggestions[i] = truncate(data.Suggestions[i], 500, &data.Truncated)
	}
	if len(data.Answers) > 10 {
		data.Answers = data.Answers[:10]
		data.Truncated = true
	}
	for i, raw := range data.Answers {
		if len(raw) > 1024 {
			data.Answers[i] = json.RawMessage(`"answer omitted: exceeds size limit"`)
			data.Truncated = true
		}
	}
	if len(data.UnresponsiveEngines) > 20 {
		data.UnresponsiveEngines = data.UnresponsiveEngines[:20]
		data.Truncated = true
	}
	for i, raw := range data.UnresponsiveEngines {
		if len(raw) > 1024 {
			data.UnresponsiveEngines[i] = json.RawMessage(`"engine diagnostic omitted"`)
			data.Truncated = true
		}
	}
	return &data, nil
}

func (data *SearXNGResponse) Text() string {
	if len(data.Results) == 0 {
		return "No search results found. Check engine diagnostics or try another query."
	}
	var parts []string
	for _, r := range data.Results {
		parts = append(parts, fmt.Sprintf("Title: %s\nDescription: %s\nURL: %s\nRelevance Score: %.3f",
			r.Title, r.Content, r.URL, r.Score))
	}

	return strings.Join(parts, "\n\n")
}

func truncate(s string, limit int, truncated *bool) string {
	r := []rune(s)
	if len(r) > limit {
		*truncated = true
		return string(r[:limit])
	}
	return s
}

func isValidTimeRange(tr string) bool {
	return tr == "day" || tr == "month" || tr == "year"
}
