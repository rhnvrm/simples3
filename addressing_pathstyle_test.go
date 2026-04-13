package simples3

import (
	"net/url"
	"testing"
	"time"
)

func TestSetUsePathStyle_PathStyleRuntime(t *testing.T) {
	s3 := New("us-west-2", "AccessKey", "SuperSecretKey").SetUsePathStyle(true)

	got := s3.getURL("examplebucket", "photos/puppy.jpg")
	want := "https://s3.us-west-2.amazonaws.com/examplebucket/photos/puppy.jpg"
	if got != want {
		t.Fatalf("getURL() = %q, want %q", got, want)
	}
}

func TestGeneratePresignedURL_UsePathStyleTrueUsesPathStyle(t *testing.T) {
	timestamp, err := time.Parse(time.RFC1123, "Fri, 24 May 2013 00:00:00 GMT")
	if err != nil {
		t.Fatalf("time.Parse() error = %v", err)
	}

	s3 := New(
		"us-west-2",
		"AKIAIOSFODNN7EXAMPLE",
		"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
	).SetUsePathStyle(true)

	presignedURL := s3.GeneratePresignedURL(PresignedInput{
		Bucket:        "examplebucket",
		ObjectKey:     "photos/puppy.jpg",
		Method:        "GET",
		Timestamp:     timestamp,
		ExpirySeconds: 3600,
	})

	parsed, err := url.Parse(presignedURL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	if parsed.Host != "s3.us-west-2.amazonaws.com" {
		t.Fatalf("host = %q, want s3.us-west-2.amazonaws.com", parsed.Host)
	}
	if parsed.EscapedPath() != "/examplebucket/photos/puppy.jpg" {
		t.Fatalf("path = %q, want /examplebucket/photos/puppy.jpg", parsed.EscapedPath())
	}
	if parsed.Query().Get("X-Amz-Signature") == "" {
		t.Fatalf("missing X-Amz-Signature in %q", presignedURL)
	}
}

func TestGeneratePresignedUploadPartURL_UsePathStyleTrueUsesPathStyle(t *testing.T) {
	s3 := New("us-west-2", "AccessKey", "SuperSecretKey").SetUsePathStyle(true)

	presignedURL := s3.GeneratePresignedUploadPartURL(PresignedMultipartInput{
		Bucket:        "examplebucket",
		ObjectKey:     "multipart/test.bin",
		UploadID:      "upload-id",
		PartNumber:    7,
		ExpirySeconds: 3600,
	})

	parsed, err := url.Parse(presignedURL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	if parsed.Host != "s3.us-west-2.amazonaws.com" {
		t.Fatalf("host = %q, want s3.us-west-2.amazonaws.com", parsed.Host)
	}
	if parsed.EscapedPath() != "/examplebucket/multipart/test.bin" {
		t.Fatalf("path = %q, want /examplebucket/multipart/test.bin", parsed.EscapedPath())
	}
	if got := parsed.Query().Get("partNumber"); got != "7" {
		t.Fatalf("partNumber = %q, want 7", got)
	}
	if got := parsed.Query().Get("uploadId"); got != "upload-id" {
		t.Fatalf("uploadId = %q, want upload-id", got)
	}
}
