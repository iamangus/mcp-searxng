package proxy_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/iamangus/mcp-searxng/internal/proxy"
)

func TestShouldBypassProxy_NoProxyNotSet(t *testing.T) {
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")

	got := proxy.ShouldBypassProxy("http://example.com/path")
	if got {
		t.Error("expected false when NO_PROXY is not set")
	}
}

func TestShouldBypassProxy_Wildcard(t *testing.T) {
	t.Setenv("NO_PROXY", "*")

	if !proxy.ShouldBypassProxy("http://anything.example.com/") {
		t.Error("expected true for wildcard NO_PROXY")
	}
}

func TestShouldBypassProxy_ExactHostnameMatch(t *testing.T) {
	t.Setenv("NO_PROXY", "example.com,other.com")

	if !proxy.ShouldBypassProxy("http://example.com/path") {
		t.Error("expected true for exact hostname match")
	}

	if !proxy.ShouldBypassProxy("http://other.com/") {
		t.Error("expected true for exact hostname match (second entry)")
	}
}

func TestShouldBypassProxy_DomainSuffixWithLeadingDot(t *testing.T) {
	t.Setenv("NO_PROXY", ".example.com")

	if !proxy.ShouldBypassProxy("http://sub.example.com/path") {
		t.Error("expected true for domain suffix match with leading dot")
	}
}

func TestShouldBypassProxy_DomainSuffixWithoutLeadingDot(t *testing.T) {
	t.Setenv("NO_PROXY", "example.com")

	if !proxy.ShouldBypassProxy("http://sub.example.com/path") {
		t.Error("expected true for domain suffix match without leading dot")
	}

	if !proxy.ShouldBypassProxy("http://example.com/path") {
		t.Error("expected true for exact match when pattern has no leading dot")
	}
}

func TestShouldBypassProxy_NonMatchingHost(t *testing.T) {
	t.Setenv("NO_PROXY", "example.com,other.com")

	if proxy.ShouldBypassProxy("http://unrelated.com/path") {
		t.Error("expected false for non-matching host")
	}
}

func TestShouldBypassProxy_LowercaseEnv(t *testing.T) {
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "example.com")

	if !proxy.ShouldBypassProxy("http://example.com/path") {
		t.Error("expected true when no_proxy (lowercase) is set")
	}
}

func TestGetProxyURL_SearchHTTP(t *testing.T) {
	t.Setenv("SEARCH_HTTP_PROXY", "http://search-proxy:8080")

	got := proxy.GetProxyURL(proxy.Search, "http://target.example.com")
	if got != "http://search-proxy:8080" {
		t.Errorf("expected http://search-proxy:8080, got %s", got)
	}
}

func TestGetProxyURL_SearchHTTPS(t *testing.T) {
	t.Setenv("SEARCH_HTTPS_PROXY", "http://search-https-proxy:8080")

	got := proxy.GetProxyURL(proxy.Search, "https://target.example.com")
	if got != "http://search-https-proxy:8080" {
		t.Errorf("expected http://search-https-proxy:8080, got %s", got)
	}
}

func TestGetProxyURL_SearchHTTPFallbackToHTTPS(t *testing.T) {
	t.Setenv("SEARCH_HTTPS_PROXY", "")
	t.Setenv("SEARCH_HTTP_PROXY", "")
	t.Setenv("search_https_proxy", "")
	t.Setenv("search_http_proxy", "")
	t.Setenv("HTTPS_PROXY", "http://global-https:8080")

	got := proxy.GetProxyURL(proxy.Search, "http://target.example.com")
	if got != "http://global-https:8080" {
		t.Errorf("expected fallback to global HTTPS_PROXY, got %s", got)
	}
}

func TestGetProxyURL_URLReaderHTTP(t *testing.T) {
	t.Setenv("URL_READER_HTTP_PROXY", "http://urlreader-proxy:8080")

	got := proxy.GetProxyURL(proxy.URLReader, "http://target.example.com")
	if got != "http://urlreader-proxy:8080" {
		t.Errorf("expected http://urlreader-proxy:8080, got %s", got)
	}
}

func TestGetProxyURL_URLReaderHTTPS(t *testing.T) {
	t.Setenv("URL_READER_HTTPS_PROXY", "http://urlreader-https-proxy:8080")

	got := proxy.GetProxyURL(proxy.URLReader, "https://target.example.com")
	if got != "http://urlreader-https-proxy:8080" {
		t.Errorf("expected http://urlreader-https-proxy:8080, got %s", got)
	}
}

func TestGetProxyURL_GlobalHTTP(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://global-proxy:8080")

	got := proxy.GetProxyURL(proxy.Global, "http://target.example.com")
	if got != "http://global-proxy:8080" {
		t.Errorf("expected http://global-proxy:8080, got %s", got)
	}
}

func TestGetProxyURL_GlobalHTTPS(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://global-https-proxy:8080")

	got := proxy.GetProxyURL(proxy.Global, "https://target.example.com")
	if got != "http://global-https-proxy:8080" {
		t.Errorf("expected http://global-https-proxy:8080, got %s", got)
	}
}

func TestGetProxyURL_FallbackFromTypeSpecificToGlobal(t *testing.T) {
	t.Setenv("SEARCH_HTTP_PROXY", "")
	t.Setenv("SEARCH_HTTPS_PROXY", "")
	t.Setenv("search_http_proxy", "")
	t.Setenv("search_https_proxy", "")
	t.Setenv("HTTP_PROXY", "http://global-fallback:3128")

	got := proxy.GetProxyURL(proxy.Search, "http://target.example.com")
	if got != "http://global-fallback:3128" {
		t.Errorf("expected http://global-fallback:3128, got %s", got)
	}
}

