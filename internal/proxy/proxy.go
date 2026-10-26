package proxy

import (
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/iamangus/mcp-searxng/internal/config"
	"golang.org/x/net/http/httpproxy"
)

type ProxyType int

const (
	Search ProxyType = iota
	URLReader
	Global
)

func ShouldBypassProxy(targetURL string) bool {
	noProxy := firstNonEmpty(
		os.Getenv("NO_PROXY"),
		os.Getenv("no_proxy"),
	)
	if noProxy == "" {
		return false
	}

	u, err := url.Parse(targetURL)
	if err != nil || u.Hostname() == "" {
		return false
	}
	f := (&httpproxy.Config{HTTPProxy: "http://proxy.invalid", HTTPSProxy: "http://proxy.invalid", NoProxy: noProxy}).ProxyFunc()
	p, err := f(u)
	return err == nil && p == nil
}

func GetProxyURL(proxyType ProxyType, targetURL string) string {
	isHTTPS := false
	if targetURL != "" {
		if u, err := url.Parse(targetURL); err == nil {
			isHTTPS = u.Scheme == "https"
		}
	}

	switch proxyType {
	case Search:
		if isHTTPS {
			return firstNonEmpty(
				os.Getenv("SEARCH_HTTPS_PROXY"),
				os.Getenv("SEARCH_HTTP_PROXY"),
				os.Getenv("search_https_proxy"),
				os.Getenv("search_http_proxy"),
				os.Getenv("HTTPS_PROXY"),
				os.Getenv("HTTP_PROXY"),
				os.Getenv("https_proxy"),
				os.Getenv("http_proxy"),
			)
		}
		return firstNonEmpty(
			os.Getenv("SEARCH_HTTP_PROXY"),
			os.Getenv("SEARCH_HTTPS_PROXY"),
			os.Getenv("search_http_proxy"),
			os.Getenv("search_https_proxy"),
			os.Getenv("HTTP_PROXY"),
			os.Getenv("HTTPS_PROXY"),
			os.Getenv("http_proxy"),
			os.Getenv("https_proxy"),
		)

	case URLReader:
		if isHTTPS {
			return firstNonEmpty(
				os.Getenv("URL_READER_HTTPS_PROXY"),
				os.Getenv("URL_READER_HTTP_PROXY"),
				os.Getenv("url_reader_https_proxy"),
				os.Getenv("url_reader_http_proxy"),
				os.Getenv("HTTPS_PROXY"),
				os.Getenv("HTTP_PROXY"),
				os.Getenv("https_proxy"),
				os.Getenv("http_proxy"),
			)
		}
		return firstNonEmpty(
			os.Getenv("URL_READER_HTTP_PROXY"),
			os.Getenv("URL_READER_HTTPS_PROXY"),
			os.Getenv("url_reader_http_proxy"),
			os.Getenv("url_reader_https_proxy"),
			os.Getenv("HTTP_PROXY"),
			os.Getenv("HTTPS_PROXY"),
			os.Getenv("http_proxy"),
			os.Getenv("https_proxy"),
		)

	default:
		if isHTTPS {
			return firstNonEmpty(
				os.Getenv("HTTPS_PROXY"),
				os.Getenv("HTTP_PROXY"),
				os.Getenv("https_proxy"),
				os.Getenv("http_proxy"),
			)
		}
		return firstNonEmpty(
			os.Getenv("HTTP_PROXY"),
			os.Getenv("HTTPS_PROXY"),
			os.Getenv("http_proxy"),
			os.Getenv("https_proxy"),
		)
	}
}

func ValidateProxyURL(proxyURL string) (string, error) {
	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return "", fmt.Errorf("invalid proxy URL: %s", proxyURL)
	}

	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return "", fmt.Errorf("unsupported proxy protocol: %s (only HTTP and HTTPS supported)", parsed.Scheme)
	}

	var auth string
	if parsed.User != nil {
		auth = parsed.User.String() + "@"
	}
	return fmt.Sprintf("%s://%s%s", parsed.Scheme, auth, parsed.Host), nil
}

func GetProxyFunc(proxyType ProxyType) func(*http.Request) (*url.URL, error) {
	return func(req *http.Request) (*url.URL, error) {
		targetStr := req.URL.String()
		if ShouldBypassProxy(targetStr) {
			return nil, nil
		}

		proxyRaw := GetProxyURL(proxyType, targetStr)
		if proxyRaw == "" {
			return nil, nil
		}

		proxyURL, err := url.Parse(proxyRaw)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL: %w", err)
		}

		return proxyURL, nil
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// NewFunc snapshots validated configuration; no environment is consulted per request.
// NO_PROXY follows Go's standard CIDR, IPv6, domain and host:port semantics.
func NewFunc(cfg *config.Config, kind ProxyType) func(*http.Request) (*url.URL, error) {
	httpProxy, httpsProxy := cfg.HTTPProxy, cfg.HTTPSProxy
	if kind == Search {
		httpProxy = firstNonEmpty(cfg.SearchHTTPProxy, cfg.SearchHTTPSProxy, httpProxy, httpsProxy)
		httpsProxy = firstNonEmpty(cfg.SearchHTTPSProxy, cfg.SearchHTTPProxy, cfg.HTTPSProxy, cfg.HTTPProxy)
	} else if kind == URLReader {
		httpProxy = firstNonEmpty(cfg.URLReaderHTTPProxy, cfg.URLReaderHTTPSProxy, httpProxy, httpsProxy)
		httpsProxy = firstNonEmpty(cfg.URLReaderHTTPSProxy, cfg.URLReaderHTTPProxy, cfg.HTTPSProxy, cfg.HTTPProxy)
	} else {
		httpProxy = firstNonEmpty(httpProxy, httpsProxy)
		httpsProxy = firstNonEmpty(httpsProxy, cfg.HTTPProxy)
	}
	f := (&httpproxy.Config{HTTPProxy: httpProxy, HTTPSProxy: httpsProxy, NoProxy: cfg.NoProxy}).ProxyFunc()
	return func(r *http.Request) (*url.URL, error) { return f(r.URL) }
}
