package main

import (
	"errors"
	"fmt"
)

type cliErrorKind string

const (
	cliErrorUsage   cliErrorKind = "usage"
	cliErrorRuntime cliErrorKind = "runtime"
	cliErrorPartial cliErrorKind = "partial_failure"
)

type cliError struct {
	kind    cliErrorKind
	message string
	cause   error
	output  any
}

func (e *cliError) Error() string {
	switch {
	case e == nil:
		return ""
	case e.message != "":
		return e.message
	case e.cause != nil:
		return e.cause.Error()
	default:
		return string(e.kind)
	}
}

func (e *cliError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func usageErrorf(format string, args ...any) error {
	return &cliError{kind: cliErrorUsage, message: fmt.Sprintf(format, args...)}
}

func partialFailureError(message string, output any) error {
	return &cliError{kind: cliErrorPartial, message: message, output: output}
}

func exitCodeForError(err error) int {
	if err == nil || errors.Is(err, flagErrHelp) {
		return 0
	}
	var cliErr *cliError
	if errors.As(err, &cliErr) {
		switch cliErr.kind {
		case cliErrorUsage:
			return 2
		case cliErrorPartial:
			return 3
		default:
			return 1
		}
	}
	return 1
}

func errorKindForError(err error) cliErrorKind {
	var cliErr *cliError
	if errors.As(err, &cliErr) {
		return cliErr.kind
	}
	return cliErrorRuntime
}

func errorOutputForCommand(command string, err error) commandErrorOutput {
	output := commandErrorOutput{
		Command: command,
		OK:      false,
		Error: commandErrorDetail{
			Type:     string(errorKindForError(err)),
			Message:  err.Error(),
			ExitCode: exitCodeForError(err),
		},
	}
	var cliErr *cliError
	if errors.As(err, &cliErr) && cliErr.output != nil {
		output.Data = cliErr.output
	}
	return output
}
