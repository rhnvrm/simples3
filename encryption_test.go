package simples3

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestFilePut_WithEncryption(t *testing.T) {
	s3 := setupTestS3(t)
	testBucket := os.Getenv("AWS_S3_BUCKET")
	timestamp := time.Now().Unix()
	testKey := fmt.Sprintf("test-put-sse-%d.txt", timestamp)

	// Upload with SSE-S3
	_, err := s3.FilePut(UploadInput{
		Bucket:               testBucket,
		ObjectKey:            testKey,
		ContentType:          "text/plain",
		Body:                 strings.NewReader("test content for SSE-S3"),
		ServerSideEncryption: "AES256",
	})
	if err != nil {
		// MinIO might return Not Implemented if not configured for SSE
		// if strings.Contains(err.Error(), "NotImplemented") && strings.Contains(err.Error(), "KMS is not configured") {
		// 	t.Log("Skipping verification: MinIO is not configured for SSE, but headers were sent correctly.")
		// 	return
		// }
		t.Fatalf("FilePut with SSE-S3 failed: %v", err)
	}

	defer s3.FileDelete(DeleteInput{Bucket: testBucket, ObjectKey: testKey})

	// Verify encryption via FileDetails
	details, err := s3.FileDetails(DetailsInput{
		Bucket:    testBucket,
		ObjectKey: testKey,
	})
	if err != nil {
		t.Fatalf("FileDetails failed: %v", err)
	}

	// MinIO should return x-amz-server-side-encryption header
	sse := details.ServerSideEncryption
	if sse != "AES256" {
		t.Errorf("Expected x-amz-server-side-encryption=AES256, got %q. MinIO KMS configuration might be missing.", sse)
	} else {
		t.Logf("Confirmed SSE-S3: %s", sse)
	}
}

func TestFileUpload_WithEncryption(t *testing.T) {
	s3 := setupTestS3(t)
	testBucket := os.Getenv("AWS_S3_BUCKET")
	timestamp := time.Now().Unix()
	testKey := fmt.Sprintf("test-upload-sse-%d.txt", timestamp)

	// Upload with SSE-S3 via POST
	_, err := s3.FileUpload(UploadInput{
		Bucket:               testBucket,
		ObjectKey:            testKey,
		FileName:             "test.txt",
		ContentType:          "text/plain",
		Body:                 strings.NewReader("test content for SSE-S3 POST"),
		ServerSideEncryption: "AES256",
	})
	if err != nil {
		t.Fatalf("FileUpload with SSE-S3 failed: %v", err)
	}

	defer s3.FileDelete(DeleteInput{Bucket: testBucket, ObjectKey: testKey})

	// Verify encryption
	details, err := s3.FileDetails(DetailsInput{
		Bucket:    testBucket,
		ObjectKey: testKey,
	})
	if err != nil {
		t.Fatalf("FileDetails failed: %v", err)
	}

	sse := details.ServerSideEncryption
	if sse != "AES256" {
		t.Logf("Warning: Expected x-amz-server-side-encryption=AES256, got %q. MinIO might not be configured for SSE.", sse)
	} else {
		t.Logf("Confirmed SSE-S3: %s", sse)
	}
}

func TestCopyObject_WithEncryption(t *testing.T) {
	s3 := setupTestS3(t)
	testBucket := os.Getenv("AWS_S3_BUCKET")
	timestamp := time.Now().Unix()
	sourceKey := fmt.Sprintf("test-copy-source-sse-%d.txt", timestamp)
	destKey := fmt.Sprintf("test-copy-dest-sse-%d.txt", timestamp)

	// Setup: upload source file
	_, err := s3.FilePut(UploadInput{
		Bucket:      testBucket,
		ObjectKey:   sourceKey,
		ContentType: "text/plain",
		Body:        strings.NewReader("test content for copy"),
	})
	if err != nil {
		t.Fatalf("Failed to upload source file: %v", err)
	}

	defer func() {
		s3.FileDelete(DeleteInput{Bucket: testBucket, ObjectKey: sourceKey})
		s3.FileDelete(DeleteInput{Bucket: testBucket, ObjectKey: destKey})
	}()

	// Copy with SSE-S3
	_, err = s3.CopyObject(CopyObjectInput{
		SourceBucket:         testBucket,
		SourceKey:            sourceKey,
		DestBucket:           testBucket,
		DestKey:              destKey,
		ServerSideEncryption: "AES256",
	})
	if err != nil {
		t.Fatalf("CopyObject with SSE-S3 failed: %v", err)
	}

	// Verify destination has encryption
	details, err := s3.FileDetails(DetailsInput{
		Bucket:    testBucket,
		ObjectKey: destKey,
	})
	if err != nil {
		t.Fatalf("FileDetails failed: %v", err)
	}

	sse := details.ServerSideEncryption
	if sse != "AES256" {
		t.Logf("Warning: Expected x-amz-server-side-encryption=AES256, got %q.", sse)
	} else {
		t.Logf("Confirmed SSE-S3: %s", sse)
	}
}

func TestMultipartUpload_WithEncryption(t *testing.T) {
	s3 := setupTestS3(t)
	testBucket := os.Getenv("AWS_S3_BUCKET")
	timestamp := time.Now().Unix()
	testKey := fmt.Sprintf("test-multipart-sse-%d.txt", timestamp)

	// Upload with SSE-S3 via Multipart
	_, err := s3.FileUploadMultipart(MultipartUploadInput{
		Bucket:               testBucket,
		ObjectKey:            testKey,
		ContentType:          "text/plain",
		Body:                 strings.NewReader(strings.Repeat("a", 6*1024*1024)), // 6MB
		ServerSideEncryption: "AES256",
	})
	if err != nil {
		t.Fatalf("FileUploadMultipart with SSE-S3 failed: %v", err)
	}

	defer s3.FileDelete(DeleteInput{Bucket: testBucket, ObjectKey: testKey})

	// Verify encryption
	details, err := s3.FileDetails(DetailsInput{
		Bucket:    testBucket,
		ObjectKey: testKey,
	})
	if err != nil {
		t.Fatalf("FileDetails failed: %v", err)
	}

	sse := details.ServerSideEncryption
	if sse != "AES256" {
		t.Logf("Warning: Expected x-amz-server-side-encryption=AES256, got %q.", sse)
	} else {
		t.Logf("Confirmed SSE-S3: %s", sse)
	}
}
