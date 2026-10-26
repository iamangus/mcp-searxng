package urlreader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/iamangus/mcp-searxng/internal/cache"
	"github.com/iamangus/mcp-searxng/internal/config"
	"github.com/iamangus/mcp-searxng/internal/netclient"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func readerFor(t *testing.T, kind, body string) (*Reader, *int) {
	t.Helper()
	calls := new(int)
	c := cache.NewCache(time.Minute)
	t.Cleanup(c.Stop)
	r := NewReader(&config.Config{}, c)
	t.Cleanup(r.Close)
	r.http = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		*calls++
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{kind}}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	})}
	return r, calls
}

func TestReadHTMLRelativeLinksAndCache(t *testing.T) {
	r, calls := readerFor(t, "text/html; charset=utf-8", `<h1>Title</h1><p>Hello 世界 <a href="next">next</a></p>`)
	result, err := r.Read(context.Background(), "https://example.com/docs/page", PaginationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Content, "世界") || !strings.Contains(result.Content, "https://example.com/docs/next") {
		t.Fatal(result.Content)
	}
	_, err = r.Read(context.Background(), "https://example.com/docs/page", PaginationOptions{ReadHeadings: true})
	if err != nil || *calls != 1 {
		t.Fatal("cache not reused", err)
	}
	entry, _ := r.cache.Get("https://example.com/docs/page")
	if entry.HTMLContent != "" {
		t.Fatal("raw HTML retained")
	}
}

func TestTextAndUnicodeContinuation(t *testing.T) {
	r, _ := readerFor(t, "text/plain; charset=utf-8", "a🌍你好z")
	result, err := r.Read(context.Background(), "https://example.com", PaginationOptions{HasMaxLength: true, MaxLength: 3})
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "a🌍你" || !result.Truncated || result.TotalChars != 5 || *result.NextStartChar != 3 {
		t.Fatalf("%+v", result)
	}
	next, err := r.Read(context.Background(), "https://example.com", PaginationOptions{HasStartChar: true, StartChar: *result.NextStartChar})
	if err != nil || next.Content != "好z" || next.Truncated {
		t.Fatal(next, err)
	}
}

func TestContentValidationAndLimits(t *testing.T) {
	for _, tc := range []struct{ kind, body string }{{"application/pdf", "%PDF-1"}, {"text/html", ""}, {"text/plain", strings.Repeat("x", netclient.MaxBodyBytes+1)}} {
		r, _ := readerFor(t, tc.kind, tc.body)
		if _, err := r.Read(context.Background(), "https://example.com", PaginationOptions{}); err == nil {
			t.Errorf("accepted %s (%d bytes)", tc.kind, len(tc.body))
		}
	}
	r, _ := readerFor(t, "text/plain", strings.Repeat("a", 20000))
	result, err := r.Read(context.Background(), "https://example.com", PaginationOptions{})
	if err != nil || len(result.Content) != 10000 || !result.Truncated {
		t.Fatal(result, err)
	}
	for _, opts := range []PaginationOptions{{StartChar: -1}, {HasMaxLength: true, MaxLength: 50001}} {
		if _, err := r.Read(context.Background(), "https://example.com", opts); err == nil {
			t.Fatal("invalid options accepted")
		}
	}
}

func TestCharsetAndCancellation(t *testing.T) {
	r, _ := readerFor(t, "text/html; charset=windows-1252", "<p>caf\xe9</p>")
	result, err := r.Read(context.Background(), "https://example.com", PaginationOptions{})
	if err != nil || !strings.Contains(result.Content, "café") {
		t.Fatal(result, err)
	}
	r.http = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, fmt.Errorf("cancelled")
	})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Read(ctx, "https://another.example.com", PaginationOptions{}); err == nil {
		t.Fatal("cancel ignored")
	}
}

func TestHTMLComplexityBound(t *testing.T) {
	r, _ := readerFor(t, "text/html", strings.Repeat("<div>", 150)+"hello"+strings.Repeat("</div>", 150))
	if _, err := r.Read(context.Background(), "https://example.com", PaginationOptions{}); err == nil || !strings.Contains(err.Error(), "complexity") {
		t.Fatal(err)
	}
}
