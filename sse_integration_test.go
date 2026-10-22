package simples3

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"testing"
	"time"
)

// Run against an existing bucket on an S3-compatible endpoint over HTTPS with
// SIMPLES3_SSE_C_INTEGRATION=1 and the usual AWS_S3_* test environment variables.
func TestSSECIntegration(t *testing.T) {
	if os.Getenv("SIMPLES3_SSE_C_INTEGRATION") != "1" {
		t.Skip("set SIMPLES3_SSE_C_INTEGRATION=1 to test a real HTTPS S3 endpoint")
	}
	s3 := setupTestS3(t)
	// The local SeaweedFS fixture exposes HTTP on loopback. Terminate TLS in a
	// test-only proxy so the client exercises its HTTPS requirement while actual
	// encryption, key rejection and object operations run against SeaweedFS.
	if os.Getenv("SIMPLES3_SSE_C_LOOPBACK_PROXY") == "1" {
		configureSSECLoopbackProxy(t, s3)
	}

	bucket := os.Getenv("AWS_S3_BUCKET")
	if bucket == "" {
		t.Fatal("AWS_S3_BUCKET must name an existing test bucket")
	}
	key := make([]byte, 32)
	newKey := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("Failed to generate SSE-C key: %v", err)
	}
	if _, err := rand.Read(newKey); err != nil {
		t.Fatalf("Failed to generate SSE-C key: %v", err)
	}
	prefix := fmt.Sprintf("simples3-ssec-%d", time.Now().UnixNano())
	for _, suffix := range []string{"put", "copy", "multipart"} {
		objectKey := prefix + "-" + suffix
		t.Cleanup(func() {
			if err := s3.FileDelete(DeleteInput{
				Bucket:    bucket,
				ObjectKey: objectKey,
			}); err != nil {
				t.Logf("cleanup %s: %v", objectKey, err)
			}
		})
	}
	content := []byte("SSE-C integration content")
	if _, err := s3.FilePut(UploadInput{
		Bucket:         bucket,
		ObjectKey:      prefix + "-put",
		Body:           bytes.NewReader(content),
		SSECustomerKey: key,
	}); err != nil {
		t.Fatalf("FilePut with SSE-C failed: %v", err)
	}
	check := func(objectKey string, key, content []byte) {
		t.Helper()
		if _, err := s3.FileDetails(DetailsInput{
			Bucket:         bucket,
			ObjectKey:      objectKey,
			SSECustomerKey: key,
		}); err != nil {
			t.Fatalf("FileDetails with SSE-C failed: %v", err)
		}
		body, err := s3.FileDownload(DownloadInput{
			Bucket:         bucket,
			ObjectKey:      objectKey,
			SSECustomerKey: key,
		})
		if err != nil {
			t.Fatalf("FileDownload with SSE-C failed: %v", err)
		}
		got, err := io.ReadAll(body)
		body.Close()
		if err != nil {
			t.Fatalf("Failed to read SSE-C response: %v", err)
		}
		if !bytes.Equal(got, content) {
			t.Fatal("download content mismatch")
		}
	}
	check(prefix+"-put", key, content)
	for _, wrongKey := range [][]byte{nil, newKey} {
		if body, err := s3.FileDownload(DownloadInput{
			Bucket:         bucket,
			ObjectKey:      prefix + "-put",
			SSECustomerKey: wrongKey,
		}); err == nil {
			body.Close()
			t.Fatal("download accepted missing/wrong key")
		}
		if _, err := s3.FileDetails(DetailsInput{
			Bucket:         bucket,
			ObjectKey:      prefix + "-put",
			SSECustomerKey: wrongKey,
		}); err == nil {
			t.Fatal("HEAD accepted missing/wrong key")
		}
	}
	if _, err := s3.CopyObject(CopyObjectInput{
		SourceBucket:         bucket,
		SourceKey:            prefix + "-put",
		DestBucket:           bucket,
		DestKey:              prefix + "-copy",
		SourceSSECustomerKey: key,
		SSECustomerKey:       newKey,
	}); err != nil {
		t.Fatalf("CopyObject with SSE-C failed: %v", err)
	}
	check(prefix+"-copy", newKey, content)
	multipartContent := bytes.Repeat([]byte("x"), int(MinPartSize)+1)
	if _, err := s3.FileUploadMultipart(MultipartUploadInput{
		Bucket:         bucket,
		ObjectKey:      prefix + "-multipart",
		Body:           bytes.NewReader(multipartContent),
		SSECustomerKey: key,
		Concurrency:    2,
	}); err != nil {
		t.Fatalf("FileUploadMultipart with SSE-C failed: %v", err)
	}
	check(prefix+"-multipart", key, multipartContent)
	headers, err := SSECustomerHeaders(key)
	if err != nil {
		t.Fatalf("SSECustomerHeaders failed: %v", err)
	}
	signedURL := s3.GeneratePresignedURL(PresignedInput{
		Bucket:        bucket,
		ObjectKey:     prefix + "-put",
		Method:        http.MethodGet,
		ExpirySeconds: 60,
		ExtraHeaders:  headers,
	})
	req, err := http.NewRequest(http.MethodGet, signedURL, nil)
	if err != nil {
		t.Fatalf("Failed to create presigned request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := s3.doSSERequest(req)
	if err != nil {
		t.Fatalf("Presigned GET with SSE-C failed: %v", err)
	}
	defer res.Body.Close()
	got, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("Failed to read presigned response: %v", err)
	}
	if res.StatusCode != http.StatusOK || !bytes.Equal(got, content) {
		t.Fatalf("presigned GET failed: %s", res.Status)
	}
}

// configureSSECLoopbackProxy is shared with the local signature regression test.
func configureSSECLoopbackProxy(t *testing.T, s3 *S3) {
	t.Helper()
	target, err := url.Parse(s3.Endpoint)
	if err != nil {
		t.Fatalf("Failed to parse test endpoint: %v", err)
	}
	ip := net.ParseIP(target.Hostname())
	if target.Scheme != "http" || (target.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback())) {
		t.Fatal("SSE-C test proxy requires an HTTP loopback endpoint")
	}
	// NewSingleHostReverseProxy preserves the incoming Host header, which
	// is required for S3 signature verification at the backend.
	proxy := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(target))
	t.Cleanup(proxy.Close)
	s3.SetEndpoint(proxy.URL).SetClient(proxy.Client())
}
