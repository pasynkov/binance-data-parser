package main

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	binancevisionconnector "binance-vision-connector/binance-vision-connector"
)

func TestValidateSymbol(t *testing.T) {
	tests := []struct {
		name    string
		symbol  string
		wantErr bool
	}{
		{"valid symbol", "AIUSDT", false},
		{"valid symbol with numbers", "BTC123", false},
		{"empty symbol", "", true},
		{"lowercase symbol", "aiusdt", true},
		{"symbol with special chars", "AI-USDT", true},
		{"symbol with spaces", "AI USDT", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSymbol(tt.symbol)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSymbol(%q) error = %v, wantErr %v", tt.symbol, err, tt.wantErr)
			}
		})
	}
}

func TestValidateDate(t *testing.T) {
	tests := []struct {
		name    string
		year    string
		month   string
		day     string
		wantErr bool
	}{
		{"valid date", "2025", "12", "28", false},
		{"valid date single digit", "2025", "1", "5", false},
		{"invalid year too old", "1999", "12", "28", true},
		{"invalid year too new", "2101", "12", "28", true},
		{"invalid month too high", "2025", "13", "28", true},
		{"invalid month zero", "2025", "0", "28", true},
		{"invalid day too high", "2025", "12", "32", true},
		{"invalid day zero", "2025", "12", "0", true},
		{"invalid date Feb 30", "2025", "02", "30", true},
		{"invalid date Feb 29 non-leap", "2025", "02", "29", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDate(tt.year, tt.month, tt.day)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDate(%q, %q, %q) error = %v, wantErr %v", tt.year, tt.month, tt.day, err, tt.wantErr)
			}
		})
	}
}

func TestFormatDate(t *testing.T) {
	tests := []struct {
		name         string
		year, month, day string
		wantYear, wantMonth, wantDay string
	}{
		{"already formatted", "2025", "12", "28", "2025", "12", "28"},
		{"single digit month", "2025", "1", "28", "2025", "01", "28"},
		{"single digit day", "2025", "12", "5", "2025", "12", "05"},
		{"both single digit", "2025", "1", "5", "2025", "01", "05"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotYear, gotMonth, gotDay := formatDate(tt.year, tt.month, tt.day)
			if gotYear != tt.wantYear || gotMonth != tt.wantMonth || gotDay != tt.wantDay {
				t.Errorf("formatDate(%q, %q, %q) = (%q, %q, %q), want (%q, %q, %q)",
					tt.year, tt.month, tt.day, gotYear, gotMonth, gotDay, tt.wantYear, tt.wantMonth, tt.wantDay)
			}
		})
	}
}

// createMockZipFile creates a zip file with CSV data for testing
func createMockZipFile(symbol, year, month, day string, trades [][]string) ([]byte, error) {
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	// Create CSV file in zip
	csvFileName := fmt.Sprintf("%s-trades-%s-%s-%s.csv", symbol, year, month, day)
	csvFile, err := zipWriter.Create(csvFileName)
	if err != nil {
		return nil, err
	}

	// Write CSV data
	csvWriter := csv.NewWriter(csvFile)
	
	// Write header
	header := []string{"TradeId", "Price", "Quantity", "QuoteQuantity", "Timestamp", "IsBuyerMaker", "IsBestMatch"}
	if err := csvWriter.Write(header); err != nil {
		return nil, err
	}

	// Write trade data
	for _, trade := range trades {
		if err := csvWriter.Write(trade); err != nil {
			return nil, err
		}
	}

	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		return nil, err
	}

	if err := zipWriter.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// setupMockBinanceServer creates a test HTTP server that serves mock zip files
