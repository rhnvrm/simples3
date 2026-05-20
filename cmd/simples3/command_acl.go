package main

import (
	"fmt"

	"github.com/rhnvrm/simples3"
)

type aclResult struct {
	Command   string                       `json:"command"`
	OK        bool                         `json:"ok"`
	Target    string                       `json:"target"`
	Status    string                       `json:"status"`
	CannedACL string                       `json:"cannedAcl,omitempty"`
	Policy    *accessControlPolicyDocument `json:"policy,omitempty"`
}

func (rt *runtime) runACL(args []string) error {
	if len(args) == 0 {
		return usageErrorf("acl requires a subcommand: get or set")
	}
	switch args[0] {
	case "get":
		return rt.runACLGet(args[1:])
	case "set":
		return rt.runACLSet(args[1:])
	default:
		return usageErrorf("unknown acl subcommand %q", args[0])
	}
}

func (rt *runtime) runACLGet(args []string) error {
	var flags awsFlags
	var versionID string
	fs := newFlagSet("acl get", func() {
		fmt.Fprint(rt.stderr, "Usage: simples3 acl get [--profile PROFILE] [--region REGION] [--endpoint URL] [--json] [--version-id ID] s3://bucket[/key]\n")
	})
	addAWSFlags(fs, &flags)
	fs.StringVar(&versionID, "version-id", "", "object version ID")
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErrorf("acl get requires exactly one bucket or object URI")
	}
	loc, err := parseLocation(fs.Arg(0))
	if err != nil {
		return err
	}
	if !loc.isS3() {
		return usageErrorf("acl get target must be an S3 URI like s3://bucket or s3://bucket/key")
	}
	if versionID != "" && loc.key == "" {
		return usageErrorf("--version-id requires an object target")
	}
	settings, err := rt.resolveAWSSettings(flags)
	if err != nil {
		return err
	}
	client := settings.newClient()
	var policy simples3.AccessControlPolicy
	if loc.key == "" {
		policy, err = client.GetBucketAcl(loc.bucket)
	} else {
		policy, err = client.GetObjectAcl(simples3.GetObjectAclInput{Bucket: loc.bucket, ObjectKey: loc.key, VersionId: versionID})
	}
	if err != nil {
		return err
	}
	doc := aclDocumentFromPolicy(policy)
	result := aclResult{Command: "acl", OK: true, Target: loc.String(), Status: "loaded", Policy: &doc}
	if flags.json {
		return writeJSON(rt.stdout, result)
	}
	return writeJSON(rt.stdout, doc)
}

func (rt *runtime) runACLSet(args []string) error {
	var flags awsFlags
	var cannedACL string
	var policyFile string
	var versionID string
	fs := newFlagSet("acl set", func() {
		fmt.Fprint(rt.stderr, "Usage: simples3 acl set [--profile PROFILE] [--region REGION] [--endpoint URL] [--json] [--version-id ID] (--acl VALUE | --policy-file PATH|-) s3://bucket[/key]\n")
	})
	addAWSFlags(fs, &flags)
	fs.StringVar(&cannedACL, "acl", "", "canned ACL value")
	fs.StringVar(&policyFile, "policy-file", "", "JSON file or - for stdin with ACL policy")
	fs.StringVar(&versionID, "version-id", "", "object version ID")
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErrorf("acl set requires exactly one bucket or object URI")
	}
	loc, err := parseLocation(fs.Arg(0))
	if err != nil {
		return err
	}
	if !loc.isS3() {
		return usageErrorf("acl set target must be an S3 URI like s3://bucket or s3://bucket/key")
	}
	if versionID != "" && loc.key == "" {
		return usageErrorf("--version-id requires an object target")
	}
	if (cannedACL == "") == (policyFile == "") {
		return usageErrorf("acl set requires exactly one of --acl or --policy-file")
	}
	settings, err := rt.resolveAWSSettings(flags)
	if err != nil {
		return err
	}
	client := settings.newClient()
	result := aclResult{Command: "acl", OK: true, Target: loc.String(), Status: "updated", CannedACL: cannedACL}
	if policyFile != "" {
		var doc accessControlPolicyDocument
		if err := decodeJSONSource(rt, policyFile, &doc); err != nil {
			return err
		}
		result.Policy = &doc
		if loc.key == "" {
			err = client.PutBucketAcl(simples3.PutBucketAclInput{Bucket: loc.bucket, AccessControlPolicy: doc.toPolicy()})
		} else {
			err = client.PutObjectAcl(simples3.PutObjectAclInput{Bucket: loc.bucket, ObjectKey: loc.key, VersionId: versionID, AccessControlPolicy: doc.toPolicy()})
		}
		if err != nil {
			return err
		}
		if flags.json {
			return writeJSON(rt.stdout, result)
		}
		printLine(rt.stdout, "updated acl %s", loc.String())
		return nil
	}
	if loc.key == "" {
		err = client.PutBucketAcl(simples3.PutBucketAclInput{Bucket: loc.bucket, CannedACL: cannedACL})
	} else {
		err = client.PutObjectAcl(simples3.PutObjectAclInput{Bucket: loc.bucket, ObjectKey: loc.key, VersionId: versionID, CannedACL: cannedACL})
	}
	if err != nil {
		return err
	}
	if flags.json {
		return writeJSON(rt.stdout, result)
	}
	printLine(rt.stdout, "updated acl %s", loc.String())
	return nil
}
