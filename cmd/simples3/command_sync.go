package main

import (
	"fmt"
	"os"
)

type syncFlags struct {
	awsFlags
	recursive bool
	dryRun    bool
	delete    bool
	includes  stringListFlag
	excludes  stringListFlag
	acl       string
	sse       string
	sseKMS    string
}

func (rt *runtime) runSync(args []string) error {
	flags := syncFlags{}
	fs := newFlagSet("sync", func() {
		fmt.Fprint(rt.stderr, `Usage: simples3 sync [flags] <source> <destination>

Synchronize source to destination. This is a one-way copy from source to destination.

Flags:
  --recursive           recurse into directories or S3 prefixes
  --dry-run             print planned operations without executing them
  --delete              delete destination files missing from source
  --include <pattern>   include glob pattern (repeatable)
  --exclude <pattern>   exclude glob pattern (repeatable)
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
	fs.BoolVar(&flags.delete, "delete", false, "delete destination entries missing from source")
	fs.Var(&flags.includes, "include", "include glob pattern")
	fs.Var(&flags.excludes, "exclude", "exclude glob pattern")
	fs.StringVar(&flags.acl, "acl", "", "canned ACL for uploads")
	fs.StringVar(&flags.sse, "sse", "", "server-side encryption mode")
	fs.StringVar(&flags.sseKMS, "sse-kms-key-id", "", "KMS key ID")
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return fmt.Errorf("sync requires a source and destination")
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
		return fmt.Errorf("local-to-local sync is not supported")
	}
	if destination.isLocal() && (flags.acl != "" || flags.sse != "" || flags.sseKMS != "") {
		return fmt.Errorf("--acl/--sse flags require an S3 destination")
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
		return fmt.Errorf("--delete is only supported for recursive syncs")
	}

	entries, err := collectSourceEntries(client, source, recursive, matcher, "")
	if err != nil {
		return err
	}

	results := []operationResult{}
	seen := map[string]struct{}{}
	options := copyOptions{DryRun: flags.dryRun, ACL: flags.acl, SSE: flags.sse, SSEKMS: flags.sseKMS}
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
			continue
		}
		status := "synced"
		if flags.dryRun {
			status = "would-sync"
		} else if err := copyEntry(client, entry, target, options, rt); err != nil {
			return err
		}
		result := operationResult{Action: "sync", Source: entrySourceString(entry), Destination: targetString(target), Status: status, Size: entry.Size, DryRun: flags.dryRun}
		results = append(results, result)
		if !flags.json {
			printOperation(rt.stdout, result)
		}
	}

	if flags.delete {
		targetEntries, err := collectTargetEntries(client, destination, true, matcher)
		if err != nil {
			return err
		}
		for _, targetEntry := range targetEntries {
			if _, ok := seen[targetEntry.Relative]; ok {
				continue
			}
			targetRef := entryAsTarget(targetEntry)
			status := "deleted"
			if flags.dryRun {
				status = "would-delete"
			} else if err := deleteTargetRef(client, targetRef); err != nil {
				return err
			}
			result := operationResult{Action: "delete", Source: targetString(targetRef), Status: status, Size: targetEntry.Size, DryRun: flags.dryRun}
			results = append(results, result)
			if !flags.json {
				printOperation(rt.stdout, result)
			}
		}
	}

	if flags.json {
		return writeJSON(rt.stdout, operationsOutput{Operations: results})
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
