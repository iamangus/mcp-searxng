package proxy

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
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

	if strings.TrimSpace(noProxy) == "*" {
		return true
	}

	u, err := url.Parse(targetURL)
	if err != nil {
		return false
	}
	hostname := strings.ToLower(u.Hostname())

	patterns := strings.Split(noProxy, ",")
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		p = strings.ToLower(p)

		if hostname == p {
			return true
		}

		if strings.HasPrefix(p, ".") && strings.HasSuffix(hostname, p) {
			return true
		}

		if !strings.HasPrefix(p, ".") {
			if hostname == p {
				return true
			}
			if strings.HasSuffix(hostname, "."+p) {
				return true
			}
		}
	}

	return false
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

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
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
