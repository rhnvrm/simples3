package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestRuntime(t *testing.T, env map[string]string) (*runtime, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	rt := &runtime{
		stdin:   strings.NewReader(""),
		stdout:  stdout,
		stderr:  stderr,
		getenv:  func(key string) string { return env[key] },
		homeDir: func() (string, error) { return t.TempDir(), nil },
	}
	return rt, stdout, stderr
}

func decodeJSON(t *testing.T, data string, target any) {
	t.Helper()
	if err := json.Unmarshal([]byte(data), target); err != nil {
		t.Fatalf("failed to decode JSON %s: %v", data, err)
	}
}

func TestRunListBucketsJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<ListAllMyBucketsResult>
  <Owner><ID>1</ID><DisplayName>test</DisplayName></Owner>
  <Buckets>
    <Bucket><Name>bucket-one</Name><CreationDate>2026-01-02T03:04:05Z</CreationDate></Bucket>
  </Buckets>
</ListAllMyBucketsResult>`))
	}))
	defer server.Close()

	rt, stdout, stderr := newTestRuntime(t, map[string]string{
		"AWS_ACCESS_KEY_ID":     "test-access",
		"AWS_SECRET_ACCESS_KEY": "test-secret",
	})
	code := rt.run([]string{"ls", "--json", "--endpoint", server.URL})
	if code != 0 {
		t.Fatalf("unexpected exit code %d, stderr=%s", code, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "bucket-one") {
		t.Fatalf("expected bucket output, got %s", output)
	}
	if !strings.Contains(output, `"command": "ls"`) || !strings.Contains(output, `"ok": true`) {
		t.Fatalf("expected JSON envelope fields, got %s", output)
	}
}

func TestRunMakeAndRemoveBucket(t *testing.T) {
	requests := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodPut {
			w.Header().Set("Location", "/example-bucket")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	defer server.Close()

	env := map[string]string{
		"AWS_ACCESS_KEY_ID":     "test-access",
		"AWS_SECRET_ACCESS_KEY": "test-secret",
	}

	rt, stdout, stderr := newTestRuntime(t, env)
	if code := rt.run([]string{"mb", "--endpoint", server.URL, "s3://example-bucket"}); code != 0 {
		t.Fatalf("mb failed: code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "created s3://example-bucket") {
		t.Fatalf("unexpected mb stdout: %s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := rt.run([]string{"rb", "--endpoint", server.URL, "s3://example-bucket"}); code != 0 {
		t.Fatalf("rb failed: code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "deleted s3://example-bucket") {
		t.Fatalf("unexpected rb stdout: %s", stdout.String())
	}

	if len(requests) != 2 || requests[0] != "PUT /example-bucket" || requests[1] != "DELETE /example-bucket" {
		t.Fatalf("unexpected requests: %#v", requests)
	}
}

func TestRunPresign(t *testing.T) {
	rt, stdout, stderr := newTestRuntime(t, map[string]string{
		"AWS_ACCESS_KEY_ID":     "test-access",
		"AWS_SECRET_ACCESS_KEY": "test-secret",
	})
	code := rt.run([]string{"presign", "--endpoint", "https://objects.example.test", "--method", "GET", "--expires", "15m", "s3://example-bucket/path/file.txt"})
	if code != 0 {
		t.Fatalf("unexpected exit code %d, stderr=%s", code, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "X-Amz-Algorithm=AWS4-HMAC-SHA256") {
		t.Fatalf("expected presigned URL output, got %s", output)
	}
	if !strings.Contains(output, "example-bucket") {
		t.Fatalf("expected bucket in URL, got %s", output)
	}
}

func TestRunCopyUploadLocalToS3(t *testing.T) {
	sourceFile := filepath.Join(t.TempDir(), "upload.txt")
	if err := os.WriteFile(sourceFile, []byte("hello upload"), 0o644); err != nil {
		t.Fatal(err)
	}

	var requestPath string
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		body = string(payload)
		w.Header().Set("ETag", "etag")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	rt, stdout, stderr := newTestRuntime(t, map[string]string{
		"AWS_ACCESS_KEY_ID":     "test-access",
		"AWS_SECRET_ACCESS_KEY": "test-secret",
	})
	code := rt.run([]string{"cp", "--endpoint", server.URL, sourceFile, "s3://example-bucket/uploads/"})
	if code != 0 {
		t.Fatalf("unexpected exit code %d, stderr=%s", code, stderr.String())
	}
	if requestPath != "/example-bucket/uploads/upload.txt" {
		t.Fatalf("unexpected request path: %s", requestPath)
	}
	if body != "hello upload" {
		t.Fatalf("unexpected uploaded body: %q", body)
	}
	if !strings.Contains(stdout.String(), "copied ") {
		t.Fatalf("expected copy output, got %s", stdout.String())
	}
}

func TestRunCopyDownloadS3ToLocal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/example-bucket/path/file.txt" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", "18")
			w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte("downloaded content"))
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	defer server.Close()

	destinationDir := t.TempDir()
	rt, stdout, stderr := newTestRuntime(t, map[string]string{
		"AWS_ACCESS_KEY_ID":     "test-access",
		"AWS_SECRET_ACCESS_KEY": "test-secret",
	})
	code := rt.run([]string{"cp", "--endpoint", server.URL, "s3://example-bucket/path/file.txt", destinationDir + string(filepath.Separator)})
	if code != 0 {
		t.Fatalf("unexpected exit code %d, stderr=%s", code, stderr.String())
	}
	data, err := os.ReadFile(filepath.Join(destinationDir, "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "downloaded content" {
		t.Fatalf("unexpected downloaded content: %q", string(data))
	}
	if !strings.Contains(stdout.String(), "copied ") {
		t.Fatalf("expected copy output, got %s", stdout.String())
	}
}

func TestRunRemoveRecursiveDryRun(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.Method == http.MethodGet && r.URL.Query().Get("list-type") == "2" {
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult>
  <Name>example-bucket</Name>
  <IsTruncated>false</IsTruncated>
  <KeyCount>1</KeyCount>
  <Contents>
    <Key>prefix/one.txt</Key>
    <LastModified>2026-01-02T03:04:05Z</LastModified>
    <ETag>etag</ETag>
    <Size>12</Size>
    <StorageClass>STANDARD</StorageClass>
  </Contents>
</ListBucketResult>`))
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	defer server.Close()

	rt, stdout, stderr := newTestRuntime(t, map[string]string{
		"AWS_ACCESS_KEY_ID":     "test-access",
		"AWS_SECRET_ACCESS_KEY": "test-secret",
	})
	code := rt.run([]string{"rm", "--recursive", "--dry-run", "--endpoint", server.URL, "s3://example-bucket/prefix/"})
	if code != 0 {
		t.Fatalf("unexpected exit code %d, stderr=%s", code, stderr.String())
	}
	if requestCount != 1 {
		t.Fatalf("expected one list request, got %d", requestCount)
	}
	if !strings.Contains(stdout.String(), "would-delete s3://example-bucket/prefix/one.txt") {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestRunRemoveSingleObjectRetriesTransientFailure(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/example-bucket/object.txt" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("temporary failure"))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	rt, stdout, stderr := newTestRuntime(t, map[string]string{
		"AWS_ACCESS_KEY_ID":     "test-access",
		"AWS_SECRET_ACCESS_KEY": "test-secret",
	})
	code := rt.run([]string{"rm", "--retries", "1", "--endpoint", server.URL, "s3://example-bucket/object.txt"})
	if code != 0 {
		t.Fatalf("unexpected exit code %d, stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if attempts != 2 {
		t.Fatalf("expected 2 delete attempts, got %d", attempts)
	}
	if !strings.Contains(stdout.String(), "deleted s3://example-bucket/object.txt") {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestRunMoveLocalToS3RemovesSource(t *testing.T) {
	sourceFile := filepath.Join(t.TempDir(), "move.txt")
	if err := os.WriteFile(sourceFile, []byte("move me"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/example-bucket/archive/move.txt" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("ETag", "etag")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	rt, stdout, stderr := newTestRuntime(t, map[string]string{
		"AWS_ACCESS_KEY_ID":     "test-access",
		"AWS_SECRET_ACCESS_KEY": "test-secret",
	})
	code := rt.run([]string{"mv", "--endpoint", server.URL, sourceFile, "s3://example-bucket/archive/"})
	if code != 0 {
		t.Fatalf("unexpected exit code %d, stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(sourceFile); !os.IsNotExist(err) {
		t.Fatalf("expected source file to be removed, stat err=%v", err)
	}
	if !strings.Contains(stdout.String(), "moved ") {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestRunCopyRequiresRecursiveForS3PrefixSource(t *testing.T) {
	rt, _, stderr := newTestRuntime(t, nil)
	code := rt.run([]string{"cp", "s3://example-bucket/prefix/", filepath.Join(t.TempDir(), "out")})
	if code == 0 {
		t.Fatalf("expected cp to fail without --recursive")
	}
	if !strings.Contains(stderr.String(), "use --recursive") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestRunMoveRequiresRecursiveForS3PrefixSource(t *testing.T) {
	rt, _, stderr := newTestRuntime(t, nil)
	code := rt.run([]string{"mv", "s3://example-bucket/prefix/", "s3://example-bucket/dst"})
	if code == 0 {
		t.Fatalf("expected mv to fail without --recursive")
	}
	if !strings.Contains(stderr.String(), "use --recursive") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestRunSyncUploadsAndDeletes(t *testing.T) {
	sourceDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceDir, "new.txt"), []byte("fresh"), 0o644); err != nil {
		t.Fatal(err)
	}

	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		switch {
		case r.Method == http.MethodHead && r.URL.Path == "/example-bucket/prefix/new.txt":
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPut && r.URL.Path == "/example-bucket/prefix/new.txt":
			w.Header().Set("ETag", "etag")
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Query().Get("list-type") == "2":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult>
  <Name>example-bucket</Name>
  <IsTruncated>false</IsTruncated>
  <KeyCount>1</KeyCount>
  <Contents>
    <Key>prefix/stale.txt</Key>
    <LastModified>2026-01-02T03:04:05Z</LastModified>
    <ETag>etag</ETag>
    <Size>9</Size>
    <StorageClass>STANDARD</StorageClass>
  </Contents>
</ListBucketResult>`))
		case r.Method == http.MethodDelete && r.URL.Path == "/example-bucket/prefix/stale.txt":
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	rt, stdout, stderr := newTestRuntime(t, map[string]string{
		"AWS_ACCESS_KEY_ID":     "test-access",
		"AWS_SECRET_ACCESS_KEY": "test-secret",
	})
	code := rt.run([]string{"sync", "--delete", "--endpoint", server.URL, sourceDir, "s3://example-bucket/prefix/"})
	if code != 0 {
		t.Fatalf("unexpected exit code %d, stderr=%s", code, stderr.String())
	}
	joined := strings.Join(requests, "\n")
	if !strings.Contains(joined, "HEAD /example-bucket/prefix/new.txt?") || !strings.Contains(joined, "PUT /example-bucket/prefix/new.txt?") || !strings.Contains(joined, "DELETE /example-bucket/prefix/stale.txt?") {
		t.Fatalf("unexpected request sequence:\n%s", joined)
	}
	if !strings.Contains(stdout.String(), "synced ") || !strings.Contains(stdout.String(), "deleted s3://example-bucket/prefix/stale.txt") {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestRunCopyDryRunJSONIncludesSummary(t *testing.T) {
	sourceFile := filepath.Join(t.TempDir(), "upload.txt")
	if err := os.WriteFile(sourceFile, []byte("hello upload"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt, stdout, stderr := newTestRuntime(t, map[string]string{
		"AWS_ACCESS_KEY_ID":     "test-access",
		"AWS_SECRET_ACCESS_KEY": "test-secret",
	})
	code := rt.run([]string{"cp", "--dry-run", "--json", sourceFile, "s3://example-bucket/uploads/"})
	if code != 0 {
		t.Fatalf("unexpected exit code %d, stderr=%s", code, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, `"command": "cp"`) || !strings.Contains(output, `"dryRun": true`) || !strings.Contains(output, `"changed": 1`) {
		t.Fatalf("unexpected JSON output: %s", output)
	}
}

func TestRunJSONUsageErrorIncludesExitCode(t *testing.T) {
	rt, stdout, stderr := newTestRuntime(t, nil)
	code := rt.run([]string{"cp", "--json", filepath.Join(t.TempDir(), "a"), filepath.Join(t.TempDir(), "b")})
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d (stderr=%s)", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr in JSON mode, got %s", stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, `"ok": false`) || !strings.Contains(output, `"type": "usage"`) || !strings.Contains(output, `"exitCode": 2`) {
		t.Fatalf("unexpected JSON error output: %s", output)
	}
}

func TestRunSingleDashJSONUsageErrorIncludesExitCode(t *testing.T) {
	rt, stdout, stderr := newTestRuntime(t, nil)
	code := rt.run([]string{"cp", "-json", filepath.Join(t.TempDir(), "a"), filepath.Join(t.TempDir(), "b")})
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d (stderr=%s)", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr in JSON mode, got %s", stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, `"ok": false`) || !strings.Contains(output, `"type": "usage"`) || !strings.Contains(output, `"exitCode": 2`) {
		t.Fatalf("unexpected JSON error output: %s", output)
	}
}

func TestRunTextUsageErrorReturnsExitCodeTwo(t *testing.T) {
	rt, stdout, stderr := newTestRuntime(t, nil)
	code := rt.run([]string{"cp", filepath.Join(t.TempDir(), "a"), filepath.Join(t.TempDir(), "b")})
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected empty stdout, got %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "local-to-local copies are not supported") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestRunSyncUpdatesS3TargetWhenOnlyModTimeDiffers(t *testing.T) {
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "same-size.txt")
	if err := os.WriteFile(sourcePath, []byte("fresh"), 0o644); err != nil {
		t.Fatal(err)
	}
	sourceTime := time.Date(2026, 1, 3, 4, 5, 6, 0, time.UTC)
	if err := os.Chtimes(sourcePath, sourceTime, sourceTime); err != nil {
		t.Fatal(err)
	}

	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		switch {
		case r.Method == http.MethodHead && r.URL.Path == "/example-bucket/prefix/same-size.txt":
			w.Header().Set("Content-Length", "5")
			w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPut && r.URL.Path == "/example-bucket/prefix/same-size.txt":
			w.Header().Set("ETag", "etag")
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	rt, stdout, stderr := newTestRuntime(t, map[string]string{
		"AWS_ACCESS_KEY_ID":     "test-access",
		"AWS_SECRET_ACCESS_KEY": "test-secret",
	})
	code := rt.run([]string{"sync", "--endpoint", server.URL, sourceDir, "s3://example-bucket/prefix/"})
	if code != 0 {
		t.Fatalf("unexpected exit code %d, stderr=%s", code, stderr.String())
	}
	joined := strings.Join(requests, "\n")
	if !strings.Contains(joined, "HEAD /example-bucket/prefix/same-size.txt?") || !strings.Contains(joined, "PUT /example-bucket/prefix/same-size.txt?") {
		t.Fatalf("expected sync to update same-size object when modtime differs, got:\n%s", joined)
	}
	if !strings.Contains(stdout.String(), "synced ") {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestRunCopyRetriesTransientUploadFailure(t *testing.T) {
	sourceFile := filepath.Join(t.TempDir(), "retry.txt")
	if err := os.WriteFile(sourceFile, []byte("retry me"), 0o644); err != nil {
		t.Fatal(err)
	}

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/example-bucket/uploads/retry.txt" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("temporary failure"))
			return
		}
		w.Header().Set("ETag", "etag")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	rt, stdout, stderr := newTestRuntime(t, map[string]string{
		"AWS_ACCESS_KEY_ID":     "test-access",
		"AWS_SECRET_ACCESS_KEY": "test-secret",
	})
	code := rt.run([]string{"cp", "--retries", "1", "--endpoint", server.URL, sourceFile, "s3://example-bucket/uploads/"})
	if code != 0 {
		t.Fatalf("unexpected exit code %d, stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
	if !strings.Contains(stdout.String(), "copied ") {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestRunCopyContinueOnErrorJSONReturnsPartialFailure(t *testing.T) {
	sourceDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceDir, "bad.txt"), []byte("bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "ok.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		switch r.URL.Path {
		case "/example-bucket/prefix/bad.txt":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("boom"))
		case "/example-bucket/prefix/ok.txt":
			w.Header().Set("ETag", "etag")
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	rt, stdout, stderr := newTestRuntime(t, map[string]string{
		"AWS_ACCESS_KEY_ID":     "test-access",
		"AWS_SECRET_ACCESS_KEY": "test-secret",
	})
	code := rt.run([]string{"cp", "--recursive", "--continue-on-error", "--retries", "0", "--json", "--endpoint", server.URL, sourceDir, "s3://example-bucket/prefix/"})
	if code != 3 {
		t.Fatalf("expected exit code 3, got %d (stdout=%s stderr=%s)", code, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %s", stderr.String())
	}
	var output struct {
		Command string             `json:"command"`
		OK      bool               `json:"ok"`
		Error   commandErrorDetail `json:"error"`
		Data    operationsData     `json:"data"`
	}
	decodeJSON(t, stdout.String(), &output)
	if output.Command != "cp" || output.OK {
		t.Fatalf("unexpected partial failure envelope: %+v", output)
	}
	if output.Error.Type != string(cliErrorPartial) || output.Error.ExitCode != 3 {
		t.Fatalf("unexpected error detail: %+v", output.Error)
	}
	if output.Data.Summary.Changed != 1 || output.Data.Summary.Failed != 1 || output.Data.Summary.Total != 2 {
		t.Fatalf("unexpected summary: %+v", output.Data.Summary)
	}
	if len(output.Data.Operations) != 2 {
		t.Fatalf("expected 2 operations, got %d", len(output.Data.Operations))
	}
	if output.Data.Operations[0].Status != "failed" || output.Data.Operations[0].Error == "" {
		t.Fatalf("expected first operation to fail with details, got %+v", output.Data.Operations[0])
	}
	if output.Data.Operations[1].Status != "copied" {
		t.Fatalf("expected second operation to succeed, got %+v", output.Data.Operations[1])
	}
}

func TestRunSyncPlanJSONIncludesUnchangedEntries(t *testing.T) {
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "same.txt")
	if err := os.WriteFile(sourcePath, []byte("same"), 0o644); err != nil {
		t.Fatal(err)
	}
	sourceTime := time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)
	if err := os.Chtimes(sourcePath, sourceTime, sourceTime); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead && r.URL.Path == "/example-bucket/prefix/same.txt" {
			w.Header().Set("Content-Length", "4")
			w.Header().Set("Last-Modified", sourceTime.Format(http.TimeFormat))
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	defer server.Close()

	rt, stdout, stderr := newTestRuntime(t, map[string]string{
		"AWS_ACCESS_KEY_ID":     "test-access",
		"AWS_SECRET_ACCESS_KEY": "test-secret",
	})
	code := rt.run([]string{"sync", "--plan", "--json", "--endpoint", server.URL, sourceDir, "s3://example-bucket/prefix/"})
	if code != 0 {
		t.Fatalf("unexpected exit code %d, stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var output operationsOutput
	decodeJSON(t, stdout.String(), &output)
	if output.Command != "sync" || !output.OK {
		t.Fatalf("unexpected output: %+v", output)
	}
	if output.Summary.Total != 1 || output.Summary.Unchanged != 1 || !output.Summary.Noop || !output.Summary.DryRun {
		t.Fatalf("unexpected summary: %+v", output.Summary)
	}
	if len(output.Operations) != 1 || output.Operations[0].Status != "unchanged" {
		t.Fatalf("unexpected operations: %+v", output.Operations)
	}
}

func TestRunRemoveRecursiveContinueOnErrorJSONReturnsPartialFailure(t *testing.T) {
	deleteRequests := 0
	var deleteBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Query().Get("list-type") == "2":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult>
  <Name>example-bucket</Name>
  <IsTruncated>false</IsTruncated>
  <KeyCount>2</KeyCount>
  <Contents>
    <Key>prefix/fail.txt</Key>
    <LastModified>2026-01-02T03:04:05Z</LastModified>
    <ETag>etag</ETag>
    <Size>4</Size>
    <StorageClass>STANDARD</StorageClass>
  </Contents>
  <Contents>
    <Key>prefix/ok.txt</Key>
    <LastModified>2026-01-02T03:04:05Z</LastModified>
    <ETag>etag</ETag>
    <Size>2</Size>
    <StorageClass>STANDARD</StorageClass>
  </Contents>
</ListBucketResult>`))
		case r.Method == http.MethodPost && r.URL.RawQuery == "delete":
			deleteRequests++
			payload, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			deleteBody = string(payload)
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<DeleteResult>
  <Deleted><Key>prefix/ok.txt</Key></Deleted>
  <Error><Key>prefix/fail.txt</Key><Code>InternalError</Code><Message>delete failed</Message></Error>
</DeleteResult>`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	rt, stdout, stderr := newTestRuntime(t, map[string]string{
		"AWS_ACCESS_KEY_ID":     "test-access",
		"AWS_SECRET_ACCESS_KEY": "test-secret",
	})
	code := rt.run([]string{"rm", "--recursive", "--continue-on-error", "--retries", "0", "--json", "--endpoint", server.URL, "s3://example-bucket/prefix/"})
	if code != 3 {
		t.Fatalf("expected exit code 3, got %d (stdout=%s stderr=%s)", code, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %s", stderr.String())
	}
	if deleteRequests != 1 {
		t.Fatalf("expected one batched delete request, got %d", deleteRequests)
	}
	if !strings.Contains(deleteBody, "<Key>prefix/fail.txt</Key>") || !strings.Contains(deleteBody, "<Key>prefix/ok.txt</Key>") {
		t.Fatalf("unexpected delete body: %s", deleteBody)
	}
	var output struct {
		Command string             `json:"command"`
		OK      bool               `json:"ok"`
		Error   commandErrorDetail `json:"error"`
		Data    operationsData     `json:"data"`
	}
	decodeJSON(t, stdout.String(), &output)
	if output.Command != "rm" || output.OK {
		t.Fatalf("unexpected partial failure envelope: %+v", output)
	}
	if output.Data.Summary.Deleted != 1 || output.Data.Summary.Failed != 1 || output.Data.Summary.Total != 2 {
		t.Fatalf("unexpected summary: %+v", output.Data.Summary)
	}
	if len(output.Data.Operations) != 2 {
		t.Fatalf("expected 2 operations, got %d", len(output.Data.Operations))
	}
	if output.Data.Operations[0].Status != "failed" || output.Data.Operations[1].Status != "deleted" {
		t.Fatalf("unexpected operations: %+v", output.Data.Operations)
	}
	if output.Data.Operations[0].Error != "InternalError: delete failed" {
		t.Fatalf("unexpected failure detail: %+v", output.Data.Operations[0])
	}
}

func TestRunTagsCommands(t *testing.T) {
	var putBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/example-bucket/object.txt" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if _, ok := r.URL.Query()["tagging"]; !ok {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		switch r.Method {
		case http.MethodPut:
			payload, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			putBody = string(payload)
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<Tagging>
  <TagSet>
    <Tag><Key>env</Key><Value>test</Value></Tag>
    <Tag><Key>team</Key><Value>platform</Value></Tag>
  </TagSet>
</Tagging>`))
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	tagsFile := filepath.Join(t.TempDir(), "tags.json")
	if err := os.WriteFile(tagsFile, []byte(`{"env":"test","team":"platform"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	env := map[string]string{"AWS_ACCESS_KEY_ID": "test-access", "AWS_SECRET_ACCESS_KEY": "test-secret"}
	rt, stdout, stderr := newTestRuntime(t, env)
	if code := rt.run([]string{"tags", "set", "--json", "--endpoint", server.URL, "--tags-file", tagsFile, "s3://example-bucket/object.txt"}); code != 0 {
		t.Fatalf("tags set failed: code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(putBody, "<Key>env</Key>") || !strings.Contains(putBody, "<Key>team</Key>") {
		t.Fatalf("unexpected tagging PUT body: %s", putBody)
	}
	var setOutput tagsResult
	decodeJSON(t, stdout.String(), &setOutput)
	if setOutput.Status != "updated" || setOutput.Tags["env"] != "test" {
		t.Fatalf("unexpected tags set output: %+v", setOutput)
	}

	rt, stdout, stderr = newTestRuntime(t, env)
	if code := rt.run([]string{"tags", "get", "--json", "--endpoint", server.URL, "s3://example-bucket/object.txt"}); code != 0 {
		t.Fatalf("tags get failed: code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var getOutput tagsResult
	decodeJSON(t, stdout.String(), &getOutput)
	if getOutput.Status != "loaded" || getOutput.Tags["team"] != "platform" {
		t.Fatalf("unexpected tags get output: %+v", getOutput)
	}

	rt, stdout, stderr = newTestRuntime(t, env)
	if code := rt.run([]string{"tags", "delete", "--json", "--endpoint", server.URL, "s3://example-bucket/object.txt"}); code != 0 {
		t.Fatalf("tags delete failed: code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var deleteOutput tagsResult
	decodeJSON(t, stdout.String(), &deleteOutput)
	if deleteOutput.Status != "deleted" {
		t.Fatalf("unexpected tags delete output: %+v", deleteOutput)
	}
}

func TestRunVersioningCommands(t *testing.T) {
	var putBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/example-bucket" || r.URL.RawQuery != "versioning" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		switch r.Method {
		case http.MethodPut:
			payload, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			putBody = string(payload)
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<VersioningConfiguration>
  <Status>Enabled</Status>
</VersioningConfiguration>`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	env := map[string]string{"AWS_ACCESS_KEY_ID": "test-access", "AWS_SECRET_ACCESS_KEY": "test-secret"}
	rt, stdout, stderr := newTestRuntime(t, env)
	if code := rt.run([]string{"versioning", "set", "--json", "--endpoint", server.URL, "--status", "enabled", "s3://example-bucket"}); code != 0 {
		t.Fatalf("versioning set failed: code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(putBody, "<Status>Enabled</Status>") {
		t.Fatalf("unexpected versioning PUT body: %s", putBody)
	}
	var setOutput versioningResult
	decodeJSON(t, stdout.String(), &setOutput)
	if setOutput.Status != "Enabled" {
		t.Fatalf("unexpected versioning set output: %+v", setOutput)
	}

	rt, stdout, stderr = newTestRuntime(t, env)
	if code := rt.run([]string{"versioning", "get", "--json", "--endpoint", server.URL, "s3://example-bucket"}); code != 0 {
		t.Fatalf("versioning get failed: code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var getOutput versioningResult
	decodeJSON(t, stdout.String(), &getOutput)
	if getOutput.Status != "Enabled" {
		t.Fatalf("unexpected versioning get output: %+v", getOutput)
	}
}

func TestRunVersionsJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if r.Method != http.MethodGet || r.URL.Path != "/example-bucket" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if _, ok := query["versions"]; !ok || query.Get("prefix") != "prefix/" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<ListVersionsResult>
  <Name>example-bucket</Name>
  <Prefix>prefix/</Prefix>
  <IsTruncated>false</IsTruncated>
  <Version>
    <Key>prefix/object.txt</Key>
    <VersionId>v2</VersionId>
    <IsLatest>true</IsLatest>
    <LastModified>2026-01-02T03:04:05Z</LastModified>
    <ETag>etag</ETag>
    <Size>12</Size>
    <StorageClass>STANDARD</StorageClass>
    <Owner><ID>owner-id</ID><DisplayName>owner</DisplayName></Owner>
  </Version>
  <DeleteMarker>
    <Key>prefix/object.txt</Key>
    <VersionId>v1</VersionId>
    <IsLatest>false</IsLatest>
    <LastModified>2026-01-01T03:04:05Z</LastModified>
    <Owner><ID>owner-id</ID><DisplayName>owner</DisplayName></Owner>
  </DeleteMarker>
</ListVersionsResult>`))
	}))
	defer server.Close()

	rt, stdout, stderr := newTestRuntime(t, map[string]string{"AWS_ACCESS_KEY_ID": "test-access", "AWS_SECRET_ACCESS_KEY": "test-secret"})
	if code := rt.run([]string{"versions", "--json", "--endpoint", server.URL, "s3://example-bucket/prefix/"}); code != 0 {
		t.Fatalf("versions failed: code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var output versionsResult
	decodeJSON(t, stdout.String(), &output)
	if output.Bucket != "example-bucket" || len(output.Versions) != 1 || output.Versions[0].VersionID != "v2" || len(output.DeleteMarkers) != 1 {
		t.Fatalf("unexpected versions output: %+v", output)
	}
}

func TestRunLifecycleCommands(t *testing.T) {
	var putBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/example-bucket" || r.URL.RawQuery != "lifecycle" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		switch r.Method {
		case http.MethodPut:
			payload, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			putBody = string(payload)
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<LifecycleConfiguration>
  <Rule>
    <ID>expire</ID>
    <Status>Enabled</Status>
    <Expiration><Days>30</Days></Expiration>
  </Rule>
</LifecycleConfiguration>`))
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	rt, stdout, stderr := newTestRuntime(t, map[string]string{"AWS_ACCESS_KEY_ID": "test-access", "AWS_SECRET_ACCESS_KEY": "test-secret"})
	rt.stdin = strings.NewReader(`{"rules":[{"id":"expire","status":"Enabled","expiration":{"days":30}}]}`)
	if code := rt.run([]string{"lifecycle", "set", "--json", "--endpoint", server.URL, "--file", "-", "s3://example-bucket"}); code != 0 {
		t.Fatalf("lifecycle set failed: code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(putBody, "<ID>expire</ID>") || !strings.Contains(putBody, "<Days>30</Days>") {
		t.Fatalf("unexpected lifecycle PUT body: %s", putBody)
	}
	var setOutput lifecycleResult
	decodeJSON(t, stdout.String(), &setOutput)
	if setOutput.Status != "updated" || setOutput.Configuration == nil || len(setOutput.Configuration.Rules) != 1 || setOutput.Configuration.Rules[0].Expiration == nil || setOutput.Configuration.Rules[0].Expiration.Days != 30 {
		t.Fatalf("unexpected lifecycle set output: %+v", setOutput)
	}

	rt, stdout, stderr = newTestRuntime(t, map[string]string{"AWS_ACCESS_KEY_ID": "test-access", "AWS_SECRET_ACCESS_KEY": "test-secret"})
	if code := rt.run([]string{"lifecycle", "get", "--json", "--endpoint", server.URL, "s3://example-bucket"}); code != 0 {
		t.Fatalf("lifecycle get failed: code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var getOutput lifecycleResult
	decodeJSON(t, stdout.String(), &getOutput)
	if getOutput.Status != "loaded" || getOutput.Configuration == nil || len(getOutput.Configuration.Rules) != 1 || getOutput.Configuration.Rules[0].ID != "expire" {
		t.Fatalf("unexpected lifecycle get output: %+v", getOutput)
	}

	rt, stdout, stderr = newTestRuntime(t, map[string]string{"AWS_ACCESS_KEY_ID": "test-access", "AWS_SECRET_ACCESS_KEY": "test-secret"})
	if code := rt.run([]string{"lifecycle", "delete", "--json", "--endpoint", server.URL, "s3://example-bucket"}); code != 0 {
		t.Fatalf("lifecycle delete failed: code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var deleteOutput lifecycleResult
	decodeJSON(t, stdout.String(), &deleteOutput)
	if deleteOutput.Status != "deleted" {
		t.Fatalf("unexpected lifecycle delete output: %+v", deleteOutput)
	}
}

func TestRunACLCommands(t *testing.T) {
	var putBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/example-bucket" || r.URL.RawQuery != "acl" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		switch r.Method {
		case http.MethodPut:
			payload, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			putBody = string(payload)
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<AccessControlPolicy>
  <Owner><ID>owner-id</ID><DisplayName>owner</DisplayName></Owner>
  <AccessControlList>
    <Grant>
      <Grantee xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:type="CanonicalUser">
        <ID>owner-id</ID>
        <DisplayName>owner</DisplayName>
      </Grantee>
      <Permission>FULL_CONTROL</Permission>
    </Grant>
  </AccessControlList>
</AccessControlPolicy>`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	policyFile := filepath.Join(t.TempDir(), "acl.json")
	policy := `{"owner":{"id":"owner-id","displayName":"owner"},"grants":[{"grantee":{"type":"CanonicalUser","id":"owner-id","displayName":"owner"},"permission":"FULL_CONTROL"}]}`
	if err := os.WriteFile(policyFile, []byte(policy), 0o644); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"AWS_ACCESS_KEY_ID": "test-access", "AWS_SECRET_ACCESS_KEY": "test-secret"}
	rt, stdout, stderr := newTestRuntime(t, env)
	if code := rt.run([]string{"acl", "set", "--json", "--endpoint", server.URL, "--policy-file", policyFile, "s3://example-bucket"}); code != 0 {
		t.Fatalf("acl set failed: code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(putBody, "<Permission>FULL_CONTROL</Permission>") {
		t.Fatalf("unexpected ACL PUT body: %s", putBody)
	}
	var setOutput aclResult
	decodeJSON(t, stdout.String(), &setOutput)
	if setOutput.Status != "updated" || setOutput.Policy == nil || len(setOutput.Policy.Grants) != 1 || setOutput.Policy.Owner.ID != "owner-id" {
		t.Fatalf("unexpected acl set output: %+v", setOutput)
	}

	rt, stdout, stderr = newTestRuntime(t, env)
	if code := rt.run([]string{"acl", "get", "--json", "--endpoint", server.URL, "s3://example-bucket"}); code != 0 {
		t.Fatalf("acl get failed: code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var getOutput aclResult
	decodeJSON(t, stdout.String(), &getOutput)
	if getOutput.Status != "loaded" || getOutput.Policy == nil || getOutput.Policy.Owner.ID != "owner-id" || len(getOutput.Policy.Grants) != 1 {
		t.Fatalf("unexpected acl get output: %+v", getOutput)
	}
}
