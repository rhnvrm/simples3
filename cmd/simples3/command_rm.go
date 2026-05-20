package main

import "fmt"

type removeFlags struct {
	awsFlags
	recursive       bool
	dryRun          bool
	continueOnError bool
	concurrency     int
	retries         int
	includes        stringListFlag
	excludes        stringListFlag
	versionID       string
}

func (rt *runtime) runRemove(args []string) error {
	flags := removeFlags{}
	fs := newFlagSet("rm", func() {
		fmt.Fprint(rt.stderr, `Usage: simples3 rm [flags] <s3://bucket/key>

Remove an S3 object or prefix.

Flags:
  --recursive           remove prefixes recursively
  --dry-run             print planned deletions without executing them
  --continue-on-error   keep processing remaining deletions after failures
  --concurrency <n>     number of deletions to run in parallel (default 1)
  --retries <n>         retry transient failures this many times (default 2)
  --include <pattern>   include glob pattern for recursive deletes (repeatable)
  --exclude <pattern>   exclude glob pattern for recursive deletes (repeatable)
  --version-id <id>     delete a specific object version
  --profile <name>      AWS profile name
  --region <name>       AWS region
  --endpoint <url>      custom S3 endpoint URL
  --json                emit JSON output
`)
	})
	addAWSFlags(fs, &flags.awsFlags)
	fs.BoolVar(&flags.recursive, "recursive", false, "remove recursively")
	fs.BoolVar(&flags.dryRun, "dry-run", false, "print deletions without executing")
	fs.BoolVar(&flags.continueOnError, "continue-on-error", false, "continue processing after failures")
	fs.IntVar(&flags.concurrency, "concurrency", 1, "number of parallel deletions")
	fs.IntVar(&flags.retries, "retries", 2, "number of retries for transient failures")
	fs.Var(&flags.includes, "include", "include glob pattern")
	fs.Var(&flags.excludes, "exclude", "exclude glob pattern")
	fs.StringVar(&flags.versionID, "version-id", "", "object version ID")
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErrorf("rm requires exactly one S3 path")
	}

	target, err := parseLocation(fs.Arg(0))
	if err != nil {
		return err
	}
	if target.isLocal() {
		return usageErrorf("rm only supports s3:// locations")
	}
	if !flags.recursive && target.key == "" {
		return usageErrorf("refusing to remove bucket root; use rb for buckets or --recursive for object prefixes")
	}
	if flags.versionID != "" && flags.recursive {
		return usageErrorf("--version-id is only supported for single-object deletes")
	}
	if !flags.recursive && target.hasTrailing {
		return usageErrorf("%s looks like a prefix; use --recursive", target.String())
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

	execution := executionOptions{DryRun: flags.dryRun, Concurrency: flags.concurrency, Retries: flags.retries, ContinueOnError: flags.continueOnError}
	results := []operationResult{}
	if flags.recursive {
		entries, err := collectSourceEntries(client, target, true, matcher, "")
		if err != nil {
			return err
		}
		results = executeDeleteBatches(client, entries, target.bucket, execution)
	} else {
		entry := sourceEntry{Kind: locationKindS3, Bucket: target.bucket, Key: target.key, Version: flags.versionID, Relative: target.key}
		ops := []plannedOperation{{
			result: buildDeleteResult(entry, flags.dryRun),
			run: func() error {
				return deleteSourceEntry(client, entry, target)
			},
		}}
		results = executePlannedOperations(ops, execution)
	}

	if !flags.json {
		printOperations(rt.stdout, results)
	}
	if err := operationsPartialFailure("rm", results); err != nil {
		return err
	}
	if flags.json {
		return writeOperationsJSON(rt.stdout, "rm", results)
	}
	return nil
}
