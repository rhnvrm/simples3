package main

import (
	"fmt"

	"github.com/rhnvrm/simples3"
)

type removeFlags struct {
	awsFlags
	recursive bool
	dryRun    bool
	includes  stringListFlag
	excludes  stringListFlag
	versionID string
}

func (rt *runtime) runRemove(args []string) error {
	flags := removeFlags{}
	fs := newFlagSet("rm", func() {
		fmt.Fprint(rt.stderr, `Usage: simples3 rm [flags] <s3://bucket/key>

Remove an S3 object or prefix.

Flags:
  --recursive           remove prefixes recursively
  --dry-run             print planned deletions without executing them
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

	settings, err := rt.resolveAWSSettings(flags.awsFlags)
	if err != nil {
		return err
	}
	matcher, err := newMatcher([]string(flags.includes), []string(flags.excludes))
	if err != nil {
		return err
	}
	client := settings.newClient()

	results := []operationResult{}
	if flags.recursive {
		entries, err := collectSourceEntries(client, target, true, matcher, "")
		if err != nil {
			return err
		}
		results = make([]operationResult, 0, len(entries))
		for _, entry := range entries {
			result := operationResult{Action: "remove", Source: entrySourceString(entry), Status: "deleted", Size: entry.Size}
			if flags.dryRun {
				result.Status = "would-delete"
				result.DryRun = true
				results = append(results, result)
				if !flags.json {
					printOperation(rt.stdout, result)
				}
				continue
			}
			results = append(results, result)
		}
		if !flags.dryRun {
			for _, chunk := range chunkKeys(entries, 1000) {
				keys := make([]string, 0, len(chunk))
				for _, entry := range chunk {
					keys = append(keys, entry.Key)
				}
				output, err := client.DeleteObjects(simples3.DeleteObjectsInput{Bucket: target.bucket, Objects: keys, Quiet: true})
				if err != nil {
					return err
				}
				if len(output.Errors) > 0 {
					return fmt.Errorf("delete failed for %s: %s", output.Errors[0].Key, output.Errors[0].Message)
				}
			}
			if !flags.json {
				for _, result := range results {
					printOperation(rt.stdout, result)
				}
			}
		}
	} else {
		entry := sourceEntry{Kind: locationKindS3, Bucket: target.bucket, Key: target.key, Version: flags.versionID, Relative: target.key}
		result := operationResult{Action: "remove", Source: entrySourceString(entry), Status: "deleted", DryRun: flags.dryRun}
		if flags.dryRun {
			result.Status = "would-delete"
		} else if err := client.FileDelete(simples3.DeleteInput{Bucket: target.bucket, ObjectKey: target.key, VersionId: flags.versionID}); err != nil {
			return err
		}
		results = append(results, result)
		if !flags.json {
			printOperation(rt.stdout, result)
		}
	}

	if flags.json {
		return writeOperationsJSON(rt.stdout, "rm", results)
	}
	return nil
}
