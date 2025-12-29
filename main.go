package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"binance-vision-connector/handlers"
	binancevisionconnector "binance-vision-connector/binance-vision-connector"
)

// Config holds application configuration
type Config struct {
	Port            string
	Timeout         time.Duration
	MaxConnsPerHost int
	MaxIdleConns    int
}

var (
	config           Config
	connector        *binancevisionconnector.Connector
	downloadHandler  *handlers.DownloadHandler
	healthHandler    *handlers.HealthHandler
	requestMetrics   *handlers.RequestMetrics
)

func init() {
	// Load .env file if it exists
	godotenv.Load()

	config = Config{
		Port:            getEnv("PORT", "8080"),
		Timeout:         30 * time.Second,
		MaxConnsPerHost: 10,
		MaxIdleConns:    100,
	}

	// Initialize connector with optimized configuration
	connectorConfig := binancevisionconnector.DefaultConfig()
	connectorConfig.Timeout = config.Timeout
	connectorConfig.MaxConnsPerHost = config.MaxConnsPerHost
	connectorConfig.MaxIdleConns = config.MaxIdleConns
	connector = binancevisionconnector.NewConnectorWithConfig(connectorConfig)

	// Initialize request metrics
	requestMetrics = &handlers.RequestMetrics{}

	// Initialize handlers
	downloadHandler = &handlers.DownloadHandler{
		Connector: connector,
		Timeout:   config.Timeout,
		Metrics:   requestMetrics,
	}

	healthHandler = &handlers.HealthHandler{
		Metrics: requestMetrics,
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// requestTrackingMiddleware tracks request metrics
func requestTrackingMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestMetrics.Mu.Lock()
		requestMetrics.TotalRequests++
		requestMetrics.ActiveRequests++
		requestMetrics.Mu.Unlock()

		defer func() {
			requestMetrics.Mu.Lock()
			requestMetrics.ActiveRequests--
			requestMetrics.Mu.Unlock()
		}()

		next(w, r)
	}
}

func main() {
	// Setup HTTP server with optimized settings for high load
	mux := http.NewServeMux()
	mux.HandleFunc("/download", requestTrackingMiddleware(downloadHandler.Handle))
	mux.HandleFunc("/health", healthHandler.Handle)

	server := &http.Server{
		Addr:           ":" + config.Port,
		Handler:        mux,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:  60 * time.Second, // Increased for large JSON responses
		IdleTimeout:   120 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1MB
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Server starting on port %s", config.Port)
		log.Printf("Configuration:")
		log.Printf("  Timeout: %v", config.Timeout)
		log.Printf("  Max Connections Per Host: %d", config.MaxConnsPerHost)
		log.Printf("  Max Idle Connections: %d", config.MaxIdleConns)
		log.Printf("Endpoints:")
		log.Printf("  GET /download?SYMBOL=<symbol>&YYYY=<year>&MM=<month>&DD=<day>")
		log.Printf("  GET /health")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}
