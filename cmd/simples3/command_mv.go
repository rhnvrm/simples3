package main

import "fmt"

type moveFlags struct {
	copyFlags
}

func (rt *runtime) runMove(args []string) error {
	flags := moveFlags{}
	fs := newFlagSet("mv", func() {
		fmt.Fprint(rt.stderr, `Usage: simples3 mv [flags] <source> <destination>

Move a local file or S3 object between local paths and s3:// locations.

Flags:
  --recursive           move directories or S3 prefixes recursively
  --dry-run             print planned operations without executing them
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
	fs.BoolVar(&flags.recursive, "recursive", false, "move recursively")
	fs.BoolVar(&flags.dryRun, "dry-run", false, "print operations without executing")
	fs.Var(&flags.includes, "include", "include glob pattern")
	fs.Var(&flags.excludes, "exclude", "exclude glob pattern")
	fs.StringVar(&flags.acl, "acl", "", "canned ACL for uploads")
	fs.StringVar(&flags.sse, "sse", "", "server-side encryption mode")
	fs.StringVar(&flags.sseKMS, "sse-kms-key-id", "", "KMS key ID")
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return fmt.Errorf("mv requires a source and destination")
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
		return fmt.Errorf("local-to-local moves are not supported")
	}
	if source.String() == destination.String() {
		return fmt.Errorf("source and destination are the same")
	}
	if destination.isLocal() && (flags.acl != "" || flags.sse != "" || flags.sseKMS != "") {
		return fmt.Errorf("--acl/--sse flags require an S3 destination")
	}
	if err := validateRecursiveSource(source, flags.recursive); err != nil {
		return err
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
	entries, err := collectSourceEntries(client, source, flags.recursive, matcher, "")
	if err != nil {
		return err
	}
	if err := ensureSingleSourceEntry(source, flags.recursive, entries); err != nil {
		return err
	}

	results := make([]operationResult, 0, len(entries))
	options := copyOptions{DryRun: flags.dryRun, ACL: flags.acl, SSE: flags.sse, SSEKMS: flags.sseKMS}
	for _, entry := range entries {
		target, err := resolveTarget(entry, destination, flags.recursive)
		if err != nil {
			return err
		}
		status := "moved"
		if flags.dryRun {
			status = "would-move"
		} else {
			if err := copyEntry(client, entry, target, options, rt); err != nil {
				return err
			}
			if err := deleteSourceEntry(client, entry, source); err != nil {
				return err
			}
		}
		result := operationResult{Action: "move", Source: entrySourceString(entry), Destination: targetString(target), Status: status, Size: entry.Size, DryRun: flags.dryRun}
		results = append(results, result)
		if !flags.json {
			printOperation(rt.stdout, result)
		}
	}
	if flags.json {
		return writeJSON(rt.stdout, operationsOutput{Operations: results})
	}
	return nil
}
