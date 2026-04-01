package errors

import (
	"fmt"
	"net/url"
	"strings"
)

type MCPSearXNGError struct {
	Message string
}

func (e *MCPSearXNGError) Error() string {
	return e.Message
}

func NewConfigurationError(message string) *MCPSearXNGError {
	return &MCPSearXNGError{Message: fmt.Sprintf("Configuration Error: %s", message)}
}

func NewNetworkError(err error, targetURL, searxngURL string) *MCPSearXNGError {
	target := "website"
	if searxngURL != "" {
		target = "SearXNG server"
	}

	errMsg := err.Error()
	errCode := extractCode(err)

	if errCode == "ECONNREFUSED" {
		return &MCPSearXNGError{Message: fmt.Sprintf("Connection Error: %s is not responding (%s)", target, targetURL)}
	}
	if errCode == "ENOTFOUND" || errCode == "EAI_NONAME" {
		hostname := "unknown"
		if u, e := url.Parse(targetURL); e == nil {
			hostname = u.Hostname()
		}
		return &MCPSearXNGError{Message: fmt.Sprintf("DNS Error: Cannot resolve hostname %q", hostname)}
	}
	if strings.Contains(errMsg, "certificate") {
		return &MCPSearXNGError{Message: fmt.Sprintf("SSL Error: Certificate problem with %s", target)}
	}
	if strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "deadline exceeded") {
		return &MCPSearXNGError{Message: fmt.Sprintf("Timeout Error: %s is too slow to respond", target)}
	}

	if errMsg == "" {
		errMsg = "Connection failed"
	}
	return &MCPSearXNGError{Message: fmt.Sprintf("Network Error: %s", errMsg)}
}

func NewServerError(status int, statusText, responseBody string, searxngURL string) *MCPSearXNGError {
	target := "Website"
	if searxngURL != "" {
		target = "SearXNG server"
	}

	switch {
	case status == 403:
		reason := "Access blocked (bot detection or geo-restriction)"
		if searxngURL != "" {
			reason = "Authentication required or IP blocked"
		}
		return &MCPSearXNGError{Message: fmt.Sprintf("%s Error (%d): %s", target, status, reason)}
	case status == 404:
		reason := "Page not found"
		if searxngURL != "" {
			reason = "Search endpoint not found"
		}
		return &MCPSearXNGError{Message: fmt.Sprintf("%s Error (%d): %s", target, status, reason)}
	case status == 429:
		return &MCPSearXNGError{Message: fmt.Sprintf("%s Error (%d): Rate limit exceeded", target, status)}
	case status >= 500:
		return &MCPSearXNGError{Message: fmt.Sprintf("%s Error (%d): Internal server error", target, status)}
	default:
		return &MCPSearXNGError{Message: fmt.Sprintf("%s Error (%d): %s", target, status, statusText)}
	}
}

func NewJSONError(responseText string) *MCPSearXNGError {
	preview := responseText
	if len(preview) > 100 {
		preview = preview[:100]
	}
	preview = strings.ReplaceAll(preview, "\n", " ")
	return &MCPSearXNGError{Message: fmt.Sprintf("SearXNG Response Error: Invalid JSON format. Response: %q...", preview)}
}

func NewDataError() *MCPSearXNGError {
	return &MCPSearXNGError{Message: "SearXNG Data Error: Missing results array in response"}
}

func NewNoResultsMessage(query string) string {
	return fmt.Sprintf("No results found for %q. Try different search terms or check if SearXNG search engines are working.", query)
}

func NewURLFormatError(u string) *MCPSearXNGError {
	return &MCPSearXNGError{Message: fmt.Sprintf("URL Format Error: Invalid URL %q", u)}
}

func NewContentError(message, targetURL string) *MCPSearXNGError {
	return &MCPSearXNGError{Message: fmt.Sprintf("Content Error: %s (%s)", message, targetURL)}
}

func NewConversionError(targetURL string) *MCPSearXNGError {
	return &MCPSearXNGError{Message: fmt.Sprintf("Conversion Error: Cannot convert HTML to Markdown (%s)", targetURL)}
}

func NewTimeoutError(timeoutMs int, targetURL string) *MCPSearXNGError {
	hostname := targetURL
	if u, err := url.Parse(targetURL); err == nil {
		hostname = u.Hostname()
	}
	return &MCPSearXNGError{Message: fmt.Sprintf("Timeout Error: %s took longer than %dms to respond", hostname, timeoutMs)}
}

func NewEmptyContentWarning(targetURL string) string {
	return fmt.Sprintf("Content Warning: Page fetched but appears empty after conversion (%s). May contain only media or require JavaScript.", targetURL)
}

func NewUnexpectedError(err error) *MCPSearXNGError {
	return &MCPSearXNGError{Message: fmt.Sprintf("Unexpected Error: %s", err.Error())}
}

func extractCode(err error) string {
	msg := err.Error()
	if idx := strings.Index(msg, ":"); idx != -1 {
		return strings.TrimSpace(msg[:idx])
	}
	return ""
}
