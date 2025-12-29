package main

import (
	"fmt"
	"os"
)

func main() {
	word := os.Getenv("WORD")
	if word == "" {
		word = "World"
	}
	fmt.Printf("Hello %s\n", word)
}