func setupMockBinanceServer(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse URL to extract symbol and date
		path := r.URL.Path
		if !strings.Contains(path, "/data/spot/daily/trades/") {
			http.NotFound(w, r)
			return
		}

		// Extract symbol from path (simplified parsing)
		parts := strings.Split(path, "/")
		if len(parts) < 6 {
			http.NotFound(w, r)
			return
		}

		symbol := parts[5]
		fileName := parts[6]
		
		// Extract date from filename: SYMBOL-trades-YYYY-MM-DD.zip
		datePart := strings.TrimPrefix(fileName, symbol+"-trades-")
		datePart = strings.TrimSuffix(datePart, ".zip")
		dateParts := strings.Split(datePart, "-")
		if len(dateParts) != 3 {
			http.NotFound(w, r)
			return
		}

		year, month, day := dateParts[0], dateParts[1], dateParts[2]

		// Create mock trade data
		trades := [][]string{
			{"123456789", "0.001234", "100.0", "0.1234", "1735430400000", "true", "true"},
			{"123456790", "0.001235", "200.0", "0.2470", "1735430401000", "false", "true"},
			{"123456791", "0.001236", "150.0", "0.1854", "1735430402000", "true", "false"},
		}

		// Create zip file
		zipData, err := createMockZipFile(symbol, year, month, day, trades)
		if err != nil {
			http.Error(w, "Failed to create zip file", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/zip")
		w.WriteHeader(http.StatusOK)
		w.Write(zipData)
	}))
}


// urlRewritingTransport rewrites URLs to point to mock server
type urlRewritingTransport struct {
	baseURL  string
	transport http.RoundTripper
}

func (t *urlRewritingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Rewrite binance.vision URLs to mock server
	if strings.Contains(req.URL.Host, "data.binance.vision") {
		req.URL.Host = strings.TrimPrefix(t.baseURL, "http://")
		req.URL.Host = strings.TrimPrefix(req.URL.Host, "https://")
		req.URL.Scheme = "http"
	}
	return t.transport.RoundTrip(req)
}

// TestE2E_DownloadEndpoint tests the full flow: server -> request -> response
func TestE2E_DownloadEndpoint(t *testing.T) {
	// Setup mock Binance Vision server
	mockBinanceServer := setupMockBinanceServer(t)
	defer mockBinanceServer.Close()

	// Create a custom connector that uses the mock server
	originalConnector := connector
	
	// Create transport that rewrites URLs
	rewriteTransport := &urlRewritingTransport{
		baseURL: mockBinanceServer.URL,
		transport: &http.Transport{},
	}

	// Create test connector with custom client
	testConnectorConfig := binancevisionconnector.DefaultConfig()
	testConnectorConfig.Timeout = 10 * time.Second
	testConnector := binancevisionconnector.NewConnectorWithConfig(testConnectorConfig)
	
	// Replace connector's client with transport that rewrites URLs
	
	testConnector.SetClient(&http.Client{
		Timeout:   10 * time.Second,
		Transport: rewriteTransport,
	})

	// Temporarily replace global connector
	connector = testConnector
	defer func() {
		connector = originalConnector
	}()

	// Start the application server
	mux := http.NewServeMux()
	mux.HandleFunc("/download", requestTrackingMiddleware(downloadHandler))
	mux.HandleFunc("/health", healthHandler)

	testServer := httptest.NewServer(mux)
	defer testServer.Close()

	// Make request to download endpoint
	url := fmt.Sprintf("%s/download?SYMBOL=AIUSDT&YYYY=2025&MM=12&DD=28", testServer.URL)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		bodyBytes := make([]byte, 1024)
		resp.Body.Read(bodyBytes)
		t.Logf("Response body: %s", string(bodyBytes))
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
		return
	}

	// Parse JSON response
	var apiResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		t.Fatalf("Failed to decode JSON response: %v", err)
	}

	// Verify response structure
	if !apiResp.Success {
		t.Errorf("Expected success=true, got success=%v, error=%s", apiResp.Success, apiResp.Error)
	}

	// Verify data structure
	dataBytes, err := json.Marshal(apiResp.Data)
	if err != nil {
		t.Fatalf("Failed to marshal data: %v", err)
	}

	var downloadResult struct {
		Symbol     string `json:"symbol"`
		Date       string `json:"date"`
		TradeCount int    `json:"trade_count"`
		Trades     []struct {
			TradeID       int64   `json:"trade_id"`
			Price         float64 `json:"price"`
			Quantity      float64 `json:"quantity"`
			QuoteQuantity float64 `json:"quote_quantity"`
			Timestamp     int64   `json:"timestamp"`
			IsBuyerMaker  bool    `json:"is_buyer_maker"`
			IsBestMatch   bool    `json:"is_best_match"`
		} `json:"trades"`
	}

	if err := json.Unmarshal(dataBytes, &downloadResult); err != nil {
		t.Fatalf("Failed to unmarshal download result: %v", err)
	}

	// Verify data content
	if downloadResult.Symbol != "AIUSDT" {
		t.Errorf("Expected symbol AIUSDT, got %s", downloadResult.Symbol)
	}

	if downloadResult.Date != "2025-12-28" {
		t.Errorf("Expected date 2025-12-28, got %s", downloadResult.Date)
	}

	if downloadResult.TradeCount != 3 {
		t.Errorf("Expected 3 trades, got %d", downloadResult.TradeCount)
	}

	if len(downloadResult.Trades) != 3 {
		t.Errorf("Expected 3 trades in array, got %d", len(downloadResult.Trades))
	}

	// Verify first trade
	if downloadResult.Trades[0].TradeID != 123456789 {
		t.Errorf("Expected trade_id 123456789, got %d", downloadResult.Trades[0].TradeID)
	}

	if downloadResult.Trades[0].Price != 0.001234 {
		t.Errorf("Expected price 0.001234, got %f", downloadResult.Trades[0].Price)
	}
}

