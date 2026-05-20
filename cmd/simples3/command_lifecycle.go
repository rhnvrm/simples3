package main

import (
	"fmt"

	"github.com/rhnvrm/simples3"
)

type lifecycleResult struct {
	Command       string                          `json:"command"`
	OK            bool                            `json:"ok"`
	Bucket        string                          `json:"bucket"`
	Status        string                          `json:"status"`
	Configuration *lifecycleConfigurationDocument `json:"configuration,omitempty"`
}

func (rt *runtime) runLifecycle(args []string) error {
	if len(args) == 0 {
		return usageErrorf("lifecycle requires a subcommand: get, set, or delete")
	}
	switch args[0] {
	case "get":
		return rt.runLifecycleGet(args[1:])
	case "set":
		return rt.runLifecycleSet(args[1:])
	case "delete", "rm":
		return rt.runLifecycleDelete(args[1:])
	default:
		return usageErrorf("unknown lifecycle subcommand %q", args[0])
	}
}

func (rt *runtime) runLifecycleGet(args []string) error {
	var flags awsFlags
	fs := newFlagSet("lifecycle get", func() {
		fmt.Fprint(rt.stderr, "Usage: simples3 lifecycle get [--profile PROFILE] [--region REGION] [--endpoint URL] [--json] s3://bucket\n")
	})
	addAWSFlags(fs, &flags)
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErrorf("lifecycle get requires exactly one bucket URI")
	}
	loc, err := parseLocation(fs.Arg(0))
	if err != nil {
		return err
	}
	if !loc.isS3() || loc.key != "" {
		return usageErrorf("lifecycle get target must be a bucket URI like s3://my-bucket")
	}
	settings, err := rt.resolveAWSSettings(flags)
	if err != nil {
		return err
	}
	config, err := settings.newClient().GetBucketLifecycle(loc.bucket)
	if err != nil {
		return err
	}
	doc := lifecycleDocumentFromConfiguration(config)
	result := lifecycleResult{Command: "lifecycle", OK: true, Bucket: loc.bucket, Status: "loaded", Configuration: &doc}
	if flags.json {
		return writeJSON(rt.stdout, result)
	}
	return writeJSON(rt.stdout, doc)
}

func (rt *runtime) runLifecycleSet(args []string) error {
	var flags awsFlags
	var file string
	fs := newFlagSet("lifecycle set", func() {
		fmt.Fprint(rt.stderr, "Usage: simples3 lifecycle set [--profile PROFILE] [--region REGION] [--endpoint URL] [--json] --file PATH|- s3://bucket\n")
	})
	addAWSFlags(fs, &flags)
	fs.StringVar(&file, "file", "", "JSON file or - for stdin with lifecycle configuration")
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErrorf("lifecycle set requires exactly one bucket URI")
	}
	if file == "" {
		return usageErrorf("lifecycle set requires --file")
	}
	loc, err := parseLocation(fs.Arg(0))
	if err != nil {
		return err
	}
	if !loc.isS3() || loc.key != "" {
		return usageErrorf("lifecycle set target must be a bucket URI like s3://my-bucket")
	}
	var doc lifecycleConfigurationDocument
	if err := decodeJSONSource(rt, file, &doc); err != nil {
		return err
	}
	if len(doc.Rules) == 0 {
		return usageErrorf("lifecycle set requires at least one rule")
	}
	settings, err := rt.resolveAWSSettings(flags)
	if err != nil {
		return err
	}
	if err := settings.newClient().PutBucketLifecycle(simples3.PutBucketLifecycleInput{Bucket: loc.bucket, Configuration: doc.toConfiguration()}); err != nil {
		return err
	}
	result := lifecycleResult{Command: "lifecycle", OK: true, Bucket: loc.bucket, Status: "updated", Configuration: &doc}
	if flags.json {
		return writeJSON(rt.stdout, result)
	}
	printLine(rt.stdout, "updated lifecycle %s", loc.String())
	return nil
}

func (rt *runtime) runLifecycleDelete(args []string) error {
	var flags awsFlags
	fs := newFlagSet("lifecycle delete", func() {
		fmt.Fprint(rt.stderr, "Usage: simples3 lifecycle delete [--profile PROFILE] [--region REGION] [--endpoint URL] [--json] s3://bucket\n")
	})
	addAWSFlags(fs, &flags)
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErrorf("lifecycle delete requires exactly one bucket URI")
	}
	loc, err := parseLocation(fs.Arg(0))
	if err != nil {
		return err
	}
	if !loc.isS3() || loc.key != "" {
		return usageErrorf("lifecycle delete target must be a bucket URI like s3://my-bucket")
	}
	settings, err := rt.resolveAWSSettings(flags)
	if err != nil {
		return err
	}
	if err := settings.newClient().DeleteBucketLifecycle(simples3.DeleteBucketInput{Bucket: loc.bucket}); err != nil {
		return err
	}
	result := lifecycleResult{Command: "lifecycle", OK: true, Bucket: loc.bucket, Status: "deleted"}
	if flags.json {
		return writeJSON(rt.stdout, result)
	}
	printLine(rt.stdout, "deleted lifecycle %s", loc.String())
	return nil
}
