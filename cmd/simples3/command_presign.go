package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/rhnvrm/simples3"
)

type presignResult struct {
	Command string `json:"command"`
	OK      bool   `json:"ok"`
	Target  string `json:"target"`
	Method  string `json:"method"`
	Expires string `json:"expires"`
	URL     string `json:"url"`
}

func (rt *runtime) runPresign(args []string) error {
	var flags awsFlags
	var method string
	var expires string
	var disposition string
	fs := newFlagSet("presign", func() {
		fmt.Fprint(rt.stderr, "Usage: simples3 presign [--profile PROFILE] [--region REGION] [--endpoint URL] [--json] [--method GET|PUT] [--expires 1h] [--response-content-disposition VALUE] s3://bucket/key\n")
	})
	addAWSFlags(fs, &flags)
	fs.StringVar(&method, "method", "GET", "HTTP method for the presigned URL (GET or PUT)")
	fs.StringVar(&expires, "expires", "1h", "URL expiry duration (for example 15m, 1h)")
	fs.StringVar(&disposition, "response-content-disposition", "", "optional response-content-disposition for GET URLs")
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	if len(fs.Args()) != 1 {
		fs.Usage()
		return usageErrorf("presign requires exactly one object URI")
	}
	loc, err := parseLocation(fs.Args()[0])
	if err != nil {
		return err
	}
	if !loc.isS3() || loc.key == "" {
		return usageErrorf("presign target must be an object URI like s3://bucket/key")
	}
	settings, err := rt.resolveAWSSettings(flags)
	if err != nil {
		return err
	}
	method = strings.ToUpper(method)
	if method != "GET" && method != "PUT" {
		return usageErrorf("unsupported presign method %q", method)
	}
	duration, err := time.ParseDuration(expires)
	if err != nil {
		return usageErrorf("invalid --expires value %q: %v", expires, err)
	}
	seconds := int(duration.Seconds())
	if seconds <= 0 {
		return usageErrorf("expires must be greater than zero")
	}
	url := settings.newClient().GeneratePresignedURL(simples3.PresignedInput{Bucket: loc.bucket, ObjectKey: loc.key, Method: method, ExpirySeconds: seconds, ResponseContentDisposition: disposition})
	if url == "" {
		return fmt.Errorf("failed to generate presigned URL")
	}
	result := presignResult{Command: "presign", OK: true, Target: loc.String(), Method: method, Expires: duration.String(), URL: url}
	if flags.json {
		return writeJSON(rt.stdout, result)
	}
	printLine(rt.stdout, "%s", url)
	return nil
}
