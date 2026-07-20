package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/iamangus/mcp-searxng/internal/aia"
	"github.com/iamangus/mcp-searxng/internal/cache"
	"github.com/iamangus/mcp-searxng/internal/config"
	"github.com/iamangus/mcp-searxng/internal/logging"
	"github.com/iamangus/mcp-searxng/internal/search"
	"github.com/iamangus/mcp-searxng/internal/urlreader"
)

func NewMCPServer(cfg *config.Config, urlCache *cache.Cache, certCache *aia.CertCache) (*mcpserver.MCPServer, error) {
	s := mcpserver.NewMCPServer(
		"iamangus/mcp-searxng",
		config.Version,
		mcpserver.WithToolCapabilities(false),
		mcpserver.WithLogging(),
		mcpserver.WithRecovery(),
	)

	registerTools(s, cfg, urlCache, certCache)
	registerResources(s, cfg)

	return s, nil
}

func registerTools(s *mcpserver.MCPServer, cfg *config.Config, urlCache *cache.Cache, certCache *aia.CertCache) {
	webSearchTool := mcp.NewTool("searxng_web_search",
		mcp.WithDescription(
			"Performs a web search using the SearXNG API, ideal for general queries, news, articles, and online content. "+
				"Use this for broad information gathering, recent events, or when you need diverse web sources.",
		),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("The search query. This is the main input for the web search"),
		),
		mcp.WithNumber("pageno",
			mcp.Description("Search page number (starts at 1)"),
		),
		mcp.WithString("time_range",
			mcp.Description("Time range of search (day, month, year)"),
			mcp.Enum("day", "month", "year"),
		),
		mcp.WithString("language",
			mcp.Description("Language code for search results (e.g., 'en', 'fr', 'de'). Default is instance-dependent."),
		),
		mcp.WithNumber("safesearch",
			mcp.Description("Safe search filter level (0: None, 1: Moderate, 2: Strict)"),
		),
	)

	s.AddTool(webSearchTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()

		query, _ := args["query"].(string)
		if query == "" {
			return mcp.NewToolResultError("query is required"), nil
		}

		pageNo := 1
		if v, ok := args["pageno"]; ok {
			if f, ok := v.(float64); ok {
				pageNo = int(f)
			}
		}

		var timeRange string
		if v, ok := args["time_range"]; ok {
			timeRange, _ = v.(string)
		}

		language := "all"
		if v, ok := args["language"]; ok {
			if s, ok := v.(string); ok && s != "" {
				language = s
			}
		}

		safeSearch := 0
		if v, ok := args["safesearch"]; ok {
			if f, ok := v.(float64); ok {
				safeSearch = int(f)
			}
		}

		result, err := search.PerformWebSearch(ctx, cfg, certCache, search.SearchParams{
			Query:      query,
			PageNo:     pageNo,
			TimeRange:  timeRange,
			Language:   language,
			SafeSearch: safeSearch,
		})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(result), nil
	})

	readURLTool := mcp.NewTool("web_url_read",
		mcp.WithDescription(
			"Read the content from an URL. "+
				"Use this for further information retrieving to understand the content of each URL.",
		),
		mcp.WithString("url",
			mcp.Required(),
			mcp.Description("URL"),
		),
		mcp.WithNumber("startChar",
			mcp.Description("Starting character position for content extraction (default: 0)"),
		),
		mcp.WithNumber("maxLength",
			mcp.Description("Maximum number of characters to return"),
		),
		mcp.WithString("section",
			mcp.Description("Extract content under a specific heading (searches for heading text)"),
		),
		mcp.WithString("paragraphRange",
			mcp.Description("Return specific paragraph ranges (e.g., '1-5', '3', '10-')"),
		),
		mcp.WithBoolean("readHeadings",
			mcp.Description("Return only a list of headings instead of full content"),
		),
	)

	s.AddTool(readURLTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()

		targetURL, _ := args["url"].(string)
		if targetURL == "" {
			return mcp.NewToolResultError("url is required"), nil
		}

		opts := urlreader.PaginationOptions{}

		if v, ok := args["startChar"]; ok {
			if f, ok := v.(float64); ok && f >= 0 {
				opts.StartChar = int(f)
				opts.HasStartChar = true
			}
		}

		if v, ok := args["maxLength"]; ok {
			if f, ok := v.(float64); ok && f >= 1 {
				opts.MaxLength = int(f)
				opts.HasMaxLength = true
			}
		}

		if v, ok := args["section"]; ok {
			opts.Section, _ = v.(string)
		}

		if v, ok := args["paragraphRange"]; ok {
			opts.ParagraphRange, _ = v.(string)
		}

		if v, ok := args["readHeadings"]; ok {
			opts.ReadHeadings, _ = v.(bool)
		}

		result, err := urlreader.FetchAndConvertToMarkdown(ctx, cfg, urlCache, certCache, targetURL, 10000, opts)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(result), nil
	})
}

