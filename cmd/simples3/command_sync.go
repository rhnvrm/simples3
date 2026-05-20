package main

import (
	"fmt"
	"os"
)

type syncFlags struct {
	awsFlags
	recursive       bool
	dryRun          bool
	plan            bool
	delete          bool
	continueOnError bool
	concurrency     int
	retries         int
	includes        stringListFlag
	excludes        stringListFlag
	acl             string
	sse             string
	sseKMS          string
}

func (rt *runtime) runSync(args []string) error {
	flags := syncFlags{}
	fs := newFlagSet("sync", func() {
		fmt.Fprint(rt.stderr, `Usage: simples3 sync [flags] <source> <destination>

Synchronize source to destination. This is a one-way copy from source to destination.

Flags:
  --recursive           recurse into directories or S3 prefixes
  --dry-run             print planned operations without executing them
  --plan                emit a full sync plan, including unchanged entries (implies --dry-run)
  --delete              delete destination files missing from source
  --include <pattern>   include glob pattern (repeatable)
  --exclude <pattern>   exclude glob pattern (repeatable)
  --continue-on-error   keep processing remaining operations after failures
  --concurrency <n>     number of operations to run in parallel (default 1)
  --retries <n>         retry transient failures this many times (default 2)
  --acl <value>         canned ACL for uploads to S3
  --sse <value>         server-side encryption mode for S3 destinations
  --sse-kms-key-id <id> KMS key ID when using aws:kms
  --profile <name>      AWS profile name
  --region <name>       AWS region
  --endpoint <url>      custom S3 endpoint URL
  --json                emit JSON output
`)
	})
	addAWSFlags(fs, &flags.awsFlags)
	fs.BoolVar(&flags.recursive, "recursive", false, "sync recursively")
	fs.BoolVar(&flags.dryRun, "dry-run", false, "print operations without executing")
	fs.BoolVar(&flags.plan, "plan", false, "show a full sync plan including unchanged entries")
	fs.BoolVar(&flags.delete, "delete", false, "delete destination entries missing from source")
	fs.Var(&flags.includes, "include", "include glob pattern")
	fs.Var(&flags.excludes, "exclude", "exclude glob pattern")
	fs.BoolVar(&flags.continueOnError, "continue-on-error", false, "continue processing after failures")
	fs.IntVar(&flags.concurrency, "concurrency", 1, "number of parallel operations")
	fs.IntVar(&flags.retries, "retries", 2, "number of retries for transient failures")
	fs.StringVar(&flags.acl, "acl", "", "canned ACL for uploads")
	fs.StringVar(&flags.sse, "sse", "", "server-side encryption mode")
	fs.StringVar(&flags.sseKMS, "sse-kms-key-id", "", "KMS key ID")
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return usageErrorf("sync requires a source and destination")
	}
	flags.sse = normalizeSSE(flags.sse, flags.sseKMS)
	flags.dryRun = flags.dryRun || flags.plan

	source, err := parseLocation(fs.Arg(0))
	if err != nil {
		return err
	}
	destination, err := parseLocation(fs.Arg(1))
	if err != nil {
		return err
	}
	if source.isLocal() && destination.isLocal() {
		return usageErrorf("local-to-local sync is not supported")
	}
	if destination.isLocal() && (flags.acl != "" || flags.sse != "" || flags.sseKMS != "") {
		return usageErrorf("--acl/--sse flags require an S3 destination")
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
	recursive, err := inferSyncRecursion(source, flags.recursive)
	if err != nil {
		return err
	}
	if flags.delete && !recursive {
		return usageErrorf("--delete is only supported for recursive syncs")
	}

	entries, err := collectSourceEntries(client, source, recursive, matcher, "")
	if err != nil {
		return err
	}

	results := []operationResult{}
	seen := map[string]struct{}{}
	options := copyOptions{
		executionOptions: executionOptions{DryRun: flags.dryRun, Concurrency: flags.concurrency, Retries: flags.retries, ContinueOnError: flags.continueOnError},
		EmitProgress:     !flags.json,
		ACL:              flags.acl,
		SSE:              flags.sse,
		SSEKMS:           flags.sseKMS,
	}
	sourceOps := make([]plannedOperation, 0, len(entries))
	for _, entry := range entries {
		seen[entry.Relative] = struct{}{}
		target, err := resolveTarget(entry, destination, recursive)
		if err != nil {
			return err
		}
		meta, err := fetchTargetMeta(client, target)
		if err != nil {
			return err
		}
		if !needsSync(entry, target, meta) {
			if flags.plan {
				sourceOps = append(sourceOps, plannedOperation{result: operationResult{Action: "sync", Source: entrySourceString(entry), Destination: targetString(target), Status: "unchanged", Size: entry.Size, DryRun: flags.dryRun}})
			}
			continue
		}
		result := operationResult{Action: "sync", Source: entrySourceString(entry), Destination: targetString(target), Status: "synced", Size: entry.Size, DryRun: flags.dryRun}
		if flags.dryRun {
			result.Status = "would-sync"
		}
		entryCopy := entry
		targetCopy := target
		sourceOps = append(sourceOps, plannedOperation{result: result, run: func() error {
			return copyEntry(client, entryCopy, targetCopy, options, rt)
		}})
	}
	results = append(results, executePlannedOperations(sourceOps, options.executionOptions)...)

	if flags.delete {
		targetEntries, err := collectTargetEntries(client, destination, true, matcher)
		if err != nil {
			return err
		}
		deleteOps := make([]plannedOperation, 0, len(targetEntries))
		for _, targetEntry := range targetEntries {
			if _, ok := seen[targetEntry.Relative]; ok {
				continue
			}
			targetRef := entryAsTarget(targetEntry)
			result := operationResult{Action: "delete", Source: targetString(targetRef), Status: "deleted", Size: targetEntry.Size, DryRun: flags.dryRun}
			if flags.dryRun {
				result.Status = "would-delete"
			}
			targetCopy := targetRef
			deleteOps = append(deleteOps, plannedOperation{result: result, run: func() error {
				return deleteTargetRef(client, targetCopy)
			}})
		}
		results = append(results, executePlannedOperations(deleteOps, options.executionOptions)...)
	}

	if !flags.json {
		printOperations(rt.stdout, results)
	}
	if err := operationsPartialFailure("sync", results); err != nil {
		return err
	}
	if flags.json {
		return writeOperationsJSON(rt.stdout, "sync", results)
	}
	return nil
}

func inferSyncRecursion(source location, requested bool) (bool, error) {
	if source.isS3() {
		return requested || source.key == "" || source.hasTrailing, nil
	}
	info, err := os.Stat(source.path)
	if err != nil {
		return false, err
	}
	return requested || info.IsDir(), nil
}

func entryAsTarget(entry sourceEntry) targetRef {
	if entry.Kind == locationKindLocal {
		return targetRef{Kind: locationKindLocal, Local: entry.Local}
	}
	return targetRef{Kind: locationKindS3, Bucket: entry.Bucket, Key: entry.Key}
}
