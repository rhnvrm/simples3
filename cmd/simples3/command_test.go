package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
