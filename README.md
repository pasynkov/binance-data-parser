# Binance Vision Connector

A production-ready Go application that downloads and parses trade data from Binance Vision, returning structured JSON data.

## Features

- ✅ RESTful API with JSON responses
- ✅ Downloads and parses CSV data in memory (no file storage)
- ✅ Returns structured trade data as JSON
- ✅ **High-load optimizations:**
  - Streaming CSV parser (reduces memory usage)
  - Concurrent CSV file processing with goroutines
  - HTTP connection pooling and reuse
  - Request metrics and monitoring
  - Optimized server timeouts and connection limits
- ✅ Input validation (symbol format, date validation)
- ✅ Graceful shutdown
- ✅ Health check endpoint with request metrics
- ✅ Context-aware HTTP requests with timeouts
- ✅ Structured error handling
- ✅ Separate connector module for download logic

## Architecture

The application consists of:
- **Main application** (`main.go`): HTTP server and API handlers
- **Connector module** (`binance-vision-connector/`): Handles downloading and parsing CSV data

## Setup

1. Install dependencies:
```bash
go mod download
```

2. Create a `.env` file (copy from `.env.example`):
```bash
cp .env.example .env
```

3. Configure the `.env` file:
```
PORT=8080
```

Note: `TMP_FOLDER` is no longer required as files are processed in memory.

## Usage

1. Start the server:
```bash
go run main.go
```

2. Make a GET request:
```bash
curl "http://localhost:8080/download?SYMBOL=AIUSDT&YYYY=2025&MM=12&DD=28"
```

The application will:
- Download the zip file from `https://data.binance.vision/data/spot/daily/trades/AIUSDT/AIUSDT-trades-2025-12-28.zip`
- Extract and parse the CSV file in memory
- Return structured JSON data with all trade records

## API Endpoints

### Download Trade Data

**GET** `/download`

Downloads and parses trade data CSV files from Binance Vision, returning structured JSON.

**Query Parameters:**
- `SYMBOL` (required): Trading pair symbol (e.g., AIUSDT, BTCUSDT)
  - Must be uppercase alphanumeric
- `YYYY` (required): Year (e.g., 2025)
  - Must be between 2000-2100
- `MM` (required): Month (e.g., 12 or 1)
  - Must be 1-12 (will be zero-padded automatically)
- `DD` (required): Day (e.g., 28 or 5)
  - Must be 1-31 (will be zero-padded automatically)

**Example Request:**
```bash
curl "http://localhost:8080/download?SYMBOL=AIUSDT&YYYY=2025&MM=12&DD=28"
```

**Success Response (200 OK):**
```json
{
  "success": true,
  "message": "Successfully downloaded and parsed 1234 trades for AIUSDT on 2025-12-28",
  "data": {
    "symbol": "AIUSDT",
    "date": "2025-12-28",
    "trade_count": 1234,
    "trades": [
      {
        "trade_id": 123456789,
        "price": 0.001234,
        "quantity": 100.0,
        "quote_quantity": 0.1234,
        "timestamp": 1735430400000,
        "is_buyer_maker": true,
        "is_best_match": true
      },
      ...
    ]
  }
}
```

**Trade Data Structure:**
- `trade_id` (int64): Unique trade identifier
- `price` (float64): Trade price
- `quantity` (float64): Base asset quantity
- `quote_quantity` (float64): Quote asset quantity
- `timestamp` (int64): Trade timestamp in milliseconds
- `is_buyer_maker` (bool): Whether the buyer is the maker
- `is_best_match` (bool): Whether this is the best match

**Error Response (400 Bad Request):**
```json
{
  "success": false,
  "error": "Missing required parameters: SYMBOL, YYYY, MM, DD"
}
```

**Error Response (500 Internal Server Error):**
```json
{
  "success": false,
  "error": "Failed to download and parse trades: <error details>"
}
```

### Health Check

**GET** `/health`

Checks the health status of the application.

**Example Request:**
```bash
curl "http://localhost:8080/health"
```

**Success Response (200 OK):**
```json
{
  "success": true,
  "data": {
    "status": "healthy",
    "timestamp": "2025-12-29T13:00:00Z",
    "total_requests": 1234,
    "successful_requests": 1200,
    "failed_requests": 34,
    "active_requests": 5
  }
}
```

## Environment Variables

- `PORT` (optional): Server port (defaults to 8080)

## Module Structure

```
binance-vision-connector/
├── main.go                          # HTTP server and handlers
├── binance-vision-connector/        # Connector module
│   └── connector.go                 # Download and CSV parsing logic
├── go.mod                           # Main module definition
└── README.md                        # This file
```

## Development

### Running Tests
```bash
go test -v ./...
```

### Building
```bash
go build -o binance-vision-connector main.go
```

### Running the Binary
```bash
./binance-vision-connector
```

## High-Load Optimizations

The application is optimized for high concurrent load:

1. **Streaming CSV Parser**: Uses `csv.Reader.Read()` instead of `ReadAll()` to process CSV files line-by-line, significantly reducing memory usage for large files
2. **Concurrent Processing**: Multiple CSV files in a zip archive are processed concurrently using goroutines with proper synchronization
3. **HTTP Connection Pooling**: Configured HTTP client with connection reuse:
   - Max idle connections: 100
   - Max connections per host: 10
   - Connection keep-alive enabled
4. **Request Metrics**: Tracks total, successful, failed, and active requests for monitoring
5. **Optimized Server Settings**:
   - Increased write timeout (60s) for large JSON responses
   - Extended idle timeout (120s) for better connection reuse
   - Max header size limit (1MB)
6. **Memory Efficiency**: 
   - CSV reader reuses record buffers (`ReuseRecord = true`)
   - Pre-allocated slice capacity for better memory management
   - 500MB download size limit to prevent memory exhaustion

## Improvements

This version includes several improvements:

1. **Separate Connector Module**: Download and parsing logic moved to `binance-vision-connector` module
2. **In-Memory Processing**: CSV files are parsed in memory, no disk storage required
3. **Structured JSON Data**: Returns parsed trade data as structured JSON instead of file paths
4. **CSV Parsing**: Automatically parses CSV with fields: TradeId, Price, Quantity, QuoteQuantity, Timestamp, IsBuyerMaker, IsBestMatch
5. **Better Error Handling**: Comprehensive error handling with meaningful error messages
6. **Health Check**: `/health` endpoint for monitoring with request metrics
7. **Graceful Shutdown**: Properly handles shutdown signals (SIGINT, SIGTERM)
8. **Context Support**: Uses context for request cancellation and timeouts
9. **High-Load Ready**: Optimized for concurrent request handling with goroutines and connection pooling