func registerResources(s *mcpserver.MCPServer, cfg *config.Config) {
	configResource := mcp.NewResource(
		"config://server-config",
		"Server Configuration",
		mcp.WithResourceDescription("Current server configuration and environment variables"),
		mcp.WithMIMEType("application/json"),
	)

	s.AddResource(configResource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		configData := map[string]any{
			"serverInfo": map[string]any{
				"name":        "iamangus/mcp-searxng",
				"version":     config.Version,
				"description": "MCP server for SearXNG integration",
			},
			"environment": map[string]any{
				"searxngUrl":      cfg.SearXNGURL,
				"hasAuth":         cfg.HasAuth(),
				"hasProxy":        cfg.HasProxy(),
				"hasNoProxy":      cfg.HasNoProxy(),
				"currentLogLevel": string(logging.GetLogLevel()),
			},
			"capabilities": map[string]any{
				"tools":      []string{"searxng_web_search", "web_url_read"},
				"logging":    true,
				"resources":  true,
				"transports": []string{"http"},
			},
		}

		jsonData, _ := json.MarshalIndent(configData, "", "  ")

		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      "config://server-config",
				MIMEType: "application/json",
				Text:     string(jsonData),
			},
		}, nil
	})

	helpResource := mcp.NewResource(
		"help://usage-guide",
		"Usage Guide",
		mcp.WithResourceDescription("How to use the MCP SearXNG server effectively"),
		mcp.WithMIMEType("text/markdown"),
	)

	s.AddResource(helpResource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		helpText := "# SearXNG MCP Server Help\n\n" +
			"## Overview\n" +
			"This is a Model Context Protocol (MCP) server that provides web search capabilities through SearXNG and URL content reading functionality.\n\n" +
			"## Available Tools\n\n" +
			"### 1. searxng_web_search\n" +
			"Performs web searches using the configured SearXNG instance.\n\n" +
			"**Parameters:**\n" +
			"- `query` (required): The search query string\n" +
			"- `pageno` (optional): Page number (default: 1)\n" +
			"- `time_range` (optional): Filter by time - \"day\", \"month\", or \"year\"\n" +
			"- `language` (optional): Language code like \"en\", \"fr\", \"de\" (default: \"all\")\n" +
			"- `safesearch` (optional): Safe search level - 0 (none), 1 (moderate), 2 (strict)\n\n" +
			"### 2. web_url_read\n" +
			"Reads and converts web page content to Markdown format.\n\n" +
			"**Parameters:**\n" +
			"- `url` (required): The URL to fetch and convert\n\n" +
			"## Configuration\n\n" +
			"### Required Environment Variables\n" +
			"- `SEARXNG_URL`: URL of your SearXNG instance (e.g., http://localhost:8080)\n" +
			"- `MCP_HTTP_PORT`: HTTP port for the MCP server (e.g., 3000)\n\n" +
			"### Optional Environment Variables\n" +
			"- `AUTH_USERNAME` & `AUTH_PASSWORD`: Basic authentication for SearXNG\n" +
			"- `HTTP_PROXY` / `HTTPS_PROXY`: Proxy server configuration\n" +
			"- `NO_PROXY` / `no_proxy`: Comma-separated list of hosts to bypass proxy\n" +
			"- `USER_AGENT`: Global User-Agent header\n" +
			"- `URL_READER_USER_AGENT`: User-Agent for URL reader (overrides USER_AGENT)\n" +
			"- `AIA_CACHE_TTL`: Cache TTL for AIA intermediate certificates (default: 24h)\n\n" +
			"## Troubleshooting\n\n" +
			"1. **\"SEARXNG_URL not set\"**: Configure the SEARXNG_URL environment variable\n" +
			"2. **Network errors**: Check if SearXNG is running and accessible\n" +
			"3. **Empty results**: Try different search terms or check SearXNG instance\n" +
			"4. **Timeout errors**: The server has a 10-second timeout for URL fetching\n" +
			"5. **SSL errors**: Server uses AIA chasing to fetch missing intermediate certificates automatically\n\n" +
			"Use logging level \"debug\" for detailed request information.\n"

		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      "help://usage-guide",
				MIMEType: "text/markdown",
				Text:     helpText,
			},
		}, nil
	})
}

func NewHTTPHandler(cfg *config.Config) (http.Handler, *cache.Cache, *aia.CertCache, error) {
	urlCache := cache.NewCache(60 * time.Second)
	certCache := aia.NewCertCache(cfg.AIACacheTTL)

	mcpSrv, err := NewMCPServer(cfg, urlCache, certCache)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create MCP server: %w", err)
	}

	streamableServer := mcpserver.NewStreamableHTTPServer(mcpSrv,
		mcpserver.WithStateful(false),
		mcpserver.WithEndpointPath("/mcp"),
	)

	mux := http.NewServeMux()
	mux.Handle("/mcp", streamableServer)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":    "healthy",
			"server":    "iamangus/mcp-searxng",
			"version":   config.Version,
			"transport": "http",
		})
	})

	return mux, urlCache, certCache, nil
}
