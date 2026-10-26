package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/iamangus/mcp-searxng/internal/config"
	"github.com/iamangus/mcp-searxng/internal/logging"
	"github.com/iamangus/mcp-searxng/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %s\n", err)
		os.Exit(1)
	}

	logging.SetLogLevel(logging.LogLevel(cfg.LogLevel))
	handler, cleanup, err := server.NewHTTPHandler(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create server: %s\n", err)
		os.Exit(1)
	}
	defer cleanup()

	httpServer := &http.Server{
		Addr:              net.JoinHostPort(cfg.HTTPHost, strconv.Itoa(cfg.HTTPPort)),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}

	go func() {
		fmt.Fprintf(os.Stderr, "MCP SearXNG Server v%s - HTTP Streamable Transport\n", config.Version)
		fmt.Fprintf(os.Stderr, "Starting HTTP server on %s\n", httpServer.Addr)
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
		httpServer.Close()
	}

	fmt.Fprintln(os.Stderr, "Server stopped")
}