// TestE2E_DownloadEndpoint_ErrorCases tests error scenarios end-to-end
func TestE2E_DownloadEndpoint_ErrorCases(t *testing.T) {
	// Setup mock Binance Vision server
	mockBinanceServer := setupMockBinanceServer(t)
	defer mockBinanceServer.Close()

	// Start the application server
	mux := http.NewServeMux()
	mux.HandleFunc("/download", requestTrackingMiddleware(downloadHandler))
	mux.HandleFunc("/health", healthHandler)

	testServer := httptest.NewServer(mux)
	defer testServer.Close()

	tests := []struct {
		name           string
		method         string
		queryParams    string
		expectedStatus int
		expectedError   string
	}{
		{
			name:           "missing parameters",
			method:         "GET",
			queryParams:    "",
			expectedStatus: http.StatusBadRequest,
			expectedError:   "Missing required parameters",
		},
		{
			name:           "invalid symbol format",
			method:         "GET",
			queryParams:    "SYMBOL=ai-usdt&YYYY=2025&MM=12&DD=28",
			expectedStatus: http.StatusBadRequest,
			expectedError:   "invalid symbol format",
		},
		{
			name:           "invalid date",
			method:         "GET",
			queryParams:    "SYMBOL=AIUSDT&YYYY=2025&MM=13&DD=28",
			expectedStatus: http.StatusBadRequest,
			expectedError:   "invalid month",
		},
		{
			name:           "wrong HTTP method",
			method:         "POST",
			queryParams:    "SYMBOL=AIUSDT&YYYY=2025&MM=12&DD=28",
			expectedStatus: http.StatusMethodNotAllowed,
			expectedError:   "Method not allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			var err error

			if tt.name == "wrong HTTP method" {
				req, err = http.NewRequest("POST", testServer.URL+"/download?"+tt.queryParams, nil)
			} else {
				req, err = http.NewRequest("GET", testServer.URL+"/download?"+tt.queryParams, nil)
			}

			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Failed to make request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, resp.StatusCode)
			}

			var apiResp APIResponse
			if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
				t.Fatalf("Failed to decode JSON response: %v", err)
			}

			if apiResp.Success {
				t.Errorf("Expected success=false, got success=true")
			}

			if !strings.Contains(apiResp.Error, tt.expectedError) {
				t.Errorf("Expected error to contain '%s', got '%s'", tt.expectedError, apiResp.Error)
			}
		})
	}
}

