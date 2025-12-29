package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	
	binancevisionconnector "binance-vision-connector/binance-vision-connector"
)

// Config holds application configuration
type Config struct {
	Port            string
	Timeout         time.Duration
	MaxConnsPerHost int
	MaxIdleConns    int
}

// APIResponse represents a standard API response
type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// RequestMetrics tracks request statistics
type RequestMetrics struct {
	mu                sync.RWMutex
	TotalRequests     int64
	SuccessfulRequests int64
	FailedRequests    int64
	ActiveRequests    int64
}

var (
	config    Config
	connector *binancevisionconnector.Connector
	metrics   RequestMetrics
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
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// validateSymbol validates the trading pair symbol
func validateSymbol(symbol string) error {
	if symbol == "" {
		return fmt.Errorf("symbol cannot be empty")
	}
	// Basic validation: should be alphanumeric, typically uppercase
	matched, _ := regexp.MatchString("^[A-Z0-9]+$", symbol)
	if !matched {
		return fmt.Errorf("invalid symbol format: %s (should be uppercase alphanumeric)", symbol)
	}
	return nil
}

// validateDate validates year, month, and day parameters
func validateDate(year, month, day string) error {
	y, err := strconv.Atoi(year)
	if err != nil || y < 2000 || y > 2100 {
		return fmt.Errorf("invalid year: %s", year)
	}

	m, err := strconv.Atoi(month)
	if err != nil || m < 1 || m > 12 {
		return fmt.Errorf("invalid month: %s (must be 01-12)", month)
	}

	d, err := strconv.Atoi(day)
	if err != nil || d < 1 || d > 31 {
		return fmt.Errorf("invalid day: %s (must be 01-31)", day)
	}

	// Format date with zero-padding for parsing
	year, month, day = formatDate(year, month, day)

	// Validate actual date (e.g., Feb 30 doesn't exist)
	dateStr := fmt.Sprintf("%s-%s-%s", year, month, day)
	_, err = time.Parse("2006-01-02", dateStr)
	if err != nil {
		return fmt.Errorf("invalid date: %s", dateStr)
	}

	return nil
}

// formatDate ensures date components are zero-padded
func formatDate(year, month, day string) (string, string, string) {
	// Ensure zero-padding
	if len(month) == 1 {
		month = "0" + month
	}
	if len(day) == 1 {
		day = "0" + day
	}
	return year, month, day
}

func writeJSONResponse(w http.ResponseWriter, statusCode int, response APIResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode JSON response: %v", err)
	}
}

// middleware for request tracking
func requestTrackingMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		metrics.mu.Lock()
		metrics.TotalRequests++
		metrics.ActiveRequests++
		metrics.mu.Unlock()

		defer func() {
			metrics.mu.Lock()
			metrics.ActiveRequests--
			metrics.mu.Unlock()
		}()

		next(w, r)
	}
}

func downloadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONResponse(w, http.StatusMethodNotAllowed, APIResponse{
			Success: false,
			Error:   "Method not allowed",
		})
		return
	}

	// Get query parameters
	symbolRaw := strings.TrimSpace(r.URL.Query().Get("SYMBOL"))
	year := strings.TrimSpace(r.URL.Query().Get("YYYY"))
	month := strings.TrimSpace(r.URL.Query().Get("MM"))
	day := strings.TrimSpace(r.URL.Query().Get("DD"))

	// Validate parameters
	if symbolRaw == "" || year == "" || month == "" || day == "" {
		metrics.mu.Lock()
		metrics.FailedRequests++
		metrics.mu.Unlock()
		writeJSONResponse(w, http.StatusBadRequest, APIResponse{
			Success: false,
			Error:   "Missing required parameters: SYMBOL, YYYY, MM, DD",
		})
		return
	}

	// Validate symbol format (before converting to uppercase)
	if err := validateSymbol(symbolRaw); err != nil {
		metrics.mu.Lock()
		metrics.FailedRequests++
		metrics.mu.Unlock()
		writeJSONResponse(w, http.StatusBadRequest, APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Convert to uppercase after validation
	symbol := strings.ToUpper(symbolRaw)

	// Validate date format
	if err := validateDate(year, month, day); err != nil {
		metrics.mu.Lock()
		metrics.FailedRequests++
		metrics.mu.Unlock()
		writeJSONResponse(w, http.StatusBadRequest, APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), config.Timeout)
	defer cancel()

	// Download and parse trades using connector
	result, err := connector.DownloadTrades(ctx, symbol, year, month, day)
	if err != nil {
		metrics.mu.Lock()
		metrics.FailedRequests++
		metrics.mu.Unlock()
		log.Printf("Error downloading and parsing trades: %v", err)
		writeJSONResponse(w, http.StatusInternalServerError, APIResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to download and parse trades: %v", err),
		})
		return
	}

	metrics.mu.Lock()
	metrics.SuccessfulRequests++
	metrics.mu.Unlock()

	writeJSONResponse(w, http.StatusOK, APIResponse{
		Success: true,
		Message: fmt.Sprintf("Successfully downloaded and parsed %d trades for %s on %s", result.TradeCount, symbol, result.Date),
		Data:    result,
	})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	metrics.mu.RLock()
	health := map[string]interface{}{
		"status":              "healthy",
		"timestamp":           time.Now().UTC().Format(time.RFC3339),
		"total_requests":      metrics.TotalRequests,
		"successful_requests": metrics.SuccessfulRequests,
		"failed_requests":     metrics.FailedRequests,
		"active_requests":     metrics.ActiveRequests,
	}
	metrics.mu.RUnlock()

	writeJSONResponse(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    health,
	})
}

func main() {
	// Setup HTTP server with optimized settings for high load
	mux := http.NewServeMux()
	mux.HandleFunc("/download", requestTrackingMiddleware(downloadHandler))
	mux.HandleFunc("/health", healthHandler)

	server := &http.Server{
		Addr:         ":" + config.Port,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second, // Increased for large JSON responses
		IdleTimeout:  120 * time.Second,
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
