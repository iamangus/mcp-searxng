package urlreader

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"

	"github.com/iamangus/mcp-searxng/internal/aia"
	"github.com/iamangus/mcp-searxng/internal/cache"
	"github.com/iamangus/mcp-searxng/internal/config"
	mcperrors "github.com/iamangus/mcp-searxng/internal/errors"
	"github.com/iamangus/mcp-searxng/internal/logging"
	"github.com/iamangus/mcp-searxng/internal/proxy"
)

func FetchAndConvertToMarkdown(
	ctx context.Context,
	cfg *config.Config,
	urlCache *cache.Cache,
	certCache *aia.CertCache,
	targetURL string,
	timeoutMs int,
	opts PaginationOptions,
) (string, error) {
	startTime := time.Now()
	logging.LogMessage(logging.Info, fmt.Sprintf("Fetching URL: %s", targetURL))

	if entry, ok := urlCache.Get(targetURL); ok {
		logging.LogMessage(logging.Info, fmt.Sprintf("Using cached content for URL: %s", targetURL))
		result := ApplyPaginationOptions(entry.MarkdownContent, opts)
		duration := time.Since(startTime)
		logging.LogMessage(logging.Info, fmt.Sprintf("Processed cached URL: %s (%d chars in %dms)", targetURL, len(result), duration.Milliseconds()))
		return result, nil
	}

	if _, err := validateURL(targetURL); err != nil {
		return "", err
	}

	if timeoutMs <= 0 {
		timeoutMs = 10000
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	transport := &http.Transport{
		Proxy: proxy.GetProxyFunc(proxy.URLReader),
		TLSClientConfig: &tls.Config{
			VerifyPeerCertificate: certCache.VerifyPeerCertificate,
		},
	}

	client := &http.Client{
		Transport: transport,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return "", mcperrors.NewURLFormatError(targetURL)
	}

	ua := cfg.URLReaderAgent()
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}

	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", mcperrors.NewTimeoutError(timeoutMs, targetURL)
		}
		return "", mcperrors.NewNetworkError(err, targetURL, "")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", mcperrors.NewServerError(resp.StatusCode, resp.Status, string(body), "")
	}

	htmlContent, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", mcperrors.NewContentError(fmt.Sprintf("Failed to read website content: %s", err.Error()), targetURL)
	}

	if len(htmlContent) == 0 || strings.TrimSpace(string(htmlContent)) == "" {
		return "", mcperrors.NewContentError("Website returned empty content.", targetURL)
	}

	markdownContent, err := htmltomarkdown.ConvertString(string(htmlContent))
	if err != nil {
		return "", mcperrors.NewConversionError(targetURL)
	}

	if strings.TrimSpace(markdownContent) == "" {
		logging.LogMessage(logging.Warning, fmt.Sprintf("Empty content after conversion: %s", targetURL))
		return mcperrors.NewEmptyContentWarning(targetURL), nil
	}

	urlCache.Set(targetURL, string(htmlContent), markdownContent)

	result := ApplyPaginationOptions(markdownContent, opts)

	duration := time.Since(startTime)
	logging.LogMessage(logging.Info, fmt.Sprintf("Successfully fetched and converted URL: %s (%d chars in %dms)", targetURL, len(result), duration.Milliseconds()))
	return result, nil
}

func validateURL(raw string) (string, error) {
	if raw == "" || !strings.HasPrefix(raw, "http") {
		return "", mcperrors.NewURLFormatError(raw)
	}
	return raw, nil
}
