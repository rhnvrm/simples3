package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/rhnvrm/simples3"
)

type listSummary struct {
	BucketCount       int `json:"bucketCount,omitempty"`
	ObjectCount       int `json:"objectCount,omitempty"`
	CommonPrefixCount int `json:"commonPrefixCount,omitempty"`
}

type listJSONOutput struct {
	Command string            `json:"command"`
	OK      bool              `json:"ok"`
	Target  string            `json:"target,omitempty"`
	Summary listSummary       `json:"summary"`
	Buckets []simples3.Bucket `json:"buckets,omitempty"`
	Prefix  string            `json:"prefix,omitempty"`
	Objects []objectEntry     `json:"objects,omitempty"`
	Common  []string          `json:"commonPrefixes,omitempty"`
}

type objectEntry struct {
	Key          string `json:"key"`
	Size         int64  `json:"size"`
	LastModified string `json:"lastModified"`
	StorageClass string `json:"storageClass,omitempty"`
}

func (rt *runtime) runList(args []string) error {
	var flags awsFlags
	var recursive bool
	fs := newFlagSet("ls", func() {
		fmt.Fprint(rt.stderr, "Usage: simples3 ls [--profile PROFILE] [--region REGION] [--endpoint URL] [--recursive] [--json] [s3://bucket[/prefix]]\n")
	})
	addAWSFlags(fs, &flags)
	fs.BoolVar(&recursive, "recursive", false, "list recursively")
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	remaining := fs.Args()
	if len(remaining) > 1 {
		fs.Usage()
		return usageErrorf("ls accepts at most one target")
	}

	settings, err := rt.resolveAWSSettings(flags)
	if err != nil {
		return err
	}
	client := settings.newClient()

	if len(remaining) == 0 {
		result, err := client.ListBuckets(simples3.ListBucketsInput{})
		if err != nil {
			return err
		}
		if flags.json {
			return writeJSON(rt.stdout, listJSONOutput{Command: "ls", OK: true, Summary: listSummary{BucketCount: len(result.Buckets)}, Buckets: result.Buckets})
		}
		for _, bucket := range result.Buckets {
			printLine(rt.stdout, "%s\t%s", bucket.CreationDate.Format(time.RFC3339), bucket.Name)
		}
		return nil
	}

	loc, err := parseLocation(remaining[0])
	if err != nil {
		return err
	}
	if !loc.isS3() {
		return usageErrorf("ls target must be an s3:// URI")
	}

	if recursive {
		seq, finish := client.ListAll(simples3.ListInput{Bucket: loc.bucket, Prefix: loc.s3Prefix()})
		output := listJSONOutput{Command: "ls", OK: true, Target: loc.String(), Prefix: loc.s3Prefix()}
		for object := range seq {
			entry := objectEntry{Key: object.Key, Size: object.Size, LastModified: object.LastModified, StorageClass: object.StorageClass}
			if flags.json {
				output.Objects = append(output.Objects, entry)
			} else {
				printLine(rt.stdout, "%12d  %s", entry.Size, entry.Key)
			}
		}
		if err := finish(); err != nil {
			return err
		}
		if flags.json {
			output.Summary = listSummary{ObjectCount: len(output.Objects)}
			return writeJSON(rt.stdout, output)
		}
		return nil
	}

	result, err := client.List(simples3.ListInput{Bucket: loc.bucket, Prefix: loc.s3Prefix(), Delimiter: "/"})
	if err != nil {
		return err
	}
	output := listJSONOutput{Command: "ls", OK: true, Target: loc.String(), Prefix: loc.s3Prefix(), Common: result.CommonPrefixes}
	for _, object := range result.Objects {
		output.Objects = append(output.Objects, objectEntry{Key: object.Key, Size: object.Size, LastModified: object.LastModified, StorageClass: object.StorageClass})
	}
	if flags.json {
		output.Summary = listSummary{ObjectCount: len(output.Objects), CommonPrefixCount: len(output.Common)}
		return writeJSON(rt.stdout, output)
	}
	for _, prefix := range output.Common {
		printLine(rt.stdout, "DIR\t%s", strings.TrimSuffix(prefix, "/")+"/")
	}
	for _, object := range output.Objects {
		printLine(rt.stdout, "%12d  %s", object.Size, object.Key)
	}
	return nil
}
