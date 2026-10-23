package simples3

import (
	"net/url"
	"testing"
	"time"
)

func TestSetUsePathStyle_VirtualHostedStyleRuntime(t *testing.T) {
	s3 := New("us-west-2", "AccessKey", "SuperSecretKey").SetUsePathStyle(false)

	got := s3.getURL("examplebucket", "photos/puppy.jpg")
	want := "https://examplebucket.s3.us-west-2.amazonaws.com/photos/puppy.jpg"
	if got != want {
		t.Fatalf("getURL() = %q, want %q", got, want)
	}
}

func TestSetUsePathStyle_VirtualHostedStyleRuntime_CustomEndpoint(t *testing.T) {
	s3 := New("nyc3", "AccessKey", "SuperSecretKey")
	s3.SetEndpoint("https://objects.example.com")
	s3.SetUsePathStyle(false)

	got := s3.getURL("examplebucket", "photos/puppy.jpg")
	want := "https://examplebucket.objects.example.com/photos/puppy.jpg"
	if got != want {
		t.Fatalf("getURL() = %q, want %q", got, want)
	}
}

func TestSetUsePathStyle_VirtualHostedStyleRuntimeFallbacks(t *testing.T) {
	tests := []struct {
		name     string
		bucket   string
		endpoint string
		want     string
	}{
		{
			name:   "dotted bucket over https",
			bucket: "example.bucket",
			want:   "https://s3.us-west-2.amazonaws.com/example.bucket/photos/puppy.jpg",
		},
		{
			name:   "non dns bucket",
			bucket: "example_bucket",
			want:   "https://s3.us-west-2.amazonaws.com/example_bucket/photos/puppy.jpg",
		},
		{
			name:   "invalid dotted label leading hyphen",
			bucket: "a.-b",
			want:   "https://s3.us-west-2.amazonaws.com/a.-b/photos/puppy.jpg",
		},
		{
			name:   "invalid dotted label trailing hyphen",
			bucket: "a-.b",
			want:   "https://s3.us-west-2.amazonaws.com/a-.b/photos/puppy.jpg",
		},
		{
			name:     "localhost endpoint",
			bucket:   "examplebucket",
			endpoint: "http://localhost:9000",
			want:     "http://localhost:9000/examplebucket/photos/puppy.jpg",
		},
		{
			name:     "path prefixed endpoint",
			bucket:   "examplebucket",
			endpoint: "https://objects.example.com/base",
			want:     "https://objects.example.com/base/examplebucket/photos/puppy.jpg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s3 := New("us-west-2", "AccessKey", "SuperSecretKey")
			if tt.endpoint != "" {
				s3.SetEndpoint(tt.endpoint)
			}
			s3.SetUsePathStyle(false)

			if got := s3.getURL(tt.bucket, "photos/puppy.jpg"); got != tt.want {
				t.Fatalf("getURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGeneratePresignedURL_UsePathStyleFalseUsesVirtualHostedStyle(t *testing.T) {
	timestamp, err := time.Parse(time.RFC1123, "Fri, 24 May 2013 00:00:00 GMT")
	if err != nil {
		t.Fatalf("time.Parse() error = %v", err)
	}

	s3 := New(
		"us-west-2",
		"AKIAIOSFODNN7EXAMPLE",
		"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
	).SetUsePathStyle(false)

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

	if parsed.Host != "examplebucket.s3.us-west-2.amazonaws.com" {
		t.Fatalf("host = %q, want examplebucket.s3.us-west-2.amazonaws.com", parsed.Host)
	}
	if parsed.EscapedPath() != "/photos/puppy.jpg" {
		t.Fatalf("path = %q, want /photos/puppy.jpg", parsed.EscapedPath())
	}
	if parsed.Query().Get("X-Amz-Signature") == "" {
		t.Fatalf("missing X-Amz-Signature in %q", presignedURL)
	}
}

func TestGeneratePresignedURL_UsePathStyleFalseFallbacksToPathStyle(t *testing.T) {
	timestamp, err := time.Parse(time.RFC1123, "Fri, 24 May 2013 00:00:00 GMT")
	if err != nil {
		t.Fatalf("time.Parse() error = %v", err)
	}

	tests := []struct {
		name     string
		bucket   string
		endpoint string
		wantHost string
		wantPath string
	}{
		{
			name:     "dotted bucket over https",
			bucket:   "example.bucket",
			wantHost: "s3.us-west-2.amazonaws.com",
			wantPath: "/example.bucket/photos/puppy.jpg",
		},
		{
			name:     "path prefixed endpoint",
			bucket:   "examplebucket",
			endpoint: "https://objects.example.com/base",
			wantHost: "objects.example.com",
			wantPath: "/base/examplebucket/photos/puppy.jpg",
		},
		{
			name:     "invalid dotted label leading hyphen",
			bucket:   "a.-b",
			wantHost: "s3.us-west-2.amazonaws.com",
			wantPath: "/a.-b/photos/puppy.jpg",
		},
		{
			name:     "invalid dotted label trailing hyphen",
			bucket:   "a-.b",
			wantHost: "s3.us-west-2.amazonaws.com",
			wantPath: "/a-.b/photos/puppy.jpg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s3 := New(
				"us-west-2",
				"AKIAIOSFODNN7EXAMPLE",
				"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			)
			if tt.endpoint != "" {
				s3.SetEndpoint(tt.endpoint)
			}
			s3.SetUsePathStyle(false)

			presignedURL := s3.GeneratePresignedURL(PresignedInput{
				Bucket:        tt.bucket,
				ObjectKey:     "photos/puppy.jpg",
				Method:        "GET",
				Timestamp:     timestamp,
				ExpirySeconds: 3600,
			})

			parsed, err := url.Parse(presignedURL)
			if err != nil {
				t.Fatalf("url.Parse() error = %v", err)
			}
			if parsed.Host != tt.wantHost {
				t.Fatalf("host = %q, want %q", parsed.Host, tt.wantHost)
			}
			if parsed.EscapedPath() != tt.wantPath {
				t.Fatalf("path = %q, want %q", parsed.EscapedPath(), tt.wantPath)
			}
		})
	}
}

func TestGeneratePresignedUploadPartURL_UsePathStyleFalseUsesVirtualHostedStyle(t *testing.T) {
	s3 := New("us-west-2", "AccessKey", "SuperSecretKey").SetUsePathStyle(false)

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

	if parsed.Host != "examplebucket.s3.us-west-2.amazonaws.com" {
		t.Fatalf("host = %q, want examplebucket.s3.us-west-2.amazonaws.com", parsed.Host)
	}
	if parsed.EscapedPath() != "/multipart/test.bin" {
		t.Fatalf("path = %q, want /multipart/test.bin", parsed.EscapedPath())
	}
	if got := parsed.Query().Get("partNumber"); got != "7" {
		t.Fatalf("partNumber = %q, want 7", got)
	}
	if got := parsed.Query().Get("uploadId"); got != "upload-id" {
		t.Fatalf("uploadId = %q, want upload-id", got)
	}
}
