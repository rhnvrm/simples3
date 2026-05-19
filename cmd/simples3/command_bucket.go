package main

import (
	"fmt"

	"github.com/rhnvrm/simples3"
)

type bucketResult struct {
	Bucket   string `json:"bucket"`
	Location string `json:"location,omitempty"`
	Status   string `json:"status"`
}

func (rt *runtime) runMakeBucket(args []string) error {
	var flags awsFlags
	fs := newFlagSet("mb", func() {
		fmt.Fprint(rt.stderr, "Usage: simples3 mb [--profile PROFILE] [--region REGION] [--endpoint URL] [--json] s3://bucket\n")
	})
	addAWSFlags(fs, &flags)
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	if len(fs.Args()) != 1 {
		fs.Usage()
		return fmt.Errorf("mb requires exactly one bucket URI")
	}
	loc, err := parseLocation(fs.Args()[0])
	if err != nil {
		return err
	}
	if !loc.isS3() || loc.bucket == "" || loc.key != "" {
		return fmt.Errorf("mb target must be a bucket URI like s3://my-bucket")
	}
	settings, err := rt.resolveAWSSettings(flags)
	if err != nil {
		return err
	}
	output, err := settings.newClient().CreateBucket(simples3.CreateBucketInput{Bucket: loc.bucket, Region: settings.Region})
	if err != nil {
		return err
	}
	result := bucketResult{Bucket: loc.bucket, Location: output.Location, Status: "created"}
	if flags.json {
		return writeJSON(rt.stdout, result)
	}
	printLine(rt.stdout, "created %s", loc.String())
	return nil
}

func (rt *runtime) runRemoveBucket(args []string) error {
	var flags awsFlags
	fs := newFlagSet("rb", func() {
		fmt.Fprint(rt.stderr, "Usage: simples3 rb [--profile PROFILE] [--region REGION] [--endpoint URL] [--json] s3://bucket\n")
	})
	addAWSFlags(fs, &flags)
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	if len(fs.Args()) != 1 {
		fs.Usage()
		return fmt.Errorf("rb requires exactly one bucket URI")
	}
	loc, err := parseLocation(fs.Args()[0])
	if err != nil {
		return err
	}
	if !loc.isS3() || loc.bucket == "" || loc.key != "" {
		return fmt.Errorf("rb target must be a bucket URI like s3://my-bucket")
	}
	settings, err := rt.resolveAWSSettings(flags)
	if err != nil {
		return err
	}
	if err := settings.newClient().DeleteBucket(simples3.DeleteBucketInput{Bucket: loc.bucket}); err != nil {
		return err
	}
	result := bucketResult{Bucket: loc.bucket, Status: "deleted"}
	if flags.json {
		return writeJSON(rt.stdout, result)
	}
	printLine(rt.stdout, "deleted %s", loc.String())
	return nil
}