// TestE2E_HealthEndpoint tests the health endpoint end-to-end
func TestE2E_HealthEndpoint(t *testing.T) {
	// Start the application server
	mux := http.NewServeMux()
	mux.HandleFunc("/download", requestTrackingMiddleware(downloadHandler))
	mux.HandleFunc("/health", healthHandler)

	testServer := httptest.NewServer(mux)
	defer testServer.Close()

	// Make request to health endpoint
	resp, err := http.Get(testServer.URL + "/health")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Parse JSON response
	var apiResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		t.Fatalf("Failed to decode JSON response: %v", err)
	}

	// Verify response structure
	if !apiResp.Success {
		t.Errorf("Expected success=true, got success=%v", apiResp.Success)
	}

	// Verify health data structure
	dataBytes, err := json.Marshal(apiResp.Data)
	if err != nil {
		t.Fatalf("Failed to marshal data: %v", err)
	}

	var healthData map[string]interface{}
	if err := json.Unmarshal(dataBytes, &healthData); err != nil {
		t.Fatalf("Failed to unmarshal health data: %v", err)
	}

	// Verify required fields
	if status, ok := healthData["status"].(string); !ok || status != "healthy" {
		t.Errorf("Expected status='healthy', got %v", healthData["status"])
	}

	if _, ok := healthData["timestamp"]; !ok {
		t.Error("Expected timestamp field in health response")
	}

	if _, ok := healthData["total_requests"]; !ok {
		t.Error("Expected total_requests field in health response")
	}
}

// TestE2E_ConcurrentRequests tests handling multiple concurrent requests
func TestE2E_ConcurrentRequests(t *testing.T) {
	// Start the application server
	mux := http.NewServeMux()
	mux.HandleFunc("/download", requestTrackingMiddleware(downloadHandler))
	mux.HandleFunc("/health", healthHandler)

	testServer := httptest.NewServer(mux)
	defer testServer.Close()

	// Make multiple concurrent requests
	const numRequests = 10
	results := make(chan error, numRequests)
	var wg sync.WaitGroup

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get(testServer.URL + "/health")
			if err != nil {
				results <- err
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				results <- fmt.Errorf("expected status 200, got %d", resp.StatusCode)
				return
			}

			var apiResp APIResponse
			if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
				results <- err
				return
			}

			if !apiResp.Success {
				results <- fmt.Errorf("expected success=true")
				return
			}

			results <- nil
		}()
	}

	wg.Wait()
	close(results)

	// Check results
	for err := range results {
		if err != nil {
			t.Errorf("Request failed: %v", err)
		}
	}
}

// TestDownloadHandler tests handler directly (unit test)
func TestDownloadHandler(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		queryParams    string
		expectedStatus int
		expectError    bool
	}{
		{
			name:           "missing parameters",
			method:         "GET",
			queryParams:    "",
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name:           "invalid symbol format",
			method:         "GET",
			queryParams:    "SYMBOL=ai-usdt&YYYY=2025&MM=12&DD=28",
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name:           "invalid date",
			method:         "GET",
			queryParams:    "SYMBOL=AIUSDT&YYYY=2025&MM=13&DD=28",
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name:           "wrong HTTP method",
			method:         "POST",
			queryParams:    "SYMBOL=AIUSDT&YYYY=2025&MM=12&DD=28",
			expectedStatus: http.StatusMethodNotAllowed,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/download?"+tt.queryParams, nil)
			w := httptest.NewRecorder()

			downloadHandler(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("downloadHandler() status code = %d, want %d", w.Code, tt.expectedStatus)
			}

			if tt.expectError {
				var response APIResponse
				if err := json.Unmarshal(w.Body.Bytes(), &response); err == nil {
					if response.Success {
						t.Errorf("Expected error response but got success")
					}
				}
			}
		})
	}
}

// TestHealthHandler tests health handler directly (unit test)
func TestHealthHandler(t *testing.T) {
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	healthHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("healthHandler() status code = %d, want %d", w.Code, http.StatusOK)
	}

	var response APIResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Errorf("Failed to parse JSON response: %v", err)
	}

	if !response.Success {
		t.Errorf("Expected health check to succeed")
	}
}
