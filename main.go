package main

import (
	"fmt"
	"os"
)

func getGreeting() string {
	word := os.Getenv("WORD")
	if word == "" {
		word = "World"
	}
	return fmt.Sprintf("Hello %s", word)
}

func main() {
	fmt.Println(getGreeting())
}

