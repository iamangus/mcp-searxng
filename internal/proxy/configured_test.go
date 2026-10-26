package proxy_test

import (
	"github.com/iamangus/mcp-searxng/internal/config"
	"github.com/iamangus/mcp-searxng/internal/proxy"
	"net/http"
	"testing"
)

func TestConfiguredProxySnapshotAndStandardBypass(t *testing.T) {
	cfg := &config.Config{HTTPProxy: "http://global:8080", SearchHTTPProxy: "http://search:8080", NoProxy: "10.0.0.0/8,2001:db8::1,example.com:443"}
	f := proxy.NewFunc(cfg, proxy.Search)
	t.Setenv("SEARCH_HTTP_PROXY", "http://unrelated:8080")
	for _, tc := range []struct {
		url    string
		bypass bool
	}{{"http://10.1.2.3", true}, {"http://[2001:db8::1]", true}, {"https://example.com", true}, {"http://example.com", false}, {"https://public.example", false}} {
		r, _ := http.NewRequest("GET", tc.url, nil)
		p, err := f(r)
		if err != nil {
			t.Fatal(err)
		}
		if (p == nil) != tc.bypass {
			t.Fatalf("%s: proxy=%v", tc.url, p)
		}
		if p != nil && p.String() != "http://search:8080" {
			t.Fatal(p)
		}
	}
}
