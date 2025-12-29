package binancevisionconnector

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Trade represents a single trade record from Binance Vision
type Trade struct {
	TradeID       int64   `json:"trade_id"`
	Price         float64 `json:"price"`
	Quantity      float64 `json:"quantity"`
	QuoteQuantity float64 `json:"quote_quantity"`
	Timestamp     int64   `json:"timestamp"`
	IsBuyerMaker  bool    `json:"is_buyer_maker"`
	IsBestMatch   bool    `json:"is_best_match"`
}

// DownloadResult contains the downloaded trades data
type DownloadResult struct {
	Symbol     string  `json:"symbol"`
	Date       string  `json:"date"`
	TradeCount int     `json:"trade_count"`
	Trades     []Trade `json:"trades"`
}

// Connector handles downloading and parsing Binance Vision trade data
type Connector struct {
	client  *http.Client
	timeout time.Duration
	mu      sync.RWMutex
}

// ConnectorConfig holds configuration for the connector
type ConnectorConfig struct {
	Timeout           time.Duration
	MaxIdleConns      int
	MaxConnsPerHost   int
	IdleConnTimeout   time.Duration
	MaxResponseSize   int64 // Maximum response size in bytes (0 = unlimited)
	MaxTradesPerFile  int   // Maximum trades to parse per file (0 = unlimited)
}

// DefaultConfig returns a default connector configuration
func DefaultConfig() *ConnectorConfig {
	return &ConnectorConfig{
		Timeout:          30 * time.Second,
		MaxIdleConns:     100,
		MaxConnsPerHost:  10,
		IdleConnTimeout:  90 * time.Second,
		MaxResponseSize:  0, // Unlimited by default
		MaxTradesPerFile: 0, // Unlimited by default
	}
}

// NewConnector creates a new Binance Vision connector with default settings
func NewConnector(timeout time.Duration) *Connector {
	return NewConnectorWithConfig(&ConnectorConfig{
		Timeout:         timeout,
		MaxIdleConns:    100,
		MaxConnsPerHost: 10,
		IdleConnTimeout: 90 * time.Second,
	})
}

// NewConnectorWithConfig creates a new connector with custom configuration
func NewConnectorWithConfig(config *ConnectorConfig) *Connector {
	transport := &http.Transport{
		MaxIdleConns:        config.MaxIdleConns,
		MaxConnsPerHost:     config.MaxConnsPerHost,
		IdleConnTimeout:     config.IdleConnTimeout,
		DisableCompression:  false,
		DisableKeepAlives:   false,
		MaxIdleConnsPerHost: config.MaxConnsPerHost,
	}

	client := &http.Client{
		Timeout:   config.Timeout,
		Transport: transport,
	}

	return &Connector{
		client:  client,
		timeout: config.Timeout,
	}
}

// DownloadTrades downloads and parses trade data for a given symbol and date
func (c *Connector) DownloadTrades(ctx context.Context, symbol, year, month, day string) (*DownloadResult, error) {
	// Format dates with zero-padding
	year, month, day = formatDate(year, month, day)

	// Construct the URL
	url := fmt.Sprintf("https://data.binance.vision/data/spot/daily/trades/%s/%s-trades-%s-%s-%s.zip",
		symbol, symbol, year, month, day)

	// Create HTTP request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers for better connection reuse
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	req.Header.Set("User-Agent", "binance-vision-connector/1.0")

	// Download the file
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download file: status code %d", resp.StatusCode)
	}

	// Read the zip file into memory with size limit check
	zipData, err := io.ReadAll(io.LimitReader(resp.Body, 500*1024*1024)) // 500MB limit
	if err != nil {
		return nil, fmt.Errorf("failed to read zip file: %w", err)
	}

	if len(zipData) == 0 {
		return nil, fmt.Errorf("downloaded file is empty")
	}

	// Create a zip reader from the in-memory data
	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, fmt.Errorf("failed to create zip reader: %w", err)
	}

	result := &DownloadResult{
		Symbol:     symbol,
		Date:       fmt.Sprintf("%s-%s-%s", year, month, day),
		TradeCount: 0,
		Trades:     []Trade{},
	}

	// Process CSV files concurrently if multiple files exist
	var wg sync.WaitGroup
	var mu sync.Mutex
	errors := make(chan error, len(zipReader.File))

	// Extract and parse CSV files from the zip
	csvFound := false
	for _, file := range zipReader.File {
		if !file.FileInfo().IsDir() {
			fileName := file.Name
			if len(fileName) > 4 && fileName[len(fileName)-4:] == ".csv" {
				csvFound = true
				wg.Add(1)

				// Process each CSV file in a goroutine
				go func(f *zip.File) {
					defer wg.Done()

					// Open the file from the zip
					rc, err := f.Open()
					if err != nil {
						errors <- fmt.Errorf("failed to open file %s: %w", f.Name, err)
						return
					}
					defer rc.Close()

					// Parse CSV using streaming parser
					trades, err := parseCSVStreaming(rc, 0) // 0 = unlimited
					if err != nil {
						errors <- fmt.Errorf("failed to parse CSV %s: %w", f.Name, err)
						return
					}

					// Thread-safe append to result
					mu.Lock()
					result.Trades = append(result.Trades, trades...)
					result.TradeCount = len(result.Trades)
					mu.Unlock()
				}(file)
			}
		}
	}

	if !csvFound {
		return nil, fmt.Errorf("no CSV files found in the archive")
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errors)

	// Check for errors
	var errs []error
	for err := range errors {
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("errors processing CSV files: %v", errs)
	}

	return result, nil
}

