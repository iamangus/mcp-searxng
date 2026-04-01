package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/iamangus/mcp-searxng/internal/config"
	"github.com/iamangus/mcp-searxng/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %s\n", err)
		os.Exit(1)
	}

	handler, urlCache, certCache, err := server.NewHTTPHandler(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create server: %s\n", err)
		os.Exit(1)
	}

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler: handler,
	}

	go func() {
		fmt.Fprintf(os.Stderr, "MCP SearXNG Server v%s - HTTP Streamable Transport\n", config.Version)
		fmt.Fprintf(os.Stderr, "SearXNG URL: %s\n", cfg.SearXNGURL)
		fmt.Fprintf(os.Stderr, "Starting HTTP server on port %d\n", cfg.HTTPPort)
		fmt.Fprintf(os.Stderr, "Health check: http://localhost:%d/health\n", cfg.HTTPPort)
		fmt.Fprintf(os.Stderr, "MCP endpoint: http://localhost:%d/mcp\n", cfg.HTTPPort)

		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Fprintln(os.Stderr, "Shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "Server shutdown error: %v\n", err)
	}

	urlCache.Stop()
	certCache.Stop()

	fmt.Fprintln(os.Stderr, "Server stopped")
}