func TestGetProxyURL_EmptyWhenNothingSet(t *testing.T) {
	got := proxy.GetProxyURL(proxy.Global, "http://target.example.com")
	if got != "" {
		t.Errorf("expected empty string, got %s", got)
	}
}

func TestValidateProxyURL_ValidHTTP(t *testing.T) {
	got, err := proxy.ValidateProxyURL("http://proxy.example.com:8080")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "http://proxy.example.com:8080" {
		t.Errorf("expected http://proxy.example.com:8080, got %s", got)
	}
}

func TestValidateProxyURL_ValidHTTPS(t *testing.T) {
	got, err := proxy.ValidateProxyURL("https://proxy.example.com:8080")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "https://proxy.example.com:8080" {
		t.Errorf("expected https://proxy.example.com:8080, got %s", got)
	}
}

func TestValidateProxyURL_WithAuth(t *testing.T) {
	got, err := proxy.ValidateProxyURL("http://user:pass@proxy.example.com:8080")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "http://user:pass@proxy.example.com:8080" {
		t.Errorf("expected http://user:pass@proxy.example.com:8080, got %s", got)
	}
}

func TestValidateProxyURL_RejectsFTP(t *testing.T) {
	_, err := proxy.ValidateProxyURL("ftp://proxy.example.com:21")
	if err == nil {
		t.Error("expected error for ftp protocol")
	}
}

func TestValidateProxyURL_RejectsSOCKS(t *testing.T) {
	_, err := proxy.ValidateProxyURL("socks5://proxy.example.com:1080")
	if err == nil {
		t.Error("expected error for socks5 protocol")
	}
}

func TestValidateProxyURL_RejectsInvalidURL(t *testing.T) {
	_, err := proxy.ValidateProxyURL("://invalid")
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestGetProxyFunc_ResolvesCorrectly(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://func-proxy:8080")
	t.Setenv("NO_PROXY", "")

	fn := proxy.GetProxyFunc(proxy.Global)
	if fn == nil {
		t.Fatal("expected non-nil function")
	}

	req := &http.Request{
		URL: &url.URL{
			Scheme: "http",
			Host:   "target.example.com",
		},
	}

	resolved, err := fn(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved == nil {
		t.Fatal("expected proxy URL, got nil")
	}
	if resolved.String() != "http://func-proxy:8080" {
		t.Errorf("expected http://func-proxy:8080, got %s", resolved.String())
	}
}

func TestGetProxyFunc_BypassesWhenNoProxySet(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://func-proxy:8080")
	t.Setenv("NO_PROXY", "target.example.com")

	fn := proxy.GetProxyFunc(proxy.Global)

	req := &http.Request{
		URL: &url.URL{
			Scheme: "http",
			Host:   "target.example.com",
		},
	}

	resolved, err := fn(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved != nil {
		t.Errorf("expected nil (bypass), got %s", resolved.String())
	}
}

func TestGetProxyFunc_ReturnsNilWhenNoProxyConfigured(t *testing.T) {
	fn := proxy.GetProxyFunc(proxy.Global)
	if fn == nil {
		t.Fatal("expected non-nil function")
	}

	req := &http.Request{
		URL: &url.URL{
			Scheme: "http",
			Host:   "target.example.com",
		},
	}

	resolved, err := fn(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved != nil {
		t.Errorf("expected nil when no proxy configured, got %s", resolved.String())
	}
}

func TestShouldBypassProxy_InvalidURL(t *testing.T) {
	t.Setenv("NO_PROXY", "example.com")

	if proxy.ShouldBypassProxy("://invalid") {
		t.Error("expected false for invalid URL")
	}
}

func TestShouldBypassProxy_EmptyPatterns(t *testing.T) {
	t.Setenv("NO_PROXY", ",,example.com,,")

	if !proxy.ShouldBypassProxy("http://example.com/path") {
		t.Error("expected true despite empty patterns in list")
	}
}

func TestGetProxyURL_SearchHTTPPrefersSearchSpecific(t *testing.T) {
	t.Setenv("SEARCH_HTTP_PROXY", "http://search-specific:9090")
	t.Setenv("HTTP_PROXY", "http://global:8080")

	got := proxy.GetProxyURL(proxy.Search, "http://target.example.com")
	if got != "http://search-specific:9090" {
		t.Errorf("expected search-specific proxy, got %s", got)
	}
}

func TestGetProxyURL_URLReaderFallbackToGlobal(t *testing.T) {
	t.Setenv("URL_READER_HTTP_PROXY", "")
	t.Setenv("URL_READER_HTTPS_PROXY", "")
	t.Setenv("url_reader_http_proxy", "")
	t.Setenv("url_reader_https_proxy", "")
	t.Setenv("HTTP_PROXY", "http://global-for-urlreader:3128")

	got := proxy.GetProxyURL(proxy.URLReader, "http://target.example.com")
	if got != "http://global-for-urlreader:3128" {
		t.Errorf("expected http://global-for-urlreader:3128, got %s", got)
	}
}

func TestValidateProxyURL_StripsPathAndQuery(t *testing.T) {
	got, err := proxy.ValidateProxyURL("http://proxy.example.com:8080/some/path?query=1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "http://proxy.example.com:8080" {
		t.Errorf("expected path/query stripped, got %s", got)
	}
}
