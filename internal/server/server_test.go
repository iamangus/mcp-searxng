package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iamangus/mcp-searxng/internal/config"
)

func testHandler(t *testing.T) http.Handler {
	t.Helper()
	h, cleanup, err := NewHTTPHandler(&config.Config{SearXNGURL: "https://search.example.com", AllowedHosts: []string{"mcp.example.com"}, AllowedOrigins: []string{"https://mcp.example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	return h
}

func request(method string, params map[string]any) *http.Request {
	if params == nil {
		params = make(map[string]any)
	}
	params["_meta"] = map[string]any{"io.modelcontextprotocol/protocolVersion": "2026-07-28", "io.modelcontextprotocol/clientCapabilities": map[string]any{}}
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": "test", "method": method, "params": params})
	r := httptest.NewRequest("POST", "http://mcp.example.com/mcp", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	r.Header.Set("MCP-Protocol-Version", "2026-07-28")
	r.Header.Set("Mcp-Method", method)
	if name, ok := params["name"].(string); ok {
		r.Header.Set("Mcp-Name", name)
	}
	if uri, ok := params["uri"].(string); ok {
		r.Header.Set("Mcp-Name", uri)
	}
	return r
}

func serve(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestModernMethodsWithoutInitialization(t *testing.T) {
	h := testHandler(t)
	for _, method := range []string{"server/discover", "tools/list", "resources/list", "resources/templates/list"} {
		w := serve(h, request(method, nil))
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", method, w.Code, w.Body.String())
		}
		var response struct {
			Result map[string]any `json:"result"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if response.Result["resultType"] != "complete" {
			t.Fatalf("%s missing resultType: %s", method, w.Body.String())
		}
		if w.Header().Get("Mcp-Session-Id") != "" {
			t.Fatal("session issued")
		}
		if method == "server/discover" {
			versions := response.Result["supportedVersions"].([]any)
			if len(versions) != 1 || versions[0] != "2026-07-28" {
				t.Fatal(versions)
			}
			if _, ok := response.Result["capabilities"].(map[string]any)["logging"]; ok {
				t.Fatal("logging falsely advertised")
			}
		}
	}
	for _, uri := range []string{"help://usage-guide", "config://server-config"} {
		w := serve(h, request("resources/read", map[string]any{"uri": uri}))
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", uri, w.Code, w.Body.String())
		}
	}
}

func TestMetadataAndHeaderErrors(t *testing.T) {
	h := testHandler(t)
	tests := []struct {
		name         string
		edit         func(*http.Request)
		status, code int
	}{
		{"missing version", func(r *http.Request) { r.Header.Del("MCP-Protocol-Version") }, 400, -32020},
		{"legacy", func(r *http.Request) { r.Header.Set("MCP-Protocol-Version", "2025-11-25") }, 400, -32020},
		{"future", func(r *http.Request) { r.Header.Set("MCP-Protocol-Version", "2099-01-01") }, 400, -32020},
		{"missing method", func(r *http.Request) { r.Header.Del("Mcp-Method") }, 400, -32020},
		{"method mismatch", func(r *http.Request) { r.Header.Set("Mcp-Method", "resources/list") }, 400, -32020},
		{"duplicate method", func(r *http.Request) { r.Header.Add("Mcp-Method", "resources/list") }, 400, -32020},
		{"missing metadata", func(r *http.Request) {
			r.Body = io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":"test","method":"tools/list","params":{}}`))
		}, 400, -32602},
		{"missing capabilities", func(r *http.Request) {
			r.Body = io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":"test","method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}}`))
		}, 400, -32602},
		{"version mismatch", func(r *http.Request) {
			r.Body = io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":"test","method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2025-11-25","io.modelcontextprotocol/clientCapabilities":{}}}}`))
		}, 400, -32020},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := request("tools/list", nil)
			tt.edit(r)
			w := serve(h, r)
			var response struct {
				Error struct {
					Code int `json:"code"`
				} `json:"error"`
			}
			json.Unmarshal(w.Body.Bytes(), &response)
			if w.Code != tt.status || response.Error.Code != tt.code {
				t.Fatalf("got %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestLegacyAndUnknownMethodsRejected(t *testing.T) {
	h := testHandler(t)
	for _, method := range []string{"initialize", "ping", "logging/setLevel", "resources/subscribe", "does/not/exist"} {
		w := serve(h, request(method, nil))
		if w.Code != 404 || !strings.Contains(w.Body.String(), `-32601`) {
			t.Fatalf("%s: %d %s", method, w.Code, w.Body.String())
		}
	}
	for _, method := range []string{"GET", "DELETE", "PUT"} {
		r := request("tools/list", nil)
		r.Method = method
		w := serve(h, r)
		if w.Code != 405 || w.Header().Get("Allow") != "POST" {
			t.Fatalf("%s: %d", method, w.Code)
		}
	}
}

func TestSecurityAndBounds(t *testing.T) {
	h := testHandler(t)
	for _, origin := range []string{"https://evil.example", "null", "https://mcp.example.com/", ""} {
		r := request("tools/list", nil)
		r.Header["Origin"] = []string{origin}
		if w := serve(h, r); w.Code != 403 {
			t.Fatalf("origin %q: %d", origin, w.Code)
		}
	}
	r := request("tools/list", nil)
	r.Header.Set("Origin", "https://mcp.example.com")
	if w := serve(h, r); w.Code != 200 {
		t.Fatalf("valid Origin: %d %s", w.Code, w.Body.String())
	}
	r = request("tools/list", nil)
	r.Host = "evil.example"
	if w := serve(h, r); w.Code != 403 {
		t.Fatal(w.Code)
	}
	r = request("tools/list", nil)
	r.Body = io.NopCloser(strings.NewReader(strings.Repeat("x", MaxRequestBytes+1)))
	if w := serve(h, r); w.Code != 413 {
		t.Fatal(w.Code)
	}
	for _, raw := range []string{`[]`, `null`, `{`, `{"jsonrpc":"2.0","id":1,"result":{}}`, `{"jsonrpc":"2.0","id":null,"method":"tools/list"}`, `{"jsonrpc":"2.0","id":1.5,"method":"tools/list"}`} {
		r := request("tools/list", nil)
		r.Body = io.NopCloser(strings.NewReader(raw))
		if w := serve(h, r); w.Code != 400 {
			t.Fatalf("accepted %s: %d", raw, w.Code)
		}
	}
}

func TestIgnoredSessionAndResumptionHeaders(t *testing.T) {
	r := request("tools/list", nil)
	r.Header.Set("Mcp-Session-Id", "obsolete")
	r.Header.Set("Last-Event-ID", "obsolete")
	w := serve(testHandler(t), r)
	if w.Code != 200 || w.Header().Get("Mcp-Session-Id") != "" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestExactNumericRequestIDs(t *testing.T) {
	h := testHandler(t)
	for _, id := range []string{"0", "9007199254740993", "1234567890123456789012345678901234567890"} {
		r := request("tools/list", nil)
		b, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(strings.NewReader(strings.Replace(string(b), `"id":"test"`, `"id":`+id, 1)))
		w := serve(h, r)
		var result map[string]json.RawMessage
		json.Unmarshal(w.Body.Bytes(), &result)
		if w.Code != 200 || string(result["id"]) != id {
			t.Fatalf("ID %s: %d %s", id, w.Code, w.Body.String())
		}
	}
}

func TestUnsupportedVersionsAndMalformedCapabilities(t *testing.T) {
	h := testHandler(t)
	for _, version := range []string{"2025-11-25", "2099-01-01"} {
		r := request("tools/list", nil)
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(strings.NewReader(strings.ReplaceAll(string(body), "2026-07-28", version)))
		r.Header.Set("MCP-Protocol-Version", version)
		w := serve(h, r)
		if w.Code != 400 || !strings.Contains(w.Body.String(), "-32022") || !strings.Contains(w.Body.String(), `"supported":["2026-07-28"]`) {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
	for _, value := range []string{"null", "[]", `"invalid"`} {
		r := request("tools/list", nil)
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(strings.NewReader(strings.Replace(string(body), `"io.modelcontextprotocol/clientCapabilities":{}`, `"io.modelcontextprotocol/clientCapabilities":`+value, 1)))
		w := serve(h, r)
		if w.Code != 400 || !strings.Contains(w.Body.String(), "-32602") {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
}

func TestNotificationAcceptedWithoutResponse(t *testing.T) {
	r := request("notifications/custom", nil)
	b, _ := io.ReadAll(r.Body)
	var object map[string]any
	json.Unmarshal(b, &object)
	delete(object, "id")
	b, _ = json.Marshal(object)
	r.Body = io.NopCloser(bytes.NewReader(b))
	w := serve(testHandler(t), r)
	if w.Code != 202 || w.Body.Len() != 0 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestSchemaAndPublicOnlyTools(t *testing.T) {
	h := testHandler(t)
	for _, args := range []map[string]any{{"query": "hi", "pageno": 1.5}, {"query": "hi", "pageno": 0}, {"query": "hi", "safesearch": 3}, {"query": "hi", "pageno": "1"}, {"query": "hi", "time_range": "week"}, {"query": "hi", "extra": true}} {
		w := serve(h, request("tools/call", map[string]any{"name": "searxng_web_search", "arguments": args}))
		if !strings.Contains(w.Body.String(), `"error"`) && !strings.Contains(w.Body.String(), `"isError":true`) {
			t.Fatalf("accepted invalid args: %s", w.Body.String())
		}
	}
	w := serve(h, request("tools/call", map[string]any{"name": "web_url_read", "arguments": map[string]any{"url": "http://127.0.0.1"}}))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"isError":true`) {
		t.Fatalf("private URL: %d %s", w.Code, w.Body.String())
	}
	r := request("resources/read", map[string]any{"uri": "help://usage-guide"})
	r.Header.Set("Mcp-Name", "wrong")
	if w := serve(h, r); w.Code != 400 || !strings.Contains(w.Body.String(), "-32020") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	r = request("resources/read", map[string]any{"uri": "help://usage-guide"})
	r.Header.Set("Mcp-Name", "=?base64?aGVscDovL3VzYWdlLWd1aWRl?=")
	if w := serve(h, r); w.Code != 200 {
		t.Fatalf("base64 name: %d %s", w.Code, w.Body.String())
	}
}

func TestConcurrencyBoundAndCancellation(t *testing.T) {
	started := make(chan struct{}, MaxConcurrentRequests)
	ctx, cancel := context.WithCancel(context.Background())
	h := modernOnly(&config.Config{AllowedHosts: []string{"mcp.example.com"}}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { started <- struct{}{}; <-r.Context().Done() }))
	var wg sync.WaitGroup
	for i := 0; i < MaxConcurrentRequests; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); serve(h, request("tools/list", nil).WithContext(ctx)) }()
	}
	defer cancel()
	for i := 0; i < MaxConcurrentRequests; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("request didn't start")
		}
	}
	if w := serve(h, request("tools/list", nil)); w.Code != 429 {
		t.Fatal(w.Code)
	}
	cancel()
	wg.Wait()
}

func TestConcurrentRequestsShareNoProtocolState(t *testing.T) {
	h := testHandler(t)
	var wg sync.WaitGroup
	for i := 0; i < MaxConcurrentRequests; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := request("tools/list", nil)
			r.Header.Set("Mcp-Session-Id", fmt.Sprint(i))
			if w := serve(h, r); w.Code != 200 {
				t.Errorf("%d %s", w.Code, w.Body.String())
			}
		}(i)
	}
	wg.Wait()
}
