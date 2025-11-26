package simples3

import (
	"strings"
	"testing"
)

type tConfig struct {
	AccessKey string
	SecretKey string
	Endpoint  string
	Region    string
}

func TestCustomEndpoint(t *testing.T) {
	s3 := New("us-east-1", "AccessKey", "SuperSecretKey")

	// no protocol specified, should default to https
	s3.SetEndpoint("example.com")
	if s3.getURL("bucket1") != "https://example.com/bucket1" {
		t.Errorf("S3.SetEndpoint() got = %v", s3.Endpoint)
	}

	// explicit http protocol
	s3.SetEndpoint("http://localhost:9000")
	if s3.getURL("bucket2") != "http://localhost:9000/bucket2" {
		t.Errorf("S3.SetEndpoint() got = %v", s3.Endpoint)
	}

	// explicit http protocol
	s3.SetEndpoint("https://example.com")
	if s3.getURL("bucket3") != "https://example.com/bucket3" {
		t.Errorf("S3.SetEndpoint() got = %v", s3.Endpoint)
	}

	// try with trailing slash
	s3.SetEndpoint("https://example.com/foobar/")
	if s3.getURL("bucket4") != "https://example.com/foobar/bucket4" {
		t.Errorf("S3.SetEndpoint() got = %v", s3.Endpoint)
	}
}

func TestGetURL(t *testing.T) {
	s3 := New("us-east-1", "AccessKey", "SuperSecretKey")

	type args struct {
		bucket string
		params []string
	}

	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "getURL: basic test",
			args: args{
				bucket: "xyz",
			},
			want: "https://s3.us-east-1.amazonaws.com/xyz",
		},
		{
			name: "getURL: multiple parameters",
			args: args{
				bucket: "xyz",
				params: []string{"hello", "world"},
			},
			want: "https://s3.us-east-1.amazonaws.com/xyz/hello/world",
		},
		{
			name: "getURL: special characters",
			args: args{
				bucket: "xyz",
				params: []string{"hello, world!", "#!@$%^&*(1).txt"},
			},
			want: "https://s3.us-east-1.amazonaws.com/xyz/hello%2C%20world%21/%23%21%40%24%25%5E%26%2A%281%29.txt",
		},
	}

	for _, testcase := range tests {
		tt := testcase
		t.Run(tt.name, func(t *testing.T) {
			url := s3.getURL(tt.args.bucket, tt.args.params...)
			if url != tt.want {
				t.Errorf("S3.getURL() got = %v, want %v", url, tt.want)
			}
		})
	}
}

// Helper functions
func uploadTestFiles(t *testing.T, s3 *S3, bucket string, filenames []string) {
	for _, filename := range filenames {
		content := strings.NewReader("test content for " + filename)
		_, err := s3.FilePut(UploadInput{
			Bucket:      bucket,
			ObjectKey:   filename,
			ContentType: "text/plain",
			Body:        content,
		})
		if err != nil {
			t.Fatalf("Failed to upload test file %s: %v", filename, err)
		}
	}
}

func cleanupTestFiles(t *testing.T, s3 *S3, bucket string, filenames []string) {
	for _, filename := range filenames {
		err := s3.FileDelete(DeleteInput{
			Bucket:    bucket,
			ObjectKey: filename,
		})
		if err != nil {
			t.Logf("Failed to cleanup test file %s: %v", filename, err)
		}
	}
}
