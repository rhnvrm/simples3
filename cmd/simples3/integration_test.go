package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rhnvrm/simples3"
)

const cliIntegrationEnv = "SIMPLES3_CLI_INTEGRATION"

type cliIntegrationHarness struct {
	t        *testing.T
	env      map[string]string
	client   *simples3.S3
	bucket   string
	homeDir  string
	endpoint string
}

func newCLIIntegrationHarness(t *testing.T) *cliIntegrationHarness {
	t.Helper()

	if os.Getenv(cliIntegrationEnv) != "1" {
		t.Skipf("set %s=1 to run MinIO-backed CLI integration tests", cliIntegrationEnv)
	}

	endpoint := os.Getenv("AWS_S3_ENDPOINT")
	accessKey := firstNonEmpty(os.Getenv("AWS_ACCESS_KEY_ID"), os.Getenv("AWS_S3_ACCESS_KEY"))
	secretKey := firstNonEmpty(os.Getenv("AWS_SECRET_ACCESS_KEY"), os.Getenv("AWS_S3_SECRET_KEY"))
	region := firstNonEmpty(os.Getenv("AWS_REGION"), os.Getenv("AWS_S3_REGION"), "us-east-1")
	if endpoint == "" || accessKey == "" || secretKey == "" {
		t.Skip("CLI integration tests require AWS_S3_ENDPOINT and AWS credentials for MinIO")
	}

	client := simples3.New(region, accessKey, secretKey)
	client.SetEndpoint(endpoint)

	bucket := fmt.Sprintf("simples3-cli-%d", time.Now().UnixNano())
	if _, err := client.CreateBucket(simples3.CreateBucketInput{Bucket: bucket, Region: region}); err != nil {
		t.Fatalf("create integration bucket %s: %v", bucket, err)
	}

	homeDir := t.TempDir()
	h := &cliIntegrationHarness{
		t:        t,
		client:   client,
		bucket:   bucket,
		homeDir:  homeDir,
		endpoint: endpoint,
		env: map[string]string{
			"AWS_ACCESS_KEY_ID":         accessKey,
			"AWS_SECRET_ACCESS_KEY":     secretKey,
			"AWS_REGION":                region,
			"AWS_DEFAULT_REGION":        region,
			"AWS_S3_ENDPOINT":           endpoint,
			"AWS_EC2_METADATA_DISABLED": "true",
		},
	}

	t.Cleanup(func() {
		h.cleanupBucket()
	})

	return h
}

func (h *cliIntegrationHarness) cleanupBucket() {
	h.t.Helper()
	for attempt := 0; attempt < 5; attempt++ {
		removed := false
		versions, err := h.client.ListVersions(simples3.ListVersionsInput{Bucket: h.bucket})
		if err == nil {
			for _, version := range versions.Versions {
				removed = true
				if err := h.client.FileDelete(simples3.DeleteInput{Bucket: h.bucket, ObjectKey: version.Key, VersionId: version.VersionId}); err != nil && !strings.Contains(strings.ToLower(err.Error()), "not found") {
					h.t.Fatalf("delete cleanup object version %s from %s: %v", version.Key, h.bucket, err)
				}
			}
			for _, marker := range versions.DeleteMarkers {
				removed = true
				if err := h.client.FileDelete(simples3.DeleteInput{Bucket: h.bucket, ObjectKey: marker.Key, VersionId: marker.VersionId}); err != nil && !strings.Contains(strings.ToLower(err.Error()), "not found") {
					h.t.Fatalf("delete cleanup delete-marker %s from %s: %v", marker.Key, h.bucket, err)
				}
			}
		}

		seq, finish := h.client.ListAll(simples3.ListInput{Bucket: h.bucket})
		keys := make([]string, 0)
		for object := range seq {
			keys = append(keys, object.Key)
		}
		if err := finish(); err != nil && !strings.Contains(strings.ToLower(err.Error()), "not found") {
			h.t.Fatalf("list cleanup bucket %s: %v", h.bucket, err)
		}
		if len(keys) == 0 && !removed {
			break
		}
		for start := 0; start < len(keys); start += 1000 {
			removed = true
			end := start + 1000
			if end > len(keys) {
				end = len(keys)
			}
			if _, err := h.client.DeleteObjects(simples3.DeleteObjectsInput{Bucket: h.bucket, Objects: keys[start:end], Quiet: true}); err != nil {
				h.t.Fatalf("delete cleanup objects from %s: %v", h.bucket, err)
			}
		}
		if !removed {
			break
		}
	}
	if err := h.client.DeleteBucket(simples3.DeleteBucketInput{Bucket: h.bucket}); err != nil && !strings.Contains(strings.ToLower(err.Error()), "not found") {
		h.t.Fatalf("delete cleanup bucket %s: %v", h.bucket, err)
	}
}

