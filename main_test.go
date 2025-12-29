package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestGetGreeting(t *testing.T) {
	tests := []struct {
		name     string
		wordEnv  string
		expected string
	}{
		{
			name:     "default when WORD is not set",
			wordEnv:  "",
			expected: "Hello World",
		},
		{
			name:     "custom word from env",
			wordEnv:  "Go",
			expected: "Hello Go",
		},
		{
			name:     "empty word env variable",
			wordEnv:  "",
			expected: "Hello World",
		},
		{
			name:     "special characters",
			wordEnv:  "123",
			expected: "Hello 123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original env value
			originalWord := os.Getenv("WORD")
			defer os.Setenv("WORD", originalWord)

			// Set test env value
			if tt.wordEnv != "" {
				os.Setenv("WORD", tt.wordEnv)
			} else {
				os.Unsetenv("WORD")
			}

			// Test the function
			result := getGreeting()
			if result != tt.expected {
				t.Errorf("getGreeting() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestMainFunctionE2E(t *testing.T) {
	tests := []struct {
		name     string
		wordEnv  string
		expected string
	}{
		{
			name:     "default output",
			wordEnv:  "",
			expected: "Hello World",
		},
		{
			name:     "custom word output",
			wordEnv:  "Testing",
			expected: "Hello Testing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build the binary
			buildCmd := exec.Command("go", "build", "-o", "test-binance-data-parser", "main.go")
			if err := buildCmd.Run(); err != nil {
				t.Fatalf("Failed to build binary: %v", err)
			}
			defer os.Remove("test-binance-data-parser")

			// Run the binary with environment variable
			cmd := exec.Command("./test-binance-data-parser")
			if tt.wordEnv != "" {
				cmd.Env = append(os.Environ(), "WORD="+tt.wordEnv)
			} else {
				// Remove WORD from env if it exists
				env := os.Environ()
				newEnv := []string{}
				for _, e := range env {
					if !strings.HasPrefix(e, "WORD=") {
						newEnv = append(newEnv, e)
					}
				}
				cmd.Env = newEnv
			}

			output, err := cmd.Output()
			if err != nil {
				t.Fatalf("Failed to run binary: %v", err)
			}

			result := strings.TrimSpace(string(output))
			if result != tt.expected {
				t.Errorf("main() output = %q, want %q", result, tt.expected)
			}
		})
	}
}

