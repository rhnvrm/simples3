package main

import (
	"bytes"
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
		stdout:  stdout,
		stderr:  stderr,
		getenv:  func(key string) string { return env[key] },
		homeDir: func() (string, error) { return t.TempDir(), nil },
	}
	return rt, stdout, stderr
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
	if !strings.Contains(stdout.String(), "bucket-one") {
		t.Fatalf("expected bucket output, got %s", stdout.String())
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
