package handlers

import (
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

