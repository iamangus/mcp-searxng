package search

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/iamangus/mcp-searxng/internal/aia"
	"github.com/iamangus/mcp-searxng/internal/config"
	mcperrors "github.com/iamangus/mcp-searxng/internal/errors"
	"github.com/iamangus/mcp-searxng/internal/logging"
	"github.com/iamangus/mcp-searxng/internal/proxy"
)

type SearXNGResult struct {
	Title   string  `json:"title"`
	Content string  `json:"content"`
	URL     string  `json:"url"`
	Score   float64 `json:"score"`
}

type SearXNGResponse struct {
	Results []SearXNGResult `json:"results"`
}

type SearchParams struct {
	Query      string
	PageNo     int
	TimeRange  string
	Language   string
	SafeSearch int
}

func PerformWebSearch(ctx context.Context, cfg *config.Config, certCache *aia.CertCache, params SearchParams) (string, error) {
	startTime := time.Now()

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
		fmt.Sprintf("Starting web search: %q (%s)", params.Query, strings.Join(searchParts, ", ")))

	searxngURL := cfg.SearXNGURL
	if searxngURL == "" {
		return "", mcperrors.NewConfigurationError("SEARXNG_URL not set")
	}

	baseURL, err := url.Parse(searxngURL)
	if err != nil {
		return "", mcperrors.NewConfigurationError(fmt.Sprintf("Invalid SEARXNG_URL format: %s", searxngURL))
	}

	searchURL := baseURL.JoinPath("search")
	if searchURL == nil {
		return "", mcperrors.NewConfigurationError(fmt.Sprintf("Invalid SEARXNG_URL format: %s", searxngURL))
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

	logging.LogMessage(logging.Info,
		fmt.Sprintf("Making request to: %s", searchURL.String()))

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: proxy.GetProxyFunc(proxy.Search),
			TLSClientConfig: &tls.Config{
				VerifyPeerCertificate: certCache.VerifyPeerCertificate,
			},
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL.String(), nil)
	if err != nil {
		return "", mcperrors.NewNetworkError(err, searchURL.String(), searxngURL)
	}

	if cfg.HasAuth() {
		req.SetBasicAuth(cfg.AuthUsername, cfg.AuthPassword)
	}

	if cfg.UserAgent != "" {
		req.Header.Set("User-Agent", cfg.UserAgent)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", mcperrors.NewNetworkError(err, searchURL.String(), searxngURL)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", mcperrors.NewServerError(resp.StatusCode, resp.Status, string(body), searxngURL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", mcperrors.NewJSONError("")
	}

	var data SearXNGResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return "", mcperrors.NewJSONError(string(body))
	}

	if data.Results == nil {
		return "", mcperrors.NewDataError()
	}

	results := data.Results
	if len(results) == 0 {
		logging.LogMessage(logging.Info,
			fmt.Sprintf("No results found for query: %q", params.Query))
		return mcperrors.NewNoResultsMessage(params.Query), nil
	}

	duration := time.Since(startTime)
	logging.LogMessage(logging.Info,
		fmt.Sprintf("Search completed: %q - %d results in %dms", params.Query, len(results), duration.Milliseconds()))

	var parts []string
	for _, r := range results {
		parts = append(parts, fmt.Sprintf("Title: %s\nDescription: %s\nURL: %s\nRelevance Score: %.3f",
			r.Title, r.Content, r.URL, r.Score))
	}

	return strings.Join(parts, "\n\n"), nil
}

func isValidTimeRange(tr string) bool {
	return tr == "day" || tr == "month" || tr == "year"
}
