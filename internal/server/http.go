package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/iamangus/mcp-searxng/internal/config"
	"github.com/mark3labs/mcp-go/mcp"
)

const MaxRequestBytes = 64 << 10
const MaxConcurrentRequests = 8

// modernOnly closes the SDK's legacy fallback paths and bounds work before it
// buffers or dispatches a message. Each accepted request is independent.
func modernOnly(cfg *config.Config, next http.Handler) http.Handler {
	var active atomic.Int32
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		host = strings.Trim(host, "[]")
		hosts := cfg.AllowedHosts
		if len(hosts) == 0 {
			hosts = []string{"localhost", "127.0.0.1", "::1"}
		}
		allowed := false
		for _, h := range hosts {
			if strings.EqualFold(host, h) {
				allowed = true
			}
		}
		if !allowed {
			http.Error(w, "Host not allowed", http.StatusForbidden)
			return
		}
		if origins := r.Header.Values("Origin"); len(origins) != 0 {
			allowed = false
			if len(origins) == 1 {
				for _, origin := range cfg.AllowedOrigins {
					if origins[0] == origin {
						allowed = true
					}
				}
			}
			if !allowed {
				http.Error(w, "Origin not allowed", http.StatusForbidden)
				return
			}
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "Only POST is supported", http.StatusMethodNotAllowed)
			return
		}
		if active.Add(1) > MaxConcurrentRequests {
			active.Add(-1)
			w.Header().Set("Retry-After", "1")
			http.Error(w, "Server busy", http.StatusTooManyRequests)
			return
		}
		defer active.Add(-1)
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxRequestBytes))
		if err != nil {
			rpcError(w, nil, -32600, "Request body is too large or unreadable", http.StatusRequestEntityTooLarge, nil)
			return
		}
		var envelope struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Method  string          `json:"method"`
			Result  json.RawMessage `json:"result"`
			Error   json.RawMessage `json:"error"`
		}
		if !json.Valid(body) {
			rpcError(w, nil, -32700, "Invalid JSON", 400, nil)
			return
		}
		if json.Unmarshal(body, &envelope) != nil || envelope.JSONRPC != "2.0" || envelope.Method == "" || envelope.Result != nil || envelope.Error != nil {
			rpcError(w, nil, -32600, "Expected one JSON-RPC request or notification", 400, nil)
			return
		}
		if envelope.ID != nil {
			var id any
			d := json.NewDecoder(bytes.NewReader(envelope.ID))
			d.UseNumber()
			if d.Decode(&id) != nil {
				rpcError(w, nil, -32600, "Invalid request ID", 400, nil)
				return
			}
			valid := false
			switch v := id.(type) {
			case string:
				valid = true
			case json.Number:
				// Preserve arbitrarily large integer IDs without float64 rounding.
				// Lexical integer form avoids expanding hostile numeric exponents.
				digits := strings.TrimPrefix(v.String(), "-")
				valid = digits != "" && strings.IndexFunc(digits, func(r rune) bool { return r < '0' || r > '9' }) == -1
			}
			if !valid {
				rpcError(w, nil, -32600, "ID must be a string or integer", 400, nil)
				return
			}
		}
		for _, name := range []string{"MCP-Protocol-Version", "Mcp-Method", "Mcp-Name"} {
			if len(r.Header.Values(name)) > 1 {
				rpcError(w, envelope.ID, -32020, "Duplicate MCP header", 400, nil)
				return
			}
		}
		version := r.Header.Get("MCP-Protocol-Version")
		if version == "" {
			rpcError(w, envelope.ID, -32020, "MCP-Protocol-Version header is required; only 2026-07-28 is supported", 400, nil)
			return
		}
		var metadata struct {
			Params struct {
				Meta map[string]json.RawMessage `json:"_meta"`
			} `json:"params"`
		}
		var bodyVersion string
		var capabilities map[string]json.RawMessage
		if json.Unmarshal(body, &metadata) != nil || json.Unmarshal(metadata.Params.Meta["io.modelcontextprotocol/protocolVersion"], &bodyVersion) != nil || bodyVersion == "" || json.Unmarshal(metadata.Params.Meta["io.modelcontextprotocol/clientCapabilities"], &capabilities) != nil || capabilities == nil {
			rpcError(w, envelope.ID, -32602, "Every request requires protocolVersion and clientCapabilities in params._meta", 400, nil)
			return
		}
		if version != bodyVersion {
			rpcError(w, envelope.ID, -32020, "Protocol version header does not match body", 400, nil)
			return
		}
		if version != mcp.ProtocolVersion20260728 {
			rpcError(w, envelope.ID, -32022, "Unsupported protocol version", 400, map[string]any{"supported": []string{mcp.ProtocolVersion20260728}, "requested": version})
			return
		}
		// No change subscriptions or streaming notifications are advertised.
		if envelope.Method == "subscriptions/listen" {
			rpcError(w, envelope.ID, -32601, "Subscriptions are not supported", 404, nil)
			return
		}
		// These legacy headers have no meaning in this revision. In particular,
		// v1.3.0 incorrectly rejects Last-Event-ID instead of ignoring it.
		r.Header = r.Header.Clone()
		r.Header.Del("Mcp-Session-Id")
		r.Header.Del("Last-Event-ID")
		if len(envelope.ID) > 0 && envelope.ID[0] != '"' {
			// mcp-go v1.3.0 decodes numeric IDs through float64. Use a string
			// internally and restore the exact original ID in the JSON reply.
			var object map[string]json.RawMessage
			json.Unmarshal(body, &object)
			object["id"], _ = json.Marshal(string(envelope.ID))
			body, _ = json.Marshal(object)
			w = &numericIDWriter{ResponseWriter: w, id: envelope.ID}
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		w.Header().Set("X-Accel-Buffering", "no")
		next.ServeHTTP(w, r)
	})
}

// The SDK emits each JSON response in one Encoder.Encode / Write call. Not
// implementing Flusher deliberately selects JSON-only replies for our static,
// non-notifying methods; this server does not offer SSE subscription streams.
type numericIDWriter struct {
	http.ResponseWriter
	id json.RawMessage
}

func (w *numericIDWriter) Write(p []byte) (int, error) {
	if strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		var object map[string]json.RawMessage
		if json.Unmarshal(p, &object) == nil && object["jsonrpc"] != nil {
			object["id"] = w.id
			out, err := json.Marshal(object)
			if err != nil {
				return 0, err
			}
			_, err = w.ResponseWriter.Write(append(out, '\n'))
			if err != nil {
				return 0, err
			}
			return len(p), nil
		}
	}
	return w.ResponseWriter.Write(p)
}

func rpcError(w http.ResponseWriter, id json.RawMessage, code int, message string, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	err := map[string]any{"code": code, "message": message}
	if data != nil {
		err["data"] = data
	}
	response := map[string]any{"jsonrpc": "2.0", "error": err}
	if id != nil {
		response["id"] = id
	}
	json.NewEncoder(w).Encode(response)
}
