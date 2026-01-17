// Package main provides the entry point for the sweep disk analyzer CLI.
package main

import (
	"os"
)

func main() {
	if err := Execute(); err != nil {
		os.Exit(1)
	}
}
