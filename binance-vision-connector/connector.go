package binancevisionconnector

import (
	"context"
	"fmt"
	"net/http"
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
	downloader *Downloader
	parser     *Parser
	mu         sync.RWMutex
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

	downloader := NewDownloader(client, config.Timeout)
	parser := NewParser()

	return &Connector{
		downloader: downloader,
		parser:     parser,
	}
}

// SetClient sets a custom HTTP client (useful for testing)
func (c *Connector) SetClient(client *http.Client) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.downloader.SetClient(client)
}

// Client returns the current HTTP client (useful for testing)
func (c *Connector) Client() *http.Client {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.downloader.Client()
}

// DownloadTrades downloads and parses trade data for a given symbol and date
func (c *Connector) DownloadTrades(ctx context.Context, symbol, year, month, day string) (*DownloadResult, error) {
	// Download the zip file
	zipData, err := c.downloader.DownloadToMemory(ctx, symbol, year, month, day)
	if err != nil {
		return nil, err
	}

	// Parse the zip file
	trades, err := c.parser.ParseZip(zipData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse zip file: %w", err)
	}

	// Format dates with zero-padding for result
	year, month, day = formatDate(year, month, day)

	result := &DownloadResult{
		Symbol:     symbol,
		Date:       fmt.Sprintf("%s-%s-%s", year, month, day),
		TradeCount: len(trades),
		Trades:     trades,
	}

	return result, nil
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
	return nil, fmt.Errorf("not implemented - use json.Marshal instead")
}
