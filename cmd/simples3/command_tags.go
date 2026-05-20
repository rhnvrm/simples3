package main

import (
	"fmt"

	"github.com/rhnvrm/simples3"
)

type tagsResult struct {
	Command string            `json:"command"`
	OK      bool              `json:"ok"`
	Target  string            `json:"target"`
	Status  string            `json:"status"`
	Tags    map[string]string `json:"tags"`
}

type tagsFlags struct {
	awsFlags
}

func (rt *runtime) runTags(args []string) error {
	if len(args) == 0 {
		return usageErrorf("tags requires a subcommand: get, set, or delete")
	}
	switch args[0] {
	case "get":
		return rt.runTagsGet(args[1:])
	case "set":
		return rt.runTagsSet(args[1:])
	case "delete", "rm":
		return rt.runTagsDelete(args[1:])
	default:
		return usageErrorf("unknown tags subcommand %q", args[0])
	}
}

func (rt *runtime) runTagsGet(args []string) error {
	flags := tagsFlags{}
	fs := newFlagSet("tags get", func() {
		fmt.Fprint(rt.stderr, "Usage: simples3 tags get [--profile PROFILE] [--region REGION] [--endpoint URL] [--json] s3://bucket/key\n")
	})
	addAWSFlags(fs, &flags.awsFlags)
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErrorf("tags get requires exactly one object URI")
	}
	loc, err := parseLocation(fs.Arg(0))
	if err != nil {
		return err
	}
	if !loc.isS3() || loc.key == "" {
		return usageErrorf("tags get target must be an object URI like s3://bucket/key")
	}
	settings, err := rt.resolveAWSSettings(flags.awsFlags)
	if err != nil {
		return err
	}
	output, err := settings.newClient().GetObjectTagging(simples3.GetObjectTaggingInput{Bucket: loc.bucket, ObjectKey: loc.key})
	if err != nil {
		return err
	}
	tags := output.Tags
	if tags == nil {
		tags = map[string]string{}
	}
	result := tagsResult{Command: "tags", OK: true, Target: loc.String(), Status: "loaded", Tags: tags}
	if flags.json {
		return writeJSON(rt.stdout, result)
	}
	writeTagLines(rt.stdout, output.Tags)
	return nil
}

func (rt *runtime) runTagsSet(args []string) error {
	flags := tagsFlags{}
	var tagsFile string
	var tags keyValueFlag
	fs := newFlagSet("tags set", func() {
		fmt.Fprint(rt.stderr, "Usage: simples3 tags set [--profile PROFILE] [--region REGION] [--endpoint URL] [--json] [--tag key=value ...] [--tags-file PATH|-] s3://bucket/key\n")
	})
	addAWSFlags(fs, &flags.awsFlags)
	fs.Var(&tags, "tag", "tag key=value (repeatable)")
	fs.StringVar(&tagsFile, "tags-file", "", "JSON file or - for stdin with tag map")
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErrorf("tags set requires exactly one object URI")
	}
	loc, err := parseLocation(fs.Arg(0))
	if err != nil {
		return err
	}
	if !loc.isS3() || loc.key == "" {
		return usageErrorf("tags set target must be an object URI like s3://bucket/key")
	}
	mergedTags := map[string]string{}
	if tagsFile != "" {
		if err := decodeJSONSource(rt, tagsFile, &mergedTags); err != nil {
			return err
		}
	}
	for key, value := range tags {
		mergedTags[key] = value
	}
	if len(mergedTags) == 0 {
		return usageErrorf("tags set requires at least one --tag or --tags-file input")
	}
	settings, err := rt.resolveAWSSettings(flags.awsFlags)
	if err != nil {
		return err
	}
	if err := settings.newClient().PutObjectTagging(simples3.PutObjectTaggingInput{Bucket: loc.bucket, ObjectKey: loc.key, Tags: mergedTags}); err != nil {
		return err
	}
	result := tagsResult{Command: "tags", OK: true, Target: loc.String(), Status: "updated", Tags: mergedTags}
	if flags.json {
		return writeJSON(rt.stdout, result)
	}
	printLine(rt.stdout, "updated tags %s", loc.String())
	return nil
}

func (rt *runtime) runTagsDelete(args []string) error {
	flags := tagsFlags{}
	fs := newFlagSet("tags delete", func() {
		fmt.Fprint(rt.stderr, "Usage: simples3 tags delete [--profile PROFILE] [--region REGION] [--endpoint URL] [--json] s3://bucket/key\n")
	})
	addAWSFlags(fs, &flags.awsFlags)
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErrorf("tags delete requires exactly one object URI")
	}
	loc, err := parseLocation(fs.Arg(0))
	if err != nil {
		return err
	}
	if !loc.isS3() || loc.key == "" {
		return usageErrorf("tags delete target must be an object URI like s3://bucket/key")
	}
	settings, err := rt.resolveAWSSettings(flags.awsFlags)
	if err != nil {
		return err
	}
	if err := settings.newClient().DeleteObjectTagging(simples3.DeleteObjectTaggingInput{Bucket: loc.bucket, ObjectKey: loc.key}); err != nil {
		return err
	}
	result := tagsResult{Command: "tags", OK: true, Target: loc.String(), Status: "deleted", Tags: map[string]string{}}
	if flags.json {
		return writeJSON(rt.stdout, result)
	}
	printLine(rt.stdout, "deleted tags %s", loc.String())
	return nil
}
