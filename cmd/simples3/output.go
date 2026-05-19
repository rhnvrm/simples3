package main

import (
	"encoding/json"
	"fmt"
	"io"
)

func writeJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func printLine(out io.Writer, format string, args ...any) {
	fmt.Fprintf(out, format+"\n", args...)
}
