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
	tests := []struct {
		name   string
		s3     *S3
		bucket string
		params []string
		want   string
	}{
		// Path-style tests
		{
			name:   "path-style: basic test",
			s3:     New("us-east-1", "AccessKey", "SuperSecretKey"),
			bucket: "xyz",
			want:   "https://s3.us-east-1.amazonaws.com/xyz",
		},
		{
			name:   "path-style: multiple parameters",
			s3:     New("us-east-1", "AccessKey", "SuperSecretKey"),
			bucket: "xyz",
			params: []string{"hello", "world"},
			want:   "https://s3.us-east-1.amazonaws.com/xyz/hello/world",
		},
		{
			name:   "path-style: special characters",
			s3:     New("us-east-1", "AccessKey", "SuperSecretKey"),
			bucket: "xyz",
			params: []string{"hello, world!", "#!@$%^&*(1).txt"},
			want:   "https://s3.us-east-1.amazonaws.com/xyz/hello%2C%20world%21/%23%21%40%24%25%5E%26%2A%281%29.txt",
		},
		{
			name:   "path-style: with custom endpoint",
			s3:     New("us-east-1", "AccessKey", "SuperSecretKey").SetEndpoint("https://s3.example.com"),
			bucket: "mybucket",
			params: []string{"mykey.txt"},
			want:   "https://s3.example.com/mybucket/mykey.txt",
		},
		{
			name:   "path-style: unicode characters",
			s3:     New("us-east-1", "AccessKey", "SuperSecretKey"),
			bucket: "mybucket",
			params: []string{"文件.txt"},
			want:   "https://s3.us-east-1.amazonaws.com/mybucket/%E6%96%87%E4%BB%B6.txt",
		},
		// Virtual-hosted style tests
		{
			name:   "virtual-hosted: basic test with endpoint",
			s3:     New("us-east-1", "AccessKey", "SuperSecretKey").SetEndpoint("https://mybucket.s3.us-east-1.amazonaws.com").SetVirtualHostedStyle(),
			bucket: "",
			params: []string{"mykey.txt"},
			want:   "https://mybucket.s3.us-east-1.amazonaws.com/mykey.txt",
		},
		{
			name:   "virtual-hosted: nested path",
			s3:     New("us-east-1", "AccessKey", "SuperSecretKey").SetEndpoint("https://mybucket.s3.us-east-1.amazonaws.com").SetVirtualHostedStyle(),
			bucket: "",
			params: []string{"folder", "subfolder", "file.txt"},
			want:   "https://mybucket.s3.us-east-1.amazonaws.com/folder/subfolder/file.txt",
		},
		{
			name:   "virtual-hosted: special characters in key",
			s3:     New("us-east-1", "AccessKey", "SuperSecretKey").SetEndpoint("https://mybucket.s3.us-east-1.amazonaws.com").SetVirtualHostedStyle(),
			bucket: "",
			params: []string{"hello world.txt"},
			want:   "https://mybucket.s3.us-east-1.amazonaws.com/hello%20world.txt",
		},
		{
			name:   "virtual-hosted: unicode characters",
			s3:     New("us-east-1", "AccessKey", "SuperSecretKey").SetEndpoint("https://mybucket.s3.us-east-1.amazonaws.com").SetVirtualHostedStyle(),
			bucket: "",
			params: []string{"文件.txt"},
			want:   "https://mybucket.s3.us-east-1.amazonaws.com/%E6%96%87%E4%BB%B6.txt",
		},
		{
			name:   "virtual-hosted: empty key",
			s3:     New("us-east-1", "AccessKey", "SuperSecretKey").SetEndpoint("https://mybucket.s3.us-east-1.amazonaws.com").SetVirtualHostedStyle(),
			bucket: "",
			params: nil,
			want:   "https://mybucket.s3.us-east-1.amazonaws.com",
		},
		{
			name:   "virtual-hosted: with custom S3-compatible endpoint",
			s3:     New("us-east-1", "AccessKey", "SuperSecretKey").SetEndpoint("https://mybucket.minio.example.com:9000").SetVirtualHostedStyle(),
			bucket: "",
			params: []string{"data", "report.pdf"},
			want:   "https://mybucket.minio.example.com:9000/data/report.pdf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := tt.s3.getURL(tt.bucket, tt.params...)
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
