package config_test

import (
	"os"
	"strings"
	"testing"

	"github.com/iamangus/mcp-searxng/internal/config"
)

// Configuration tests must not depend on the developer's or CI runner's environment.
func TestMain(m *testing.M) {
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.Contains(strings.ToUpper(key), "PROXY") || strings.HasPrefix(key, "MCP_") || strings.HasPrefix(key, "SEARXNG_") || strings.HasPrefix(key, "AUTH_") {
			os.Unsetenv(key)
		}
	}
	os.Exit(m.Run())
}

func TestValidateRejectsHostlessAndCredentialURLs(t *testing.T) {
	for _, raw := range []string{"http:", "https:///path", "https://user:secret@example.com", "https://example.com/#fragment"} {
		if (&config.Config{SearXNGURL: raw}).Validate() == nil {
			t.Errorf("accepted %q", raw)
		}
	}
}

func TestNetworkDefaultsAndAllowLists(t *testing.T) {
	t.Setenv("MCP_HTTP_PORT", "3000")
	t.Setenv("SEARXNG_URL", "http://localhost:8080")
	cfg, err := config.Load()
	if err != nil || cfg.HTTPHost != "127.0.0.1" || len(cfg.AllowedHosts) != 3 {
		t.Fatalf("unexpected defaults: %+v, %v", cfg, err)
	}
	t.Setenv("MCP_ALLOWED_ORIGINS", "https://mcp.example.com/path")
	if _, err := config.Load(); err == nil {
		t.Fatal("accepted origin with path")
	}
	t.Setenv("MCP_ALLOWED_ORIGINS", "https://mcp.example.com")
	t.Setenv("MCP_ALLOWED_HOSTS", "mcp.example.com, localhost")
	cfg, err = config.Load()
	if err != nil || len(cfg.AllowedHosts) != 2 {
		t.Fatalf("unexpected allow lists: %+v, %v", cfg, err)
	}
}

func TestLoad_MissingHTTPPort(t *testing.T) {
	t.Setenv("SEARXNG_URL", "http://localhost:8080")
	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error when MCP_HTTP_PORT is missing")
	}
}

func TestLoad_InvalidHTTPPort(t *testing.T) {
	tests := []struct {
		name string
		port string
	}{
		{"not a number", "abc"},
		{"zero", "0"},
		{"negative", "-1"},
		{"too large", "65536"},
		{"float", "80.5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("MCP_HTTP_PORT", tt.port)
			t.Setenv("SEARXNG_URL", "http://localhost:8080")
			_, err := config.Load()
			if err == nil {
				t.Fatalf("expected error for MCP_HTTP_PORT=%q", tt.port)
			}
		})
	}
}

func TestLoad_MissingSearXNGURL(t *testing.T) {
	t.Setenv("MCP_HTTP_PORT", "3000")
	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error when SEARXNG_URL is missing")
	}
}

func TestLoad_ValidConfig(t *testing.T) {
	t.Setenv("MCP_HTTP_PORT", "3000")
	t.Setenv("SEARXNG_URL", "http://localhost:8080")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTPPort != 3000 {
		t.Errorf("HTTPPort = %d, want 3000", cfg.HTTPPort)
	}
	if cfg.SearXNGURL != "http://localhost:8080" {
		t.Errorf("SearXNGURL = %q, want %q", cfg.SearXNGURL, "http://localhost:8080")
	}
}

func TestLoad_ValidConfigWithHTTPS(t *testing.T) {
	t.Setenv("MCP_HTTP_PORT", "443")
	t.Setenv("SEARXNG_URL", "https://search.example.com")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTPPort != 443 {
		t.Errorf("HTTPPort = %d, want 443", cfg.HTTPPort)
	}
}

func TestValidate_InvalidURLProtocol(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"ftp scheme", "ftp://localhost:8080"},
		{"no scheme", "localhost:8080"},
		{"empty", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{SearXNGURL: tt.url}
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("expected error for SearXNGURL=%q", tt.url)
			}
		})
	}
}

