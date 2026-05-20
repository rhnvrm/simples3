package main

import (
	"bytes"
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
	seq, finish := h.client.ListAll(simples3.ListInput{Bucket: h.bucket})
	keys := make([]string, 0)
	for object := range seq {
		keys = append(keys, object.Key)
	}
	if err := finish(); err != nil && !strings.Contains(strings.ToLower(err.Error()), "not found") {
		h.t.Fatalf("list cleanup bucket %s: %v", h.bucket, err)
	}
	for start := 0; start < len(keys); start += 1000 {
		end := start + 1000
		if end > len(keys) {
			end = len(keys)
		}
		if _, err := h.client.DeleteObjects(simples3.DeleteObjectsInput{Bucket: h.bucket, Objects: keys[start:end], Quiet: true}); err != nil {
			h.t.Fatalf("delete cleanup objects from %s: %v", h.bucket, err)
		}
	}
	if err := h.client.DeleteBucket(simples3.DeleteBucketInput{Bucket: h.bucket}); err != nil && !strings.Contains(strings.ToLower(err.Error()), "not found") {
		h.t.Fatalf("delete cleanup bucket %s: %v", h.bucket, err)
	}
}

func (h *cliIntegrationHarness) run(args ...string) (int, string, string) {
	h.t.Helper()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	rt := &runtime{
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
		h.t.Fatalf("command failed (%v): %s", args, stderr)
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
