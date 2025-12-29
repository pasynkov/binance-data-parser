package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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

func TestDownloadHandler(t *testing.T) {
	// Note: These tests only validate request handling, not actual downloads
	// Actual download tests would require mocking the HTTP client

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