func (h *cliIntegrationHarness) run(args ...string) (int, string, string) {
	return h.runWithInput("", args...)
}

func (h *cliIntegrationHarness) runWithInput(input string, args ...string) (int, string, string) {
	h.t.Helper()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	rt := &runtime{
		stdin:  strings.NewReader(input),
		stdout: stdout,
		stderr: stderr,
		getenv: func(key string) string { return h.env[key] },
		homeDir: func() (string, error) {
			return h.homeDir, nil
		},
	}
	code := rt.run(args)
	return code, stdout.String(), stderr.String()
}

func (h *cliIntegrationHarness) mustRun(args ...string) string {
	h.t.Helper()
	code, stdout, stderr := h.run(args...)
	if code != 0 {
		h.t.Fatalf("command failed (%v): stdout=%s stderr=%s", args, stdout, stderr)
	}
	return stdout
}

func (h *cliIntegrationHarness) mustRunWithInput(input string, args ...string) string {
	h.t.Helper()
	code, stdout, stderr := h.runWithInput(input, args...)
	if code != 0 {
		h.t.Fatalf("command failed (%v): stdout=%s stderr=%s", args, stdout, stderr)
	}
	return stdout
}

func (h *cliIntegrationHarness) putObject(key, content string) {
	h.t.Helper()
	_, err := h.client.FilePut(simples3.UploadInput{
		Bucket:      h.bucket,
		ObjectKey:   key,
		ContentType: "text/plain",
		Body:        strings.NewReader(content),
	})
	if err != nil {
		h.t.Fatalf("put object %s: %v", key, err)
	}
}

func (h *cliIntegrationHarness) readObject(key string) string {
	h.t.Helper()
	body, err := h.client.FileDownload(simples3.DownloadInput{Bucket: h.bucket, ObjectKey: key})
	if err != nil {
		h.t.Fatalf("download object %s: %v", key, err)
	}
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil {
		h.t.Fatalf("read object %s: %v", key, err)
	}
	return string(data)
}

func (h *cliIntegrationHarness) objectExists(key string) bool {
	h.t.Helper()
	_, err := h.client.FileDetails(simples3.DetailsInput{Bucket: h.bucket, ObjectKey: key})
	return err == nil
}

func TestCLIIntegrationBucketLifecycle(t *testing.T) {
	h := newCLIIntegrationHarness(t)
	bucket := fmt.Sprintf("simples3-cli-extra-%d", time.Now().UnixNano())

	stdout := h.mustRun("mb", "s3://"+bucket)
	if !strings.Contains(stdout, "created s3://"+bucket) {
		t.Fatalf("unexpected mb output: %s", stdout)
	}

	stdout = h.mustRun("ls", "--json")
	if !strings.Contains(stdout, bucket) {
		t.Fatalf("expected bucket %s in ls output: %s", bucket, stdout)
	}

	stdout = h.mustRun("rb", "s3://"+bucket)
	if !strings.Contains(stdout, "deleted s3://"+bucket) {
		t.Fatalf("unexpected rb output: %s", stdout)
	}
}

