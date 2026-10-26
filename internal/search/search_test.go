package search

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/iamangus/mcp-searxng/internal/config"
	"github.com/iamangus/mcp-searxng/internal/netclient"
)

func params() SearchParams {
	return SearchParams{Query: "hello", PageNo: 1, Language: "en", SafeSearch: 1}
}

func TestSearchParametersAndStructuredResults(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/prefix/search" || q.Get("q") != "hello" || q.Get("format") != "json" || q.Get("safesearch") != "1" || q.Get("language") != "en" || q.Get("pageno") != "1" {
			t.Error(r.URL)
		}
		if u, p, ok := r.BasicAuth(); !ok || u != "user" || p != "pass" {
			t.Error("missing auth")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"results":[{"title":"Test","url":"https://example.com","content":"Example","score":1}],"unresponsive_engines":[["google","timeout"]],"answers":["42"]}`)
	}))
	defer s.Close()
	c := NewClient(&config.Config{SearXNGURL: s.URL + "/prefix", AuthUsername: "user", AuthPassword: "pass"})
	defer c.Close()
	r, err := c.Search(context.Background(), params())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Results) != 1 || len(r.UnresponsiveEngines) != 1 || len(r.Answers) != 1 || r.PageNo != 1 || !strings.Contains(r.Text(), "Test") {
		t.Fatalf("%+v", r)
	}
}

func TestSearchFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"missing", `{}`, 200}, {"malformed", `<html>secret upstream response</html>`, 200}, {"empty", `{"results":[]}`, 200}, {"status", "secret", 503}, {"oversize", strings.Repeat("x", netclient.MaxBodyBytes+1), 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); io.WriteString(w, tc.body) }))
			defer s.Close()
			c := NewClient(&config.Config{SearXNGURL: s.URL})
			defer c.Close()
			r, err := c.Search(context.Background(), params())
			if tc.name == "empty" {
				if err != nil || len(r.Results) != 0 {
					t.Fatal(r, err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error")
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatal("leaked response")
			}
		})
	}
}

func TestSearchCancellationAndRedirectCredentials(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer s.Close()
	c := NewClient(&config.Config{SearXNGURL: s.URL})
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := c.Search(ctx, params()); err == nil {
		t.Fatal("cancellation ignored")
	}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("redirect reached another origin") }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer redirect.Close()
	c2 := NewClient(&config.Config{SearXNGURL: redirect.URL, AuthUsername: "secret", AuthPassword: "secret"})
	defer c2.Close()
	if _, err := c2.Search(context.Background(), params()); err == nil {
		t.Fatal("cross-origin redirect accepted")
	}
}

func TestSearchOutputBound(t *testing.T) {
	results := make([]SearXNGResult, 100)
	for i := range results {
		results[i] = SearXNGResult{Title: strings.Repeat("x", 600), Content: strings.Repeat("x", 2000)}
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(SearXNGResponse{Results: results})
	}))
	defer s.Close()
	c := NewClient(&config.Config{SearXNGURL: s.URL})
	defer c.Close()
	r, err := c.Search(context.Background(), params())
	if err != nil {
		t.Fatal(err)
	}
	if !r.Truncated || len(r.Results) != 20 || len(r.Results[0].Content) != 1000 {
		t.Fatal("missing truncation")
	}
}