// parseCSVStreaming parses CSV data using streaming to reduce memory usage
func parseCSVStreaming(reader io.Reader, maxTrades int) ([]Trade, error) {
	csvReader := csv.NewReader(reader)
	csvReader.ReuseRecord = true // Reuse record buffer for better performance
	
	var trades []Trade
	var tradesCapacity int
	
	if maxTrades > 0 {
		tradesCapacity = maxTrades
	} else {
		tradesCapacity = 10000 // Initial capacity for better memory allocation
	}
	trades = make([]Trade, 0, tradesCapacity)
	
	headerSkipped := false
	lineNum := 0

	// Read records one by one (streaming)
	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read CSV record at line %d: %w", lineNum+1, err)
		}

		lineNum++

		// Skip header row
		if !headerSkipped {
			if len(record) > 0 && (record[0] == "TradeId" || record[0] == "trade_id" || !isNumeric(record[0])) {
				headerSkipped = true
				continue
			}
			headerSkipped = true
		}

		// Check if we've reached the limit
		if maxTrades > 0 && len(trades) >= maxTrades {
			break
		}

		if len(record) < 7 {
			continue // Skip incomplete records
		}

		trade, err := parseTradeRecord(record)
		if err != nil {
			// Log parsing error but continue with other records
			continue
		}

		trades = append(trades, trade)
	}

	return trades, nil
}

// parseCSV parses CSV data and returns a slice of Trade structs (legacy method, uses streaming internally)
func parseCSV(reader io.Reader) ([]Trade, error) {
	return parseCSVStreaming(reader, 0)
}

// parseTradeRecord parses a single CSV record into a Trade struct
// CSV format: TradeId, Price, Quantity, QuoteQuantity, Timestamp, IsBuyerMaker, IsBestMatch
func parseTradeRecord(record []string) (Trade, error) {
	var trade Trade
	var err error

	// TradeId
	trade.TradeID, err = strconv.ParseInt(record[0], 10, 64)
	if err != nil {
		return trade, fmt.Errorf("invalid TradeId: %w", err)
	}

	// Price
	trade.Price, err = strconv.ParseFloat(record[1], 64)
	if err != nil {
		return trade, fmt.Errorf("invalid Price: %w", err)
	}

	// Quantity
	trade.Quantity, err = strconv.ParseFloat(record[2], 64)
	if err != nil {
		return trade, fmt.Errorf("invalid Quantity: %w", err)
	}

	// QuoteQuantity
	trade.QuoteQuantity, err = strconv.ParseFloat(record[3], 64)
	if err != nil {
		return trade, fmt.Errorf("invalid QuoteQuantity: %w", err)
	}

	// Timestamp
	trade.Timestamp, err = strconv.ParseInt(record[4], 10, 64)
	if err != nil {
		return trade, fmt.Errorf("invalid Timestamp: %w", err)
	}

	// IsBuyerMaker (boolean, typically "true"/"false" or "1"/"0")
	trade.IsBuyerMaker, err = parseBool(record[5])
	if err != nil {
		return trade, fmt.Errorf("invalid IsBuyerMaker: %w", err)
	}

	// IsBestMatch (boolean)
	trade.IsBestMatch, err = parseBool(record[6])
	if err != nil {
		return trade, fmt.Errorf("invalid IsBestMatch: %w", err)
	}

	return trade, nil
}

// parseBool parses various boolean string formats
func parseBool(s string) (bool, error) {
	switch s {
	case "true", "True", "TRUE", "1", "t", "T":
		return true, nil
	case "false", "False", "FALSE", "0", "f", "F":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean value: %s", s)
	}
}

// isNumeric checks if a string is numeric
func isNumeric(s string) bool {
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
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

// ToJSON converts DownloadResult to JSON
func (r *DownloadResult) ToJSON() ([]byte, error) {
	return json.Marshal(r)
}
