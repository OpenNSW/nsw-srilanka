package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/OpenNSW/nsw/backend/internal/config"
	"github.com/OpenNSW/nsw/backend/srilanka/internal/bootstrap"
)

func main() {
	// Load configuration from environment variables
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load configuration: %v", err)
	}

	opts := &slog.HandlerOptions{
		// AddSource: cfg.Server.Debug,
		Level: cfg.Server.LogLevel,
	}
	logHandler := slog.NewTextHandler(os.Stdout, opts)
	slog.SetDefault(slog.New(logHandler))

	slog.Info("configuration loaded successfully",
		"db_host", cfg.Database.Host,
		"db_port", cfg.Database.Port,
		"db_name", cfg.Database.Name,
		"db_sslmode", cfg.Database.SSLMode,
	)

	slog.Info("CORS configuration",
		"allowed_origins", cfg.CORS.AllowedOrigins,
		"allowed_methods", cfg.CORS.AllowedMethods,
		"allowed_headers", cfg.CORS.AllowedHeaders,
		"allow_credentials", cfg.CORS.AllowCredentials,
		"max_age", cfg.CORS.MaxAge,
	)

	slog.Info("server configuration",
		"port", cfg.Server.Port,
	)

	app, err := bootstrap.Build(context.Background(), cfg)
	if err != nil {
		log.Fatalf("failed to bootstrap application: %v", err)
	}
	defer func() {
		if err := app.Close(); err != nil {
			slog.Error("failed to close application", "error", err)
		}
	}()
	server := app.Server

	// Channel to listen for interrupt signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Start server in a goroutine
	go func() {
		slog.Info("starting server", "port", cfg.Server.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("failed to start server", "error", err)
			quit <- syscall.SIGTERM
		}
	}()

	// Wait for interrupt signal
	<-quit
	slog.Info("shutting down server...")

	// Create a context with timeout for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Attempt graceful shutdown of HTTP server
	if err := server.Shutdown(ctx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
	} else {
		slog.Info("server gracefully stopped")
	}

	slog.Info("server stopped")
}