func TestCLIIntegrationCopyMoveRemoveList(t *testing.T) {
	h := newCLIIntegrationHarness(t)

	sourceDir := t.TempDir()
	sourceFile := filepath.Join(sourceDir, "hello.txt")
	if err := os.WriteFile(sourceFile, []byte("hello from cli"), 0o644); err != nil {
		t.Fatal(err)
	}

	h.mustRun("cp", sourceFile, fmt.Sprintf("s3://%s/uploads/", h.bucket))
	if got := h.readObject("uploads/hello.txt"); got != "hello from cli" {
		t.Fatalf("unexpected uploaded content: %q", got)
	}

	stdout := h.mustRun("ls", "--json", fmt.Sprintf("s3://%s/uploads/", h.bucket))
	if !strings.Contains(stdout, "uploads/hello.txt") {
		t.Fatalf("expected uploaded key in ls output: %s", stdout)
	}

	downloadDir := t.TempDir()
	h.mustRun("cp", fmt.Sprintf("s3://%s/uploads/hello.txt", h.bucket), downloadDir+string(filepath.Separator))
	data, err := os.ReadFile(filepath.Join(downloadDir, "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello from cli" {
		t.Fatalf("unexpected downloaded content: %q", string(data))
	}

	h.mustRun("mv", fmt.Sprintf("s3://%s/uploads/hello.txt", h.bucket), fmt.Sprintf("s3://%s/archive/hello.txt", h.bucket))
	if h.objectExists("uploads/hello.txt") {
		t.Fatalf("expected source object to be removed after mv")
	}
	if got := h.readObject("archive/hello.txt"); got != "hello from cli" {
		t.Fatalf("unexpected moved content: %q", got)
	}

	stdout = h.mustRun("rm", fmt.Sprintf("s3://%s/archive/hello.txt", h.bucket))
	if !strings.Contains(stdout, "deleted s3://"+h.bucket+"/archive/hello.txt") {
		t.Fatalf("unexpected rm output: %s", stdout)
	}
	if h.objectExists("archive/hello.txt") {
		t.Fatalf("expected object to be removed after rm")
	}
}

func TestCLIIntegrationSyncAndPresign(t *testing.T) {
	h := newCLIIntegrationHarness(t)

	sourceDir := t.TempDir()
	freshFile := filepath.Join(sourceDir, "fresh.txt")
	sameSizeFile := filepath.Join(sourceDir, "same-size.txt")
	if err := os.WriteFile(freshFile, []byte("fresh-data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sameSizeFile, []byte("fresh"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.putObject("sync/same-size.txt", "stale")
	h.putObject("sync/stale.txt", "remove-me")
	time.Sleep(1100 * time.Millisecond)
	now := time.Now()
	if err := os.Chtimes(sameSizeFile, now, now); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(freshFile, now, now); err != nil {
		t.Fatal(err)
	}

	stdout := h.mustRun("sync", "--delete", sourceDir, fmt.Sprintf("s3://%s/sync/", h.bucket))
	if !strings.Contains(stdout, "synced ") || !strings.Contains(stdout, "deleted s3://"+h.bucket+"/sync/stale.txt") {
		t.Fatalf("unexpected sync output: %s", stdout)
	}
	if got := h.readObject("sync/same-size.txt"); got != "fresh" {
		t.Fatalf("expected sync to replace same-size object, got %q", got)
	}
	if got := h.readObject("sync/fresh.txt"); got != "fresh-data" {
		t.Fatalf("unexpected synced content: %q", got)
	}
	if h.objectExists("sync/stale.txt") {
		t.Fatalf("expected stale object to be deleted by sync")
	}

	h.putObject("presign.txt", "presigned hello")
	stdout = strings.TrimSpace(h.mustRun("presign", fmt.Sprintf("s3://%s/presign.txt", h.bucket)))
	resp, err := http.Get(stdout)
	if err != nil {
		t.Fatalf("GET presigned URL: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read presigned response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from presigned URL, got %d: %s", resp.StatusCode, string(body))
	}
	if string(body) != "presigned hello" {
		t.Fatalf("unexpected presigned body: %q", string(body))
	}
}

func TestCLIIntegrationTags(t *testing.T) {
	h := newCLIIntegrationHarness(t)
	objectURI := fmt.Sprintf("s3://%s/meta/tagged.txt", h.bucket)
	h.putObject("meta/tagged.txt", "hello tags")

	stdout := h.mustRun("tags", "set", "--json", "--tag", "env=test", "--tag", "team=platform", objectURI)
	var setOutput tagsResult
	decodeJSON(t, stdout, &setOutput)
	if setOutput.Status != "updated" || setOutput.Tags["env"] != "test" || setOutput.Tags["team"] != "platform" {
		t.Fatalf("unexpected tags set output: %+v", setOutput)
	}

	stdout = h.mustRun("tags", "get", "--json", objectURI)
	var getOutput tagsResult
	decodeJSON(t, stdout, &getOutput)
	if getOutput.Status != "loaded" || len(getOutput.Tags) != 2 || getOutput.Tags["team"] != "platform" {
		t.Fatalf("unexpected tags get output: %+v", getOutput)
	}

	stdout = h.mustRun("tags", "delete", "--json", objectURI)
	var deleteOutput tagsResult
	decodeJSON(t, stdout, &deleteOutput)
	if deleteOutput.Status != "deleted" {
		t.Fatalf("unexpected tags delete output: %+v", deleteOutput)
	}

	stdout = h.mustRun("tags", "get", "--json", objectURI)
	var finalGetOutput tagsResult
	decodeJSON(t, stdout, &finalGetOutput)
	if len(finalGetOutput.Tags) != 0 {
		t.Fatalf("expected tags to be empty after delete, got %+v", finalGetOutput.Tags)
	}
}

func TestCLIIntegrationVersioningAndVersions(t *testing.T) {
	h := newCLIIntegrationHarness(t)
	bucketURI := fmt.Sprintf("s3://%s", h.bucket)
	objectKey := "history/object.txt"
	objectURI := fmt.Sprintf("s3://%s/%s", h.bucket, objectKey)

	stdout := h.mustRun("versioning", "set", "--json", "--status", "enabled", bucketURI)
	var setOutput versioningResult
	decodeJSON(t, stdout, &setOutput)
	if setOutput.Status != "Enabled" {
		t.Fatalf("unexpected versioning set output: %+v", setOutput)
	}

	stdout = h.mustRun("versioning", "get", "--json", bucketURI)
	var getOutput versioningResult
	decodeJSON(t, stdout, &getOutput)
	if getOutput.Status != "Enabled" {
		t.Fatalf("unexpected versioning get output: %+v", getOutput)
	}

	h.putObject(objectKey, "v1")
	time.Sleep(1100 * time.Millisecond)
	h.putObject(objectKey, "v2")

	stdout = h.mustRun("versions", "--json", objectURI)
	var versionsOutput versionsResult
	decodeJSON(t, stdout, &versionsOutput)
	if versionsOutput.Bucket != h.bucket || len(versionsOutput.Versions) < 2 {
		t.Fatalf("unexpected versions output: %+v", versionsOutput)
	}
	latestCount := 0
	for _, version := range versionsOutput.Versions {
		if version.IsLatest {
			latestCount++
		}
	}
	if latestCount != 1 {
		t.Fatalf("expected exactly one latest version, got %d in %+v", latestCount, versionsOutput.Versions)
	}
}

func TestCLIIntegrationLifecycleAndACL(t *testing.T) {
	h := newCLIIntegrationHarness(t)
	bucketURI := fmt.Sprintf("s3://%s", h.bucket)
	objectURI := fmt.Sprintf("s3://%s/acl/object.txt", h.bucket)
	lifecycleInput := `{"rules":[{"id":"expire-logs","status":"Enabled","filter":{"prefix":"logs/"},"expiration":{"days":30}}]}`

	stdout := h.mustRunWithInput(lifecycleInput, "lifecycle", "set", "--json", "--file", "-", bucketURI)
	var lifecycleSet lifecycleResult
	decodeJSON(t, stdout, &lifecycleSet)
	if lifecycleSet.Status != "updated" || lifecycleSet.Configuration == nil || len(lifecycleSet.Configuration.Rules) != 1 {
		t.Fatalf("unexpected lifecycle set output: %+v", lifecycleSet)
	}

	stdout = h.mustRun("lifecycle", "get", "--json", bucketURI)
	var lifecycleGet lifecycleResult
	decodeJSON(t, stdout, &lifecycleGet)
	if lifecycleGet.Status != "loaded" || lifecycleGet.Configuration == nil || len(lifecycleGet.Configuration.Rules) != 1 || lifecycleGet.Configuration.Rules[0].Filter == nil || lifecycleGet.Configuration.Rules[0].Filter.Prefix != "logs/" {
		t.Fatalf("unexpected lifecycle get output: %+v", lifecycleGet)
	}

	stdout = h.mustRun("lifecycle", "delete", "--json", bucketURI)
	var lifecycleDelete lifecycleResult
	decodeJSON(t, stdout, &lifecycleDelete)
	if lifecycleDelete.Status != "deleted" {
		t.Fatalf("unexpected lifecycle delete output: %+v", lifecycleDelete)
	}

	h.putObject("acl/object.txt", "hello acl")
	stdout = h.mustRun("acl", "get", "--json", bucketURI)
	var aclGet aclResult
	decodeJSON(t, stdout, &aclGet)
	if aclGet.Status != "loaded" || aclGet.Policy == nil || len(aclGet.Policy.Grants) == 0 {
		t.Fatalf("unexpected bucket acl get output: %+v", aclGet)
	}

	policyBytes, err := json.Marshal(aclGet.Policy)
	if err != nil {
		t.Fatalf("marshal ACL policy: %v", err)
	}
	policyFile := filepath.Join(t.TempDir(), "bucket-acl.json")
	if err := os.WriteFile(policyFile, policyBytes, 0o644); err != nil {
		t.Fatalf("write ACL policy file: %v", err)
	}
	code, stdout, stderr := h.run("acl", "set", "--json", "--policy-file", policyFile, bucketURI)
	if code != 0 {
		if strings.Contains(stdout, "501 Not Implemented") || strings.Contains(stderr, "501 Not Implemented") {
			t.Skip("backend does not support ACL updates")
		}
		t.Fatalf("acl set failed: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	var aclSet aclResult
	decodeJSON(t, stdout, &aclSet)
	if aclSet.Status != "updated" || aclSet.Policy == nil || len(aclSet.Policy.Grants) == 0 {
		t.Fatalf("unexpected bucket acl set output: %+v", aclSet)
	}

	stdout = h.mustRun("acl", "get", "--json", objectURI)
	var objectACL aclResult
	decodeJSON(t, stdout, &objectACL)
	if objectACL.Status != "loaded" || objectACL.Policy == nil || len(objectACL.Policy.Grants) == 0 {
		t.Fatalf("unexpected object acl get output: %+v", objectACL)
	}
}
