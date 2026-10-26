# SearXNG MCP Server

An [MCP server](https://modelcontextprotocol.io/introduction) implementation that integrates the [SearXNG](https://docs.searxng.org) API, providing web search capabilities.

Container images: [ghcr.io/iamangus/mcp-searxng](https://github.com/iamangus/mcp-searxng/pkgs/container/mcp-searxng).

<a href="https://glama.ai/mcp/servers/0j7jjyt7m9"><img width="380" height="200" src="https://glama.ai/mcp/servers/0j7jjyt7m9/badge" alt="SearXNG Server MCP server" /></a>

## How It Works

`mcp-searxng` is an **MCP (Model Context Protocol) server** — it is a separate process that AI assistants (such as Claude) connect to in order to perform web searches. It communicates with a SearXNG instance over SearXNG's HTTP JSON API.

> **Not a SearXNG plugin:** This project cannot be installed as a native SearXNG plugin (i.e., a Python module loaded inside the SearXNG process). It is a standalone MCP server that runs alongside your SearXNG instance and queries it via its API. You point it at any existing SearXNG instance by setting the `SEARXNG_URL` environment variable.

```
AI Assistant (e.g. Claude)
        │  MCP Streamable HTTP Transport
        ▼
  mcp-searxng  (this project — Go HTTP server)
        │  HTTP JSON API  (SEARXNG_URL)
        ▼
  SearXNG instance
```

## Features

- **Web Search**: General queries, news, articles, with pagination.
- **URL Content Reading**: Advanced content extraction with pagination, section filtering, and heading extraction.
- **HTML to Markdown**: Fetched web pages are automatically converted to clean Markdown.
- **Intelligent Caching**: URL content is cached with TTL to improve performance and reduce redundant requests.
- **Public-only URL Reading**: Destination IPs are checked and pinned for every request and redirect; normal TLS trust and hostname verification is required.
- **Flexible Proxy Support**: Per-tool proxy configuration with fallback to global proxy settings.

## Tools

- **searxng_web_search**
  - Execute web searches with pagination
  - Inputs:
    - `query` (string, required): The search query
    - `pageno` (integer, optional): Search page number, 1–100 (default: 1)
    - `time_range` (string, optional): Filter results by time range — `"day"`, `"month"`, `"year"`
    - `language` (string, optional): Language code for results (e.g., `"en"`, `"fr"`, `"de"`). Default: `"all"`
    - `safesearch` (integer, optional): Safe search filter level — 0: None, 1: Moderate, 2: Strict (default: 0)
  - Returns structured results and readable text, including upstream engine failures and truncation information. Results are capped at 20 per page; individual fields and diagnostics are also bounded.

- **web_url_read**
  - Read and convert web page content to Markdown with advanced content extraction
  - Inputs:
    - `url` (string, required): Public HTTP(S) URL on port 80 or 443. Private, loopback, link-local, metadata, reserved, and mixed public/private DNS destinations are rejected.
    - `startChar` (integer, optional): Starting Unicode code-point position, 0–10000000 (default: 0)
    - `maxLength` (integer, optional): Maximum Unicode code points to return, 1–50000 (default: 10000)
    - `section` (string, optional): Extract content under a specific heading
    - `paragraphRange` (string, optional): Return specific paragraph ranges (e.g., `"1-5"`, `"3"`, `"10-"`)
    - `readHeadings` (boolean, optional): Return only a list of headings instead of full content
  - Returns `content`, `url`, `startChar`, `totalChars`, `truncated`, and optional `nextStartChar` as structured content, plus a text fallback. Continue with `nextStartChar` and the same filters. `readHeadings` takes precedence over section/paragraph filters; character pagination and limits apply last.
  - Supports HTML/XHTML, plain text and Markdown, decodes declared charsets, and resolves relative HTML links against the final page URL. JavaScript rendering and binary/PDF extraction are not supported.

## Configuration

### Environment Variables

#### Required
- **`MCP_HTTP_PORT`**: Port for the MCP HTTP server (e.g., `3000`). Uses [MCP 2026-07-28 Streamable HTTP](https://modelcontextprotocol.io/specification/2026-07-28/basic/transports/streamable-http).
- **`SEARXNG_URL`**: SearXNG instance URL (e.g., `http://localhost:8080` or `https://search.example.com`)

#### Optional
- **`MCP_HTTP_HOST`**: Bind address (default: `127.0.0.1`; container image: `0.0.0.0`). Kubernetes Services require `0.0.0.0`.
- **`MCP_ALLOWED_HOSTS`**: Comma-separated exact hostnames/IPs, without ports or wildcards (default: `localhost,127.0.0.1,::1`). Set the external hostname when using a reverse proxy. Forwarded headers are not trusted for this check.
- **`MCP_ALLOWED_ORIGINS`**: Comma-separated exact HTTP(S) origins without paths. Default: none. Requests without `Origin` are allowed; present origins must match or receive 403. This is Origin validation, not browser CORS support.
- **`AUTH_USERNAME`** / **`AUTH_PASSWORD`**: HTTP Basic Auth credentials for password-protected SearXNG instances
- **`USER_AGENT`**: Default User-Agent header for both `searxng_web_search` and `web_url_read` (e.g., `MyBot/1.0`)
- **`URL_READER_USER_AGENT`**: Custom User-Agent specifically for the `web_url_read` tool (overrides `USER_AGENT`)
- **`LOG_LEVEL`**: Application stderr logging threshold: `debug`, `info` (default), `warning`, or `error`. MCP logging notifications are not advertised.
- **`HTTP_PROXY`** / **`HTTPS_PROXY`**: Global proxy URLs for all traffic. Format: `http://[user:pass@]proxy.host:port`
- **`NO_PROXY`**: Standard Go bypass list supporting CIDRs, IPv6, domains, and host:port entries (e.g., `localhost,.internal,10.0.0.0/8`)
- **`SEARCH_HTTP_PROXY`** / **`SEARCH_HTTPS_PROXY`**: Proxy for `searxng_web_search` tool only (overrides global proxy)
- **`URL_READER_HTTP_PROXY`** / **`URL_READER_HTTPS_PROXY`**: Proxy for `web_url_read` tool only (overrides global proxy)

> **Proxy precedence:** Tool-specific proxies (`SEARCH_*`, `URL_READER_*`) take priority over global proxies (`HTTP_PROXY`, `HTTPS_PROXY`). All lowercase variants (`http_proxy`, `search_http_proxy`, etc.) are also recognized.

URL-reader proxies must support **CONNECT to numeric IPs on both ports 80 and 443**. Hostname resolution and public-IP checks happen locally before tunneling, while HTTP Host and TLS SNI remain the original hostname. HTTP(S) proxies are trusted administrator-controlled infrastructure; they must honor CONNECT destinations. There is no unsafe forward-proxy fallback. Search redirects are restricted to the configured origin to protect upstream credentials.

Automatic AIA certificate fetching and `AIA_CACHE_TTL` have been removed. Upstream HTTPS servers must present a complete, valid certificate chain. The configured SearXNG URL is administrator-controlled and may be internal; the public-only restriction applies to user-supplied URL-reader destinations.

**No MCP authentication is implemented.** Keep this service on a trusted internal network or behind an authenticated gateway; Host/Origin validation is not authentication. Fetched web content is untrusted data, not instructions.

### Resource limits

- MCP request: 64 KiB; at most 8 concurrent MCP requests (excess requests receive 429).
- Upstream body: 2 MiB after decompression, and again after charset decoding; converted content is also capped at 2 MiB.
- Upstream deadline: 15 seconds; MCP deadline: 20 seconds; bounded HTTP headers, redirects and connection pools. HTML conversion is limited to 50000 DOM nodes and nesting depth 128.
- URL cache: 60-second TTL, at most 128 entries / 16 MiB; only Markdown/text is retained, never raw HTML.
- Tool result: 1 MiB SDK safety cap, in addition to per-tool limits.
- Logs omit query strings, full URLs and upstream response bodies.

## Installation

### Build from Source

Requires [Go](https://go.dev/dl/) 1.26.10+.

```bash
git clone https://github.com/iamangus/mcp-searxng.git
cd mcp-searxng
go build -o mcp-searxng .
```

Run the server:

```bash
export SEARXNG_URL=https://search.example.com
export MCP_HTTP_PORT=3000
./mcp-searxng
```

### MCP Client Configuration

The server supports **MCP 2026-07-28 only**: there is no `initialize` handshake or protocol session. Older MCP clients are intentionally unsupported. Configure a compatible client with the server's URL:

```json
{
  "mcpServers": {
    "searxng": {
      "url": "http://localhost:3000/mcp"
    }
  }
}
```

<details>
<summary>Full Configuration Example</summary>

Set these on the **server process**, not in a remote client's configuration:

```bash
export SEARXNG_URL=https://search.example.com
export MCP_HTTP_PORT=3000
export USER_AGENT=MyBot/1.0
export NO_PROXY=localhost,127.0.0.1,.local
./mcp-searxng
```
</details>

### Docker

#### Run from GHCR

```bash
docker pull ghcr.io/iamangus/mcp-searxng:latest
docker run -d \
  -p 127.0.0.1:3000:3000 \
  -e SEARXNG_URL=https://search.example.com \
  -e MCP_HTTP_PORT=3000 \
  ghcr.io/iamangus/mcp-searxng:latest
```

Then point your MCP client to `http://localhost:3000/mcp`.

#### Build Locally

```bash
docker build -t mcp-searxng:latest .
docker run -d -p 127.0.0.1:3000:3000 \
  -e SEARXNG_URL=https://search.example.com \
  -e MCP_HTTP_PORT=3000 \
  mcp-searxng:latest
```

#### Docker Compose

```yaml
services:
  mcp-searxng:
    image: ghcr.io/iamangus/mcp-searxng:latest
    ports:
      - "127.0.0.1:3000:3000"
    environment:
      - SEARXNG_URL=https://search.example.com
      - MCP_HTTP_PORT=3000
      # Optional:
      # - AUTH_USERNAME=your_username
      # - AUTH_PASSWORD=your_password
      # - USER_AGENT=MyBot/1.0
      # - URL_READER_USER_AGENT=Mozilla/5.0 (compatible; MyBot/1.0)
      # - HTTP_PROXY=http://proxy:8080
      # - NO_PROXY=localhost,127.0.0.1,.local
```

### HTTP Endpoints

- **MCP Protocol**: `POST /mcp` only; `GET` and `DELETE` return 405
- **Health Check**: `GET /health`

```bash
curl http://localhost:3000/health
# {"server":"iamangus/mcp-searxng","status":"healthy","transport":"http","version":"1.0.0"}
```

Each MCP POST requires `Content-Type: application/json`, `Accept: application/json, text/event-stream`, `MCP-Protocol-Version: 2026-07-28`, `Mcp-Method`, and (for tool calls / resource reads) `Mcp-Name`. Header values must match the body. Every request includes `params._meta` with protocol version and client capabilities. Discovery is optional for clients:

```bash
curl http://localhost:3000/mcp \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H 'MCP-Protocol-Version: 2026-07-28' \
  -H 'Mcp-Method: server/discover' \
  -d '{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}'
```

### Kubernetes / reverse proxy

For the internal Traefik deployment at `https://mcp.srvd.site/search/mcp`, preserve the existing `/search` strip-prefix middleware and set:

```yaml
MCP_HTTP_HOST: 0.0.0.0
MCP_HTTP_PORT: 8080
MCP_ALLOWED_HOSTS: mcp.srvd.site
MCP_ALLOWED_ORIGINS: https://mcp.srvd.site
```

TLS terminates at Traefik. Use `GET /health` on the container port for probes; it is a process-liveness endpoint, not an upstream SearXNG readiness check. The image runs as UID/GID 65532 and supports a read-only root filesystem. Pin a newly published image SHA/digest before rollout; an old image will not gain these protections merely by changing environment variables.

## Troubleshooting

### 403 Forbidden Error from SearXNG

If you receive a `403 Forbidden` error when using `mcp-searxng`, it is likely because your SearXNG instance does not have JSON format enabled. This server requests results in JSON format (`format=json`), which must be explicitly allowed in SearXNG's configuration.

**To fix this**, edit your SearXNG `settings.yml` (commonly located at `/etc/searxng/settings.yml`) and add `json` to the list of allowed formats:

```yaml
search:
  formats:
    - html
    - json
```

After saving the file, restart your SearXNG instance. For example, if running with Docker:

```bash
docker restart searxng
```

You can verify JSON format is working by running:

```bash
curl 'http://localhost:8080/search?q=test&format=json'
```

You should receive a JSON response. If you still get a 403 error, double-check that:

- The `settings.yml` file is correctly mounted into your Docker container
- The YAML indentation is correct
- The SearXNG instance was fully restarted after the configuration change

For more details, see the [SearXNG settings documentation](https://docs.searxng.org/admin/settings/settings.html).

### Session Errors

This server has no protocol sessions. Upgrade clients to MCP 2026-07-28; merely disabling session headers in an older client is insufficient. `initialize`, `ping`, and `logging/setLevel` are not supported. Obsolete session/resumption headers are ignored.

## Contributing

Requirements: [Go](https://go.dev/dl/) 1.26.10+

```bash
git clone https://github.com/iamangus/mcp-searxng.git
cd mcp-searxng
go mod download

# Build
go build -o mcp-searxng .

# Run tests
go test ./...

# Run with coverage
go test -cover ./...

go test -race ./...
go vet ./...
go test ./internal/urlreader -run='^$' -fuzz=FuzzPagination -fuzztime=10s
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

Coding standards:

- Follow standard Go conventions and idioms
- Use `gofmt` for formatting
- Write tests for new functionality
- Follow existing error handling patterns

## License

This MCP server is licensed under the MIT License. This means you are free to use, modify, and distribute the software, subject to the terms and conditions of the MIT License. For more details, please see the LICENSE file in the project repository.
