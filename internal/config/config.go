package config

import (
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const Version = "1.0.0"

type Config struct {
	SearXNGURL          string
	HTTPPort            int
	AuthUsername        string
	AuthPassword        string
	UserAgent           string
	URLReaderUserAgent  string
	HTTPProxy           string
	HTTPSProxy          string
	NoProxy             string
	SearchHTTPProxy     string
	SearchHTTPSProxy    string
	URLReaderHTTPProxy  string
	URLReaderHTTPSProxy string
	HTTPHost            string
	AllowedHosts        []string
	AllowedOrigins      []string
	LogLevel            string
}

func Load() (*Config, error) {
	cfg := &Config{}

	cfg.SearXNGURL = os.Getenv("SEARXNG_URL")

	portStr := os.Getenv("MCP_HTTP_PORT")
	if portStr == "" {
		return nil, fmt.Errorf("MCP_HTTP_PORT is required")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid MCP_HTTP_PORT: %s (must be 1-65535)", portStr)
	}
	cfg.HTTPPort = port
	cfg.HTTPHost = firstNonEmpty(os.Getenv("MCP_HTTP_HOST"), "127.0.0.1")
	cfg.AllowedHosts = splitList(os.Getenv("MCP_ALLOWED_HOSTS"))
	cfg.AllowedOrigins = splitList(os.Getenv("MCP_ALLOWED_ORIGINS"))
	cfg.LogLevel = firstNonEmpty(os.Getenv("LOG_LEVEL"), "info")
	if len(cfg.AllowedHosts) == 0 {
		cfg.AllowedHosts = []string{"localhost", "127.0.0.1", "::1"}
	}

	cfg.AuthUsername = os.Getenv("AUTH_USERNAME")
	cfg.AuthPassword = os.Getenv("AUTH_PASSWORD")
	cfg.UserAgent = os.Getenv("USER_AGENT")
	cfg.URLReaderUserAgent = os.Getenv("URL_READER_USER_AGENT")

	cfg.HTTPProxy = firstNonEmpty(
		os.Getenv("HTTP_PROXY"),
		os.Getenv("http_proxy"),
	)
	cfg.HTTPSProxy = firstNonEmpty(
		os.Getenv("HTTPS_PROXY"),
		os.Getenv("https_proxy"),
	)
	cfg.NoProxy = firstNonEmpty(
		os.Getenv("NO_PROXY"),
		os.Getenv("no_proxy"),
	)
	cfg.SearchHTTPProxy = firstNonEmpty(
		os.Getenv("SEARCH_HTTP_PROXY"),
		os.Getenv("search_http_proxy"),
	)
	cfg.SearchHTTPSProxy = firstNonEmpty(
		os.Getenv("SEARCH_HTTPS_PROXY"),
		os.Getenv("search_https_proxy"),
	)
	cfg.URLReaderHTTPProxy = firstNonEmpty(
		os.Getenv("URL_READER_HTTP_PROXY"),
		os.Getenv("url_reader_http_proxy"),
	)
	cfg.URLReaderHTTPSProxy = firstNonEmpty(
		os.Getenv("URL_READER_HTTPS_PROXY"),
		os.Getenv("url_reader_https_proxy"),
	)

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	var issues []string
	if c.LogLevel != "" && c.LogLevel != "debug" && c.LogLevel != "info" && c.LogLevel != "warning" && c.LogLevel != "error" {
		issues = append(issues, "LOG_LEVEL must be debug, info, warning or error")
	}

	if c.SearXNGURL == "" {
		issues = append(issues, "SEARXNG_URL not set")
	} else {
		u, err := url.Parse(c.SearXNGURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
			issues = append(issues, "SEARXNG_URL must be an HTTP(S) URL with a hostname and no credentials or fragment")
		}
	}
	for _, raw := range []string{c.HTTPProxy, c.HTTPSProxy, c.SearchHTTPProxy, c.SearchHTTPSProxy, c.URLReaderHTTPProxy, c.URLReaderHTTPSProxy} {
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
			issues = append(issues, "proxy must be an HTTP(S) URL with a hostname")
		}
	}
	for _, origin := range c.AllowedOrigins {
		u, err := url.Parse(origin)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
			issues = append(issues, "MCP_ALLOWED_ORIGINS must contain exact HTTP(S) origins without paths")
		}
	}
	for _, host := range c.AllowedHosts {
		_, ipErr := netip.ParseAddr(host)
		if strings.ContainsAny(host, "/@*[] \t\r\n") || host == "" || (strings.Contains(host, ":") && ipErr != nil) {
			issues = append(issues, "MCP_ALLOWED_HOSTS must contain exact hostnames or IP addresses")
		}
	}

	if c.AuthUsername != "" && c.AuthPassword == "" {
		issues = append(issues, "AUTH_USERNAME set but AUTH_PASSWORD missing")
	} else if c.AuthUsername == "" && c.AuthPassword != "" {
		issues = append(issues, "AUTH_PASSWORD set but AUTH_USERNAME missing")
	}

	if len(issues) > 0 {
		return fmt.Errorf("configuration issues: %s. Set SEARXNG_URL (e.g., http://localhost:8080 or https://search.example.com)",
			strings.Join(issues, ", "))
	}

	return nil
}

func (c *Config) HasAuth() bool {
	return c.AuthUsername != "" && c.AuthPassword != ""
}

func (c *Config) HasProxy() bool {
	return c.HTTPProxy != "" || c.HTTPSProxy != "" || c.SearchHTTPProxy != "" || c.SearchHTTPSProxy != "" || c.URLReaderHTTPProxy != "" || c.URLReaderHTTPSProxy != ""
}

func splitList(raw string) []string {
	var out []string
	for _, value := range strings.Split(raw, ",") {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func (c *Config) HasNoProxy() bool {
	return c.NoProxy != ""
}

func (c *Config) URLReaderAgent() string {
	if c.URLReaderUserAgent != "" {
		return c.URLReaderUserAgent
	}
	return c.UserAgent
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
