package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type runtime struct {
	stdout  io.Writer
	stderr  io.Writer
	getenv  func(string) string
	homeDir func() (string, error)
}

func newRuntime(stdout, stderr io.Writer) *runtime {
	return &runtime{
		stdout:  stdout,
		stderr:  stderr,
		getenv:  os.Getenv,
		homeDir: os.UserHomeDir,
	}
}

func main() {
	os.Exit(newRuntime(os.Stdout, os.Stderr).run(os.Args[1:]))
}

func (rt *runtime) run(args []string) int {
	if len(args) == 0 {
		rt.printUsage()
		return 1
	}

	cmd := args[0]
	cmdArgs := args[1:]
	jsonMode := wantsJSON(cmdArgs)

	var err error
	switch cmd {
	case "help", "-h", "--help":
		rt.printUsage()
		return 0
	case "ls":
		err = rt.runList(cmdArgs)
	case "mb":
		err = rt.runMakeBucket(cmdArgs)
	case "rb":
		err = rt.runRemoveBucket(cmdArgs)
	case "presign":
		err = rt.runPresign(cmdArgs)
	case "cp":
		err = rt.runCopy(cmdArgs)
	case "rm":
		err = rt.runRemove(cmdArgs)
	case "mv":
		err = rt.runMove(cmdArgs)
	case "sync":
		err = rt.runSync(cmdArgs)
	default:
		err = usageErrorf("unknown command %q", cmd)
	}

	if err == nil {
		return 0
	}
	if errors.Is(err, flagErrHelp) {
		return 0
	}
	if jsonMode {
		if encodeErr := writeJSON(rt.stdout, errorOutputForCommand(cmd, err)); encodeErr != nil {
			fmt.Fprintf(rt.stderr, "error: %v\n", err)
			return 1
		}
	} else {
		fmt.Fprintf(rt.stderr, "error: %v\n", err)
	}
	return exitCodeForError(err)
}

func wantsJSON(args []string) bool {
	jsonMode := false
	for _, arg := range args {
		switch {
		case arg == "--":
			return jsonMode
		case arg == "-json" || arg == "--json":
			jsonMode = true
		case strings.HasPrefix(arg, "-json=") || strings.HasPrefix(arg, "--json="):
			_, value, _ := strings.Cut(arg, "=")
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return true
			}
			jsonMode = parsed
		case strings.HasPrefix(arg, "-"):
			continue
		default:
			return jsonMode
		}
	}
	return jsonMode
}

func (rt *runtime) printUsage() {
	fmt.Fprint(rt.stderr, `simples3 - simple S3 CLI

Usage:
  simples3 <command> [flags] [arguments]

Commands:
  ls       list buckets or objects
  mb       make bucket
  rb       remove bucket
  presign  generate a presigned object URL
  cp       copy local files and S3 objects
  rm       remove S3 objects and prefixes
  mv       move local files and S3 objects
  sync     synchronize source to destination

Run 'simples3 help' to see this message.
`)
}
