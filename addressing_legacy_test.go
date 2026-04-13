package simples3

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestGeneratePresignedURL_LegacyCustomEndpointUsesPathStyle(t *testing.T) {
	timestamp, err := time.Parse(time.RFC1123, "Fri, 24 May 2013 00:00:00 GMT")
	if err != nil {
		t.Fatalf("time.Parse() error = %v", err)
	}

	s3 := New(
		"us-east-1",
		"AKIAIOSFODNN7EXAMPLE",
		"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
	)
	s3.SetEndpoint("https://objects.example.com/base")

	presignedURL := s3.GeneratePresignedURL(PresignedInput{
		Bucket:        "examplebucket",
		ObjectKey:     "test.txt",
		Method:        "GET",
		Timestamp:     timestamp,
		ExpirySeconds: 86400,
	})

	parsed, err := url.Parse(presignedURL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	if parsed.Scheme != "https" {
		t.Fatalf("scheme = %q, want https", parsed.Scheme)
	}
	if parsed.Host != "objects.example.com" {
		t.Fatalf("host = %q, want objects.example.com", parsed.Host)
	}
	if parsed.EscapedPath() != "/base/examplebucket/test.txt" {
		t.Fatalf("path = %q, want /base/examplebucket/test.txt", parsed.EscapedPath())
	}
	if strings.HasPrefix(parsed.Host, "examplebucket.") {
		t.Fatalf("legacy custom endpoint should keep bucket in path, got host %q", parsed.Host)
	}
}

func TestGeneratePresignedUploadPartURL_LegacyCustomEndpointUsesPathStyle(t *testing.T) {
	s3 := New("us-east-1", "AccessKey", "SuperSecretKey")
	s3.SetEndpoint("https://objects.example.com/base")

	presignedURL := s3.GeneratePresignedUploadPartURL(PresignedMultipartInput{
		Bucket:        "examplebucket",
		ObjectKey:     "test.txt",
		UploadID:      "upload-id",
		PartNumber:    7,
		ExpirySeconds: 3600,
	})

	parsed, err := url.Parse(presignedURL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	if parsed.Host != "objects.example.com" {
		t.Fatalf("host = %q, want objects.example.com", parsed.Host)
	}
	if parsed.EscapedPath() != "/base/examplebucket/test.txt" {
		t.Fatalf("path = %q, want /base/examplebucket/test.txt", parsed.EscapedPath())
	}
	if got := parsed.Query().Get("partNumber"); got != "7" {
		t.Fatalf("partNumber = %q, want 7", got)
	}
	if got := parsed.Query().Get("uploadId"); got != "upload-id" {
		t.Fatalf("uploadId = %q, want upload-id", got)
	}
}

func TestCreateUploadPolicies_LegacyDefaultUploadURL(t *testing.T) {
	s3 := New("us-east-1", "AccessKey", "SuperSecretKey")

	policies, err := s3.CreateUploadPolicies(UploadConfig{
		BucketName:  "examplebucket",
		ObjectKey:   "test.txt",
		ContentType: "text/plain",
		FileSize:    123,
	})
	if err != nil {
		t.Fatalf("CreateUploadPolicies() error = %v", err)
	}

	if policies.URL != "http://examplebucket.s3.amazonaws.com/" {
		t.Fatalf("URL = %q, want http://examplebucket.s3.amazonaws.com/", policies.URL)
	}
	if got := policies.Form["key"]; got != "test.txt" {
		t.Fatalf("form[key] = %q, want test.txt", got)
	}
	if got := policies.Form["Content-Type"]; got != "text/plain" {
		t.Fatalf("form[Content-Type] = %q, want text/plain", got)
	}
}
