package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	binancevisionconnector "binance-vision-connector/binance-vision-connector"
)

// DownloadHandler handles download requests
type DownloadHandler struct {
	Connector *binancevisionconnector.Connector
	Timeout   time.Duration
	Metrics   *RequestMetrics
}

// APIResponse represents a standard API response
type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// WriteJSONResponse writes a JSON response
func WriteJSONResponse(w http.ResponseWriter, statusCode int, response APIResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode JSON response: %v", err)
	}
}

// Handle handles download requests
func (h *DownloadHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONResponse(w, http.StatusMethodNotAllowed, APIResponse{
			Success: false,
			Error:   "Method not allowed",
		})
		return
	}

	// Track request
	h.Metrics.TotalRequests++
	h.Metrics.ActiveRequests++
	defer func() {
		h.Metrics.ActiveRequests--
	}()

	// Get query parameters
	symbolRaw := strings.TrimSpace(r.URL.Query().Get("SYMBOL"))
	year := strings.TrimSpace(r.URL.Query().Get("YYYY"))
	month := strings.TrimSpace(r.URL.Query().Get("MM"))
	day := strings.TrimSpace(r.URL.Query().Get("DD"))

	// Validate parameters
	if symbolRaw == "" || year == "" || month == "" || day == "" {
		h.Metrics.FailedRequests++
		WriteJSONResponse(w, http.StatusBadRequest, APIResponse{
			Success: false,
			Error:   "Missing required parameters: SYMBOL, YYYY, MM, DD",
		})
		return
	}

	// Validate symbol format (before converting to uppercase)
	if err := validateSymbol(symbolRaw); err != nil {
		h.Metrics.FailedRequests++
		WriteJSONResponse(w, http.StatusBadRequest, APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Convert to uppercase after validation
	symbol := strings.ToUpper(symbolRaw)

	// Validate date format
	if err := validateDate(year, month, day); err != nil {
		h.Metrics.FailedRequests++
		WriteJSONResponse(w, http.StatusBadRequest, APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), h.Timeout)
	defer cancel()

	// Download and parse trades using connector
	result, err := h.Connector.DownloadTrades(ctx, symbol, year, month, day)
	if err != nil {
		h.Metrics.FailedRequests++
		log.Printf("Error downloading and parsing trades: %v", err)
		WriteJSONResponse(w, http.StatusInternalServerError, APIResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to download and parse trades: %v", err),
		})
		return
	}

	h.Metrics.SuccessfulRequests++

	WriteJSONResponse(w, http.StatusOK, APIResponse{
		Success: true,
		Message: fmt.Sprintf("Successfully downloaded and parsed %d trades for %s on %s", result.TradeCount, symbol, result.Date),
		Data:    result,
	})
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

