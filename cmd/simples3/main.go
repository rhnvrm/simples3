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
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
	getenv  func(string) string
	homeDir func() (string, error)
}

func newRuntime(stdout, stderr io.Writer) *runtime {
	return &runtime{
		stdin:   os.Stdin,
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
	case "tags":
		err = rt.runTags(cmdArgs)
	case "versioning":
		err = rt.runVersioning(cmdArgs)
	case "versions":
		err = rt.runVersions(cmdArgs)
	case "lifecycle":
		err = rt.runLifecycle(cmdArgs)
	case "acl":
		err = rt.runACL(cmdArgs)
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
	for i := 0; i < len(args); i++ {
		arg := args[i]
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
			name, _, hasInlineValue := strings.Cut(arg, "=")
			if expectsFlagValue(name) && !hasInlineValue && i+1 < len(args) {
				i++
			}
		default:
			return jsonMode
		}
	}
	return jsonMode
}

func expectsFlagValue(name string) bool {
	switch name {
	case "-profile", "--profile",
		"-region", "--region",
		"-endpoint", "--endpoint",
		"-include", "--include",
		"-exclude", "--exclude",
		"-acl", "--acl",
		"-sse", "--sse",
		"-sse-kms-key-id", "--sse-kms-key-id",
		"-version-id", "--version-id",
		"-method", "--method",
		"-expires", "--expires",
		"-response-content-disposition", "--response-content-disposition",
		"-concurrency", "--concurrency",
		"-retries", "--retries",
		"-tag", "--tag",
		"-tags-file", "--tags-file",
		"-status", "--status",
		"-mfa-delete", "--mfa-delete",
		"-delimiter", "--delimiter",
		"-max-keys", "--max-keys",
		"-key-marker", "--key-marker",
		"-version-id-marker", "--version-id-marker",
		"-file", "--file",
		"-policy-file", "--policy-file":
		return true
	default:
		return false
	}
}

func (rt *runtime) printUsage() {
	fmt.Fprint(rt.stderr, `simples3 - simple S3 CLI

Usage:
  simples3 <command> [flags] [arguments]

Commands:
  ls          list buckets or objects
  mb          make bucket
  rb          remove bucket
  presign     generate a presigned object URL
  cp          copy local files and S3 objects
  rm          remove S3 objects and prefixes
  mv          move local files and S3 objects
  sync        synchronize source to destination
  tags        get, set, or delete object tags
  versioning  get or set bucket versioning
  versions    list object versions and delete markers
  lifecycle   get, set, or delete bucket lifecycle config
  acl         get or set bucket or object ACLs

Run 'simples3 help' to see this message.
`)
}
