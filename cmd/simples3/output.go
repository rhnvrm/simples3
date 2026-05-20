package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type commandEnvelope struct {
	Command string `json:"command"`
	OK      bool   `json:"ok"`
}

type commandErrorDetail struct {
	Type     string `json:"type"`
	Message  string `json:"message"`
	ExitCode int    `json:"exitCode"`
}

type commandErrorOutput struct {
	Command string             `json:"command"`
	OK      bool               `json:"ok"`
	Error   commandErrorDetail `json:"error"`
	Data    any                `json:"data,omitempty"`
}

type operationSummary struct {
	Total     int  `json:"total"`
	Changed   int  `json:"changed,omitempty"`
	Deleted   int  `json:"deleted,omitempty"`
	Failed    int  `json:"failed,omitempty"`
	Unchanged int  `json:"unchanged,omitempty"`
	DryRun    bool `json:"dryRun,omitempty"`
	Noop      bool `json:"noop,omitempty"`
}

type operationsOutput struct {
	commandEnvelope
	Summary    operationSummary  `json:"summary"`
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

func writeOperationsJSON(out io.Writer, command string, results []operationResult) error {
	return writeJSON(out, operationsOutput{
		commandEnvelope: commandEnvelope{Command: command, OK: true},
		Summary:         summarizeOperations(results),
		Operations:      results,
	})
}

func summarizeOperations(results []operationResult) operationSummary {
	summary := operationSummary{Total: len(results)}
	for _, result := range results {
		if result.DryRun {
			summary.DryRun = true
		}
		switch {
		case isDeleteStatus(result.Status):
			summary.Deleted++
		case isChangedStatus(result.Status):
			summary.Changed++
		case isFailureStatus(result.Status):
			summary.Failed++
		case isUnchangedStatus(result.Status):
			summary.Unchanged++
		}
	}
	summary.Noop = summary.Total == 0 || (summary.Changed == 0 && summary.Deleted == 0 && summary.Failed == 0)
	return summary
}

func isChangedStatus(status string) bool {
	return status == "copied" || status == "moved" || status == "synced" || status == "would-copy" || status == "would-move" || status == "would-sync"
}

func isDeleteStatus(status string) bool {
	return status == "deleted" || status == "would-delete"
}

func isFailureStatus(status string) bool {
	return status == "failed" || strings.HasPrefix(status, "failed-")
}

func isUnchangedStatus(status string) bool {
	return status == "unchanged"
}
