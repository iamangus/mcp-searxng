package errors_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	mcperrors "github.com/iamangus/mcp-searxng/internal/errors"
)

func TestNewConfigurationError(t *testing.T) {
	msg := "base URL is required"
	err := mcperrors.NewConfigurationError(msg)
	want := "Configuration Error: base URL is required"
	if err.Error() != want {
		t.Errorf("NewConfigurationError(%q) = %q, want %q", msg, err.Error(), want)
	}
}

func TestNewNetworkError_ConnectionRefused(t *testing.T) {
	inner := errors.New("ECONNREFUSED: something")
	err := mcperrors.NewNetworkError(inner, "http://example.com", "")
	want := "Connection Error: website is not responding (http://example.com)"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestNewNetworkError_ConnectionRefused_SearXNG(t *testing.T) {
	inner := errors.New("ECONNREFUSED: something")
	err := mcperrors.NewNetworkError(inner, "http://localhost:8888", "http://localhost:8888")
	want := "Connection Error: SearXNG server is not responding (http://localhost:8888)"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestNewNetworkError_DNSFailure_ENOTFOUND(t *testing.T) {
	inner := errors.New("ENOTFOUND: no such host")
	err := mcperrors.NewNetworkError(inner, "http://badhost.example.com/path", "")
	want := `DNS Error: Cannot resolve hostname "badhost.example.com"`
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestNewNetworkError_DNSFailure_EAI_NONAME(t *testing.T) {
	inner := errors.New("EAI_NONAME: name does not resolve")
	err := mcperrors.NewNetworkError(inner, "http://nope.invalid/", "")
	want := `DNS Error: Cannot resolve hostname "nope.invalid"`
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestNewNetworkError_DNSFailure_UnparseableURL(t *testing.T) {
	inner := errors.New("ENOTFOUND: fail")
	err := mcperrors.NewNetworkError(inner, "://badurl", "")
	want := `DNS Error: Cannot resolve hostname "unknown"`
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestNewNetworkError_Certificate(t *testing.T) {
	inner := errors.New("tls: certificate verify failed")
	err := mcperrors.NewNetworkError(inner, "https://example.com", "")
	if !strings.HasPrefix(err.Error(), "SSL Error:") {
		t.Errorf("expected SSL error, got %q", err.Error())
	}
	want := "SSL Error: Certificate problem with website"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestNewNetworkError_Timeout(t *testing.T) {
	inner := errors.New("context deadline exceeded")
	err := mcperrors.NewNetworkError(inner, "http://slow.example.com", "http://slow.example.com")
	want := "Timeout Error: SearXNG server is too slow to respond"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestNewNetworkError_TimeoutKeyword(t *testing.T) {
	inner := errors.New("read timeout")
	err := mcperrors.NewNetworkError(inner, "http://slow.example.com", "")
	want := "Timeout Error: website is too slow to respond"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestNewNetworkError_Generic(t *testing.T) {
	inner := errors.New("some random network failure")
	err := mcperrors.NewNetworkError(inner, "http://example.com", "")
	want := "Network Error: some random network failure"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

type emptyErr struct{}

func (emptyErr) Error() string { return "" }

func TestNewNetworkError_EmptyMessage(t *testing.T) {
	err := mcperrors.NewNetworkError(emptyErr{}, "http://example.com", "")
	want := "Network Error: Connection failed"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestNewServerError_403_Website(t *testing.T) {
	err := mcperrors.NewServerError(403, "Forbidden", "", "")
	want := "Website Error (403): Access blocked (bot detection or geo-restriction)"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestNewServerError_403_SearXNG(t *testing.T) {
	err := mcperrors.NewServerError(403, "Forbidden", "", "http://localhost:8888")
	want := "SearXNG server Error (403): Authentication required or IP blocked"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestNewServerError_404_Website(t *testing.T) {
	err := mcperrors.NewServerError(404, "Not Found", "", "")
	want := "Website Error (404): Page not found"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestNewServerError_404_SearXNG(t *testing.T) {
	err := mcperrors.NewServerError(404, "Not Found", "", "http://localhost:8888")
	want := "SearXNG server Error (404): Search endpoint not found"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestNewServerError_429(t *testing.T) {
	err := mcperrors.NewServerError(429, "Too Many Requests", "", "")
	want := "Website Error (429): Rate limit exceeded"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestNewServerError_429_SearXNG(t *testing.T) {
	err := mcperrors.NewServerError(429, "Too Many Requests", "", "http://searxng:8080")
	want := "SearXNG server Error (429): Rate limit exceeded"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestNewServerError_500(t *testing.T) {
	err := mcperrors.NewServerError(500, "Internal Server Error", "", "")
	want := "Website Error (500): Internal server error"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestNewServerError_502_SearXNG(t *testing.T) {
	err := mcperrors.NewServerError(502, "Bad Gateway", "", "http://searxng:8080")
	want := "SearXNG server Error (502): Internal server error"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestNewServerError_Other(t *testing.T) {
	err := mcperrors.NewServerError(418, "I'm a Teapot", "", "")
	want := "Website Error (418): I'm a Teapot"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestNewJSONError_Short(t *testing.T) {
	body := `{"status":"error"}`
	err := mcperrors.NewJSONError(body)
	msg := err.Error()
	if !strings.HasPrefix(msg, "SearXNG Response Error: Invalid JSON format.") {
		t.Errorf("expected JSON error prefix, got %q", msg)
	}
	if !strings.Contains(msg, "status") {
		t.Errorf("expected body content in error, got %q", msg)
	}
}

func TestNewJSONError_TruncatesLongResponse(t *testing.T) {
	longBody := strings.Repeat("x", 200)
	err := mcperrors.NewJSONError(longBody)
	msg := err.Error()
	if strings.Contains(msg, strings.Repeat("x", 200)) {
		t.Error("long response should have been truncated")
	}
	preview := strings.Repeat("x", 100)
	if !strings.Contains(msg, preview) {
		t.Error("truncated preview should contain first 100 chars")
	}
}

func TestNewJSONError_ReplacesNewlines(t *testing.T) {
	body := "line1\nline2\nline3"
	err := mcperrors.NewJSONError(body)
	msg := err.Error()
	if strings.Contains(msg, "\n") {
		t.Error("newlines should be replaced with spaces")
	}
	if !strings.Contains(msg, "line1 line2 line3") {
		t.Errorf("expected newlines replaced, got %q", msg)
	}
}

func TestNewDataError(t *testing.T) {
	err := mcperrors.NewDataError()
	want := "SearXNG Data Error: Missing results array in response"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestNewNoResultsMessage(t *testing.T) {
	query := "golang testing"
	msg := mcperrors.NewNoResultsMessage(query)
	want := fmt.Sprintf(`No results found for %q. Try different search terms or check if SearXNG search engines are working.`, query)
	if msg != want {
		t.Errorf("got %q, want %q", msg, want)
	}
}

func TestNewURLFormatError(t *testing.T) {
	badURL := "not a url"
	err := mcperrors.NewURLFormatError(badURL)
	want := fmt.Sprintf("URL Format Error: Invalid URL %q", badURL)
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestNewContentError(t *testing.T) {
	err := mcperrors.NewContentError("failed to parse", "http://example.com/page")
	want := "Content Error: failed to parse (http://example.com/page)"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestNewConversionError(t *testing.T) {
	err := mcperrors.NewConversionError("http://example.com/article")
	want := "Conversion Error: Cannot convert HTML to Markdown (http://example.com/article)"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestNewTimeoutError_ExtractsHostname(t *testing.T) {
	err := mcperrors.NewTimeoutError(5000, "https://slow.example.com/path?q=1")
	want := "Timeout Error: slow.example.com took longer than 5000ms to respond"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestNewTimeoutError_UnparseableURL(t *testing.T) {
	err := mcperrors.NewTimeoutError(3000, "://bad")
	want := "Timeout Error: ://bad took longer than 3000ms to respond"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestNewEmptyContentWarning(t *testing.T) {
	msg := mcperrors.NewEmptyContentWarning("http://example.com/empty")
	want := "Content Warning: Page fetched but appears empty after conversion (http://example.com/empty). May contain only media or require JavaScript."
	if msg != want {
		t.Errorf("got %q, want %q", msg, want)
	}
}

func TestNewUnexpectedError(t *testing.T) {
	inner := errors.New("something went wrong")
	err := mcperrors.NewUnexpectedError(inner)
	want := "Unexpected Error: something went wrong"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestMCPSearXNGError_ImplementsError(t *testing.T) {
	var err error = mcperrors.NewConfigurationError("test")
	if err == nil {
		t.Fatal("MCPSearXNGError should implement error interface")
	}
	if err.Error() != "Configuration Error: test" {
		t.Errorf("Error() = %q, want %q", err.Error(), "Configuration Error: test")
	}
}
