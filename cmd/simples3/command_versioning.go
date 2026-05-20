package main

import (
	"fmt"
	"strings"

	"github.com/rhnvrm/simples3"
)

type versioningResult struct {
	Command   string `json:"command"`
	OK        bool   `json:"ok"`
	Bucket    string `json:"bucket"`
	Status    string `json:"status"`
	MFADelete string `json:"mfaDelete,omitempty"`
}

func (rt *runtime) runVersioning(args []string) error {
	if len(args) == 0 {
		return usageErrorf("versioning requires a subcommand: get or set")
	}
	switch args[0] {
	case "get":
		return rt.runVersioningGet(args[1:])
	case "set":
		return rt.runVersioningSet(args[1:])
	default:
		return usageErrorf("unknown versioning subcommand %q", args[0])
	}
}

func (rt *runtime) runVersioningGet(args []string) error {
	var flags awsFlags
	fs := newFlagSet("versioning get", func() {
		fmt.Fprint(rt.stderr, "Usage: simples3 versioning get [--profile PROFILE] [--region REGION] [--endpoint URL] [--json] s3://bucket\n")
	})
	addAWSFlags(fs, &flags)
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErrorf("versioning get requires exactly one bucket URI")
	}
	loc, err := parseLocation(fs.Arg(0))
	if err != nil {
		return err
	}
	if !loc.isS3() || loc.key != "" {
		return usageErrorf("versioning get target must be a bucket URI like s3://my-bucket")
	}
	settings, err := rt.resolveAWSSettings(flags)
	if err != nil {
		return err
	}
	output, err := settings.newClient().GetBucketVersioning(loc.bucket)
	if err != nil {
		return err
	}
	result := versioningResult{Command: "versioning", OK: true, Bucket: loc.bucket, Status: firstNonEmpty(output.Status, "Unversioned"), MFADelete: output.MfaDelete}
	if flags.json {
		return writeJSON(rt.stdout, result)
	}
	printLine(rt.stdout, "status: %s", result.Status)
	if result.MFADelete != "" {
		printLine(rt.stdout, "mfa-delete: %s", result.MFADelete)
	}
	return nil
}

func (rt *runtime) runVersioningSet(args []string) error {
	var flags awsFlags
	var status string
	var mfaDelete string
	fs := newFlagSet("versioning set", func() {
		fmt.Fprint(rt.stderr, "Usage: simples3 versioning set [--profile PROFILE] [--region REGION] [--endpoint URL] [--json] --status enabled|suspended [--mfa-delete enabled|disabled] s3://bucket\n")
	})
	addAWSFlags(fs, &flags)
	fs.StringVar(&status, "status", "", "versioning status: enabled or suspended")
	fs.StringVar(&mfaDelete, "mfa-delete", "", "MFA delete status: enabled or disabled")
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErrorf("versioning set requires exactly one bucket URI")
	}
	loc, err := parseLocation(fs.Arg(0))
	if err != nil {
		return err
	}
	if !loc.isS3() || loc.key != "" {
		return usageErrorf("versioning set target must be a bucket URI like s3://my-bucket")
	}
	status = normalizeVersioningStatus(status)
	if status == "" {
		return usageErrorf("versioning set requires --status enabled|suspended")
	}
	if status != "Enabled" && status != "Suspended" {
		return usageErrorf("invalid --status value %q", status)
	}
	mfaDelete = normalizeVersioningStatus(mfaDelete)
	if mfaDelete != "" && mfaDelete != "Enabled" && mfaDelete != "Disabled" {
		return usageErrorf("invalid --mfa-delete value %q", mfaDelete)
	}
	settings, err := rt.resolveAWSSettings(flags)
	if err != nil {
		return err
	}
	if err := settings.newClient().PutBucketVersioning(simples3.PutBucketVersioningInput{Bucket: loc.bucket, Status: status, MfaDelete: mfaDelete}); err != nil {
		return err
	}
	result := versioningResult{Command: "versioning", OK: true, Bucket: loc.bucket, Status: status, MFADelete: mfaDelete}
	if flags.json {
		return writeJSON(rt.stdout, result)
	}
	printLine(rt.stdout, "updated versioning %s to %s", loc.String(), status)
	return nil
}

func normalizeVersioningStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "enabled":
		return "Enabled"
	case "disabled":
		return "Disabled"
	case "suspended":
		return "Suspended"
	case "":
		return ""
	default:
		return value
	}
}
