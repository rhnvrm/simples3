package main

import (
	"fmt"
)

type copyFlags struct {
	awsFlags
	recursive       bool
	dryRun          bool
	continueOnError bool
	concurrency     int
	retries         int
	includes        stringListFlag
	excludes        stringListFlag
	acl             string
	sse             string
	sseKMS          string
	versionID       string
}

func (rt *runtime) runCopy(args []string) error {
	flags := copyFlags{}
	fs := newFlagSet("cp", func() {
		fmt.Fprint(rt.stderr, `Usage: simples3 cp [flags] <source> <destination>

Copy a local file or S3 object between local paths and s3:// locations.

Flags:
  --recursive           copy directories or S3 prefixes recursively
  --dry-run             print planned operations without executing them
  --include <pattern>   include glob pattern (repeatable)
  --exclude <pattern>   exclude glob pattern (repeatable)
  --continue-on-error   keep processing remaining operations after failures
  --concurrency <n>     number of operations to run in parallel (default 1)
  --retries <n>         retry transient failures this many times (default 2)
  --acl <value>         canned ACL for uploads to S3
  --sse <value>         server-side encryption mode for S3 destinations
  --sse-kms-key-id <id> KMS key ID when using aws:kms
  --version-id <id>     source object version for exact S3 downloads
  --profile <name>      AWS profile name
  --region <name>       AWS region
  --endpoint <url>      custom S3 endpoint URL
  --json                emit JSON output
`)
	})
	addAWSFlags(fs, &flags.awsFlags)
	fs.BoolVar(&flags.recursive, "recursive", false, "copy recursively")
	fs.BoolVar(&flags.dryRun, "dry-run", false, "print operations without executing")
	fs.Var(&flags.includes, "include", "include glob pattern")
	fs.Var(&flags.excludes, "exclude", "exclude glob pattern")
	fs.BoolVar(&flags.continueOnError, "continue-on-error", false, "continue processing after failures")
	fs.IntVar(&flags.concurrency, "concurrency", 1, "number of parallel operations")
	fs.IntVar(&flags.retries, "retries", 2, "number of retries for transient failures")
	fs.StringVar(&flags.acl, "acl", "", "canned ACL for uploads")
	fs.StringVar(&flags.sse, "sse", "", "server-side encryption mode")
	fs.StringVar(&flags.sseKMS, "sse-kms-key-id", "", "KMS key ID")
	fs.StringVar(&flags.versionID, "version-id", "", "source object version ID")
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return usageErrorf("cp requires a source and destination")
	}
	flags.sse = normalizeSSE(flags.sse, flags.sseKMS)

	source, err := parseLocation(fs.Arg(0))
	if err != nil {
		return err
	}
	destination, err := parseLocation(fs.Arg(1))
	if err != nil {
		return err
	}
	if source.isLocal() && destination.isLocal() {
		return usageErrorf("local-to-local copies are not supported")
	}
	if destination.isLocal() && (flags.acl != "" || flags.sse != "" || flags.sseKMS != "") {
		return usageErrorf("--acl/--sse flags require an S3 destination")
	}
	if err := validateRecursiveSource(source, flags.recursive); err != nil {
		return err
	}
	if flags.versionID != "" {
		if source.isLocal() {
			return usageErrorf("--version-id requires an S3 source")
		}
		if flags.recursive {
			return usageErrorf("--version-id is only supported for single-object copies")
		}
		if destination.isS3() {
			return usageErrorf("--version-id is currently only supported for downloads to local paths")
		}
	}

	if flags.concurrency < 1 {
		return usageErrorf("--concurrency must be at least 1")
	}
	if flags.retries < 0 {
		return usageErrorf("--retries cannot be negative")
	}

	settings, err := rt.resolveAWSSettings(flags.awsFlags)
	if err != nil {
		return err
	}
	matcher, err := newMatcher([]string(flags.includes), []string(flags.excludes))
	if err != nil {
		return err
	}
	client := settings.newClient()
	entries, err := collectSourceEntries(client, source, flags.recursive, matcher, flags.versionID)
	if err != nil {
		return err
	}
	if err := ensureSingleSourceEntry(source, flags.recursive, entries); err != nil {
		return err
	}

	ops := make([]plannedOperation, 0, len(entries))
	options := copyOptions{
		executionOptions: executionOptions{DryRun: flags.dryRun, Concurrency: flags.concurrency, Retries: flags.retries, ContinueOnError: flags.continueOnError},
		EmitProgress:     !flags.json,
		ACL:              flags.acl,
		SSE:              flags.sse,
		SSEKMS:           flags.sseKMS,
	}
	for _, entry := range entries {
		target, err := resolveTarget(entry, destination, flags.recursive)
		if err != nil {
			return err
		}
		result := operationResult{Action: "copy", Source: entrySourceString(entry), Destination: targetString(target), Status: "copied", Size: entry.Size, DryRun: flags.dryRun}
		if flags.dryRun {
			result.Status = "would-copy"
		}
		entryCopy := entry
		targetCopy := target
		ops = append(ops, plannedOperation{result: result, run: func() error {
			return copyEntry(client, entryCopy, targetCopy, options, rt)
		}})
	}
	results := executePlannedOperations(ops, options.executionOptions)
	if !flags.json {
		printOperations(rt.stdout, results)
	}
	if err := operationsPartialFailure("cp", results); err != nil {
		return err
	}
	if flags.json {
		return writeOperationsJSON(rt.stdout, "cp", results)
	}
	return nil
}
