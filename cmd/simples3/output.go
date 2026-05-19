package main

import (
	"encoding/json"
	"fmt"
	"io"
)

type operationsOutput struct {
	Operations []operationResult `json:"operations"`
}

func writeJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func printLine(out io.Writer, format string, args ...any) {
	fmt.Fprintf(out, format+"\n", args...)
}

func printOperation(out io.Writer, result operationResult) {
	if result.Destination != "" {
		printLine(out, "%s %s -> %s", result.Status, result.Source, result.Destination)
		return
	}
	printLine(out, "%s %s", result.Status, result.Source)
}