func TestValidate_ValidURL(t *testing.T) {
	cfg := &config.Config{SearXNGURL: "http://localhost:8080"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg = &config.Config{SearXNGURL: "https://search.example.com"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_AuthUsernameWithoutPassword(t *testing.T) {
	cfg := &config.Config{
		SearXNGURL:   "http://localhost:8080",
		AuthUsername: "admin",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error when AUTH_USERNAME set without AUTH_PASSWORD")
	}
}

func TestValidate_AuthPasswordWithoutUsername(t *testing.T) {
	cfg := &config.Config{
		SearXNGURL:   "http://localhost:8080",
		AuthPassword: "secret",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error when AUTH_PASSWORD set without AUTH_USERNAME")
	}
}

func TestValidate_BothAuthSet(t *testing.T) {
	cfg := &config.Config{
		SearXNGURL:   "http://localhost:8080",
		AuthUsername: "admin",
		AuthPassword: "secret",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHasAuth(t *testing.T) {
	tests := []struct {
		name     string
		username string
		password string
		want     bool
	}{
		{"both set", "admin", "secret", true},
		{"only username", "admin", "", false},
		{"only password", "", "secret", false},
		{"neither set", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				AuthUsername: tt.username,
				AuthPassword: tt.password,
			}
			if got := cfg.HasAuth(); got != tt.want {
				t.Errorf("HasAuth() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasProxy(t *testing.T) {
	tests := []struct {
		name       string
		httpProxy  string
		httpsProxy string
		want       bool
	}{
		{"both set", "http://proxy:8080", "http://proxy:8080", true},
		{"only http", "http://proxy:8080", "", true},
		{"only https", "", "http://proxy:8080", true},
		{"neither set", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				HTTPProxy:  tt.httpProxy,
				HTTPSProxy: tt.httpsProxy,
			}
			if got := cfg.HasProxy(); got != tt.want {
				t.Errorf("HasProxy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasNoProxy(t *testing.T) {
	cfg := &config.Config{NoProxy: "localhost,127.0.0.1"}
	if !cfg.HasNoProxy() {
		t.Error("HasNoProxy() = false, want true")
	}

	cfg = &config.Config{NoProxy: ""}
	if cfg.HasNoProxy() {
		t.Error("HasNoProxy() = true, want false")
	}
}

func TestURLReaderAgent_CustomSet(t *testing.T) {
	cfg := &config.Config{
		UserAgent:          "default-agent",
		URLReaderUserAgent: "custom-agent",
	}
	if got := cfg.URLReaderAgent(); got != "custom-agent" {
		t.Errorf("URLReaderAgent() = %q, want %q", got, "custom-agent")
	}
}

func TestURLReaderAgent_Fallback(t *testing.T) {
	cfg := &config.Config{
		UserAgent:          "default-agent",
		URLReaderUserAgent: "",
	}
	if got := cfg.URLReaderAgent(); got != "default-agent" {
		t.Errorf("URLReaderAgent() = %q, want %q", got, "default-agent")
	}
}

func TestURLReaderAgent_BothEmpty(t *testing.T) {
	cfg := &config.Config{}
	if got := cfg.URLReaderAgent(); got != "" {
		t.Errorf("URLReaderAgent() = %q, want empty string", got)
	}
}

func TestFirstNonEmpty_ViaProxyFallback(t *testing.T) {
	t.Setenv("MCP_HTTP_PORT", "3000")
	t.Setenv("SEARXNG_URL", "http://localhost:8080")
	t.Setenv("http_proxy", "http://lower:8080")
	t.Setenv("HTTP_PROXY", "http://upper:8080")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTPProxy != "http://upper:8080" {
		t.Errorf("HTTPProxy = %q, want uppercase to take priority", cfg.HTTPProxy)
	}
}

func TestFirstNonEmpty_SecondWins(t *testing.T) {
	t.Setenv("MCP_HTTP_PORT", "3000")
	t.Setenv("SEARXNG_URL", "http://localhost:8080")
	t.Setenv("http_proxy", "http://lower:8080")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTPProxy != "http://lower:8080" {
		t.Errorf("HTTPProxy = %q, want lowercase fallback", cfg.HTTPProxy)
	}
}

func TestFirstNonEmpty_NeitherSet(t *testing.T) {
	t.Setenv("MCP_HTTP_PORT", "3000")
	t.Setenv("SEARXNG_URL", "http://localhost:8080")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTPProxy != "" {
		t.Errorf("HTTPProxy = %q, want empty", cfg.HTTPProxy)
	}
}

func TestLoad_ProxyEnvVars(t *testing.T) {
	t.Setenv("MCP_HTTP_PORT", "3000")
	t.Setenv("SEARXNG_URL", "http://localhost:8080")
	t.Setenv("HTTP_PROXY", "http://proxy:8080")
	t.Setenv("HTTPS_PROXY", "https://proxy:8443")
	t.Setenv("NO_PROXY", "localhost,127.0.0.1")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTPProxy != "http://proxy:8080" {
		t.Errorf("HTTPProxy = %q, want %q", cfg.HTTPProxy, "http://proxy:8080")
	}
	if cfg.HTTPSProxy != "https://proxy:8443" {
		t.Errorf("HTTPSProxy = %q, want %q", cfg.HTTPSProxy, "https://proxy:8443")
	}
	if cfg.NoProxy != "localhost,127.0.0.1" {
		t.Errorf("NoProxy = %q, want %q", cfg.NoProxy, "localhost,127.0.0.1")
	}
}

func TestLoad_LowercaseProxyFallback(t *testing.T) {
	t.Setenv("MCP_HTTP_PORT", "3000")
	t.Setenv("SEARXNG_URL", "http://localhost:8080")
	t.Setenv("http_proxy", "http://lowercase:8080")
	t.Setenv("https_proxy", "https://lowercase:8443")
	t.Setenv("no_proxy", "localhost")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTPProxy != "http://lowercase:8080" {
		t.Errorf("HTTPProxy = %q, want %q", cfg.HTTPProxy, "http://lowercase:8080")
	}
	if cfg.HTTPSProxy != "https://lowercase:8443" {
		t.Errorf("HTTPSProxy = %q, want %q", cfg.HTTPSProxy, "https://lowercase:8443")
	}
	if cfg.NoProxy != "localhost" {
		t.Errorf("NoProxy = %q, want %q", cfg.NoProxy, "localhost")
	}
}
