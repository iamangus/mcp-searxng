package urlreader

import (
	"bytes"
	"context"
	"fmt"
	"mime"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"

	"github.com/iamangus/mcp-searxng/internal/cache"
	"github.com/iamangus/mcp-searxng/internal/config"
	mcperrors "github.com/iamangus/mcp-searxng/internal/errors"
	"github.com/iamangus/mcp-searxng/internal/logging"
	"github.com/iamangus/mcp-searxng/internal/netclient"
	"github.com/iamangus/mcp-searxng/internal/proxy"
)

type Reader struct {
	cfg   *config.Config
	cache *cache.Cache
	http  *http.Client
}

type ReadResult struct {
	URL           string `json:"url"`
	Content       string `json:"content"`
	StartChar     int    `json:"startChar"`
	TotalChars    int    `json:"totalChars"`
	NextStartChar *int   `json:"nextStartChar,omitempty"`
	Truncated     bool   `json:"truncated"`
}

func NewReader(cfg *config.Config, urlCache *cache.Cache) *Reader {
	return &Reader{cfg: cfg, cache: urlCache, http: netclient.NewPublic(proxy.NewFunc(cfg, proxy.URLReader))}
}

func (r *Reader) Close() { r.http.CloseIdleConnections() }

func (r *Reader) Read(ctx context.Context, targetURL string, opts PaginationOptions) (*ReadResult, error) {
	startTime := time.Now()
	u, err := netclient.ParseURL(targetURL)
	if err != nil {
		return nil, err
	}
	targetURL = u.String()
	if opts.StartChar < 0 || opts.StartChar > 10000000 || (opts.HasMaxLength && (opts.MaxLength < 1 || opts.MaxLength > netclient.MaxOutputChars)) {
		return nil, fmt.Errorf("invalid pagination bounds")
	}

	if entry, ok := r.cache.Get(targetURL); ok {
		return paginate(targetURL, entry.MarkdownContent, opts), nil
	}

	ctx, cancel := context.WithTimeout(ctx, netclient.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, mcperrors.NewURLFormatError(targetURL)
	}

	ua := r.cfg.URLReaderAgent()
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}

	req.Header.Set("Accept", "text/html, application/xhtml+xml, text/plain, text/markdown")
	resp, err := r.http.Do(req)
	if err != nil {
		// Do not echo URLs, proxy credentials or upstream response bodies.
		if ctx.Err() != nil {
			return nil, fmt.Errorf("URL request cancelled or timed out")
		}
		return nil, fmt.Errorf("URL fetch rejected or failed: check public destination, TLS and network connectivity")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, mcperrors.NewServerError(resp.StatusCode, http.StatusText(resp.StatusCode), "", "")
	}

	htmlContent, err := netclient.ReadBody(resp.Body)
	if err != nil {
		return nil, err
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(htmlContent)
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || (mediaType != "text/html" && mediaType != "application/xhtml+xml" && mediaType != "text/plain" && mediaType != "text/markdown") {
		return nil, fmt.Errorf("unsupported content type; only HTML, plain text and Markdown are supported")
	}
	decoded, err := charset.NewReader(bytes.NewReader(htmlContent), contentType)
	if err != nil {
		return nil, fmt.Errorf("unsupported text charset")
	}
	htmlContent, err = netclient.ReadBody(decoded)
	if err != nil {
		return nil, err
	}

	if len(htmlContent) == 0 || strings.TrimSpace(string(htmlContent)) == "" {
		return nil, fmt.Errorf("website returned empty content")
	}

	markdownContent := string(htmlContent)
	if mediaType == "text/html" || mediaType == "application/xhtml+xml" {
		doc, parseErr := html.Parse(bytes.NewReader(htmlContent))
		if parseErr != nil {
			return nil, fmt.Errorf("cannot parse HTML")
		}
		if err := checkDOM(ctx, doc); err != nil {
			return nil, err
		}
		converted, convertErr := htmltomarkdown.ConvertNode(doc, converter.WithDomain(resp.Request.URL.String()), converter.WithContext(ctx))
		markdownContent, err = string(converted), convertErr
	}
	if err != nil {
		return nil, fmt.Errorf("cannot convert HTML to Markdown")
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if len(markdownContent) > netclient.MaxBodyBytes {
		return nil, fmt.Errorf("converted content exceeds size limit")
	}

	if strings.TrimSpace(markdownContent) == "" {
		return nil, fmt.Errorf("page has no readable content; it may require JavaScript")
	}

	r.cache.Set(targetURL, "", markdownContent)

	result := paginate(targetURL, markdownContent, opts)

	duration := time.Since(startTime)
	logging.LogMessage(logging.Info, fmt.Sprintf("URL fetched and converted (%d chars in %dms)", utf8.RuneCountInString(result.Content), duration.Milliseconds()))
	return result, nil
}

// Bound recursive converter work in addition to the downloaded byte budget.
func checkDOM(ctx context.Context, root *html.Node) error {
	type item struct {
		node  *html.Node
		depth int
	}
	stack := []item{{root, 0}}
	count := 0
	for len(stack) > 0 {
		last := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		count++
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if last.depth > 128 || count > 50000 {
			return fmt.Errorf("HTML complexity exceeds conversion limit")
		}
		for child := last.node.FirstChild; child != nil; child = child.NextSibling {
			stack = append(stack, item{child, last.depth + 1})
		}
	}
	return nil
}

func paginate(targetURL, content string, opts PaginationOptions) *ReadResult {
	filters := opts
	filters.HasStartChar, filters.HasMaxLength = false, false
	filtered := ApplyPaginationOptions(content, filters)
	total := utf8.RuneCountInString(filtered)
	limit := 10000
	if opts.HasMaxLength {
		limit = opts.MaxLength
	}
	start := min(opts.StartChar, total)
	result := &ReadResult{URL: targetURL, StartChar: start, TotalChars: total, Content: ApplyCharacterPagination(filtered, start, limit)}
	next := start + utf8.RuneCountInString(result.Content)
	if next < total {
		result.NextStartChar = &next
		result.Truncated = true
	}
	return result
}
