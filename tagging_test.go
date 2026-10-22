package simples3

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPutGetDeleteObjectTagging(t *testing.T) {
	s3 := setupTestS3(t)
	testBucket := os.Getenv("AWS_S3_BUCKET")
	timestamp := time.Now().Unix()
	testKey := fmt.Sprintf("test-tagging-%d.txt", timestamp)

	// Setup: upload a test file
	_, err := s3.FilePut(UploadInput{
		Bucket:      testBucket,
		ObjectKey:   testKey,
		ContentType: "text/plain",
		Body:        strings.NewReader("test content for tagging"),
	})
	if err != nil {
		t.Fatalf("Failed to upload test file: %v", err)
	}

	defer func() {
		s3.FileDelete(DeleteInput{Bucket: testBucket, ObjectKey: testKey})
	}()

	// Test 1: Put tags on the object
	t.Run("PutTags", func(t *testing.T) {
		err := s3.PutObjectTagging(PutObjectTaggingInput{
			Bucket:    testBucket,
			ObjectKey: testKey,
			Tags: map[string]string{
				"Environment": "test",
				"Project":     "simples3",
				"Version":     "v0.14.0",
			},
		})
		if err != nil {
			t.Fatalf("PutObjectTagging failed: %v", err)
		}
		t.Log("Successfully put tags on object")
	})

	// Test 2: Get tags from the object
	t.Run("GetTags", func(t *testing.T) {
		output, err := s3.GetObjectTagging(GetObjectTaggingInput{
			Bucket:    testBucket,
			ObjectKey: testKey,
		})
		if err != nil {
			t.Fatalf("GetObjectTagging failed: %v", err)
		}

		if len(output.Tags) != 3 {
			t.Errorf("Expected 3 tags, got %d", len(output.Tags))
		}

		expectedTags := map[string]string{
			"Environment": "test",
			"Project":     "simples3",
			"Version":     "v0.14.0",
		}

		for key, expectedValue := range expectedTags {
			if gotValue, ok := output.Tags[key]; !ok {
				t.Errorf("Expected tag %s not found", key)
			} else if gotValue != expectedValue {
				t.Errorf("Tag %s: expected %s, got %s", key, expectedValue, gotValue)
			}
		}

		t.Logf("Successfully retrieved tags: %+v", output.Tags)
	})

	// Test 3: Update tags (replace all existing tags)
	t.Run("UpdateTags", func(t *testing.T) {
		err := s3.PutObjectTagging(PutObjectTaggingInput{
			Bucket:    testBucket,
			ObjectKey: testKey,
			Tags: map[string]string{
				"Status": "updated",
			},
		})
		if err != nil {
			t.Fatalf("PutObjectTagging (update) failed: %v", err)
		}

		// Verify only new tag exists
		output, err := s3.GetObjectTagging(GetObjectTaggingInput{
			Bucket:    testBucket,
			ObjectKey: testKey,
		})
		if err != nil {
			t.Fatalf("GetObjectTagging failed: %v", err)
		}

		if len(output.Tags) != 1 {
			t.Errorf("Expected 1 tag after update, got %d", len(output.Tags))
		}

		if output.Tags["Status"] != "updated" {
			t.Errorf("Expected Status=updated, got %v", output.Tags)
		}

		t.Log("Successfully updated tags")
	})

	// Test 4: Delete all tags
	t.Run("DeleteTags", func(t *testing.T) {
		err := s3.DeleteObjectTagging(DeleteObjectTaggingInput{
			Bucket:    testBucket,
			ObjectKey: testKey,
		})
		if err != nil {
			t.Fatalf("DeleteObjectTagging failed: %v", err)
		}
		t.Log("Successfully deleted tags")

		// Verify tags are empty
		output, err := s3.GetObjectTagging(GetObjectTaggingInput{
			Bucket:    testBucket,
			ObjectKey: testKey,
		})
		if err != nil {
			t.Fatalf("GetObjectTagging after delete failed: %v", err)
		}

		if len(output.Tags) != 0 {
			t.Errorf("Expected 0 tags after deletion, got %d: %v", len(output.Tags), output.Tags)
		}
	})
}

func TestPutObjectTagging_Validation(t *testing.T) {
	s3 := setupTestS3(t)

	t.Run("EmptyBucket", func(t *testing.T) {
		err := s3.PutObjectTagging(PutObjectTaggingInput{
			Bucket:    "",
			ObjectKey: "key",
			Tags:      map[string]string{"key": "value"},
		})
		if err == nil {
			t.Error("Expected error for empty bucket")
		}
		if !strings.Contains(err.Error(), "bucket name is required") {
			t.Errorf("Expected 'bucket name is required' error, got: %v", err)
		}
	})

	t.Run("EmptyObjectKey", func(t *testing.T) {
		err := s3.PutObjectTagging(PutObjectTaggingInput{
			Bucket:    "bucket",
			ObjectKey: "",
			Tags:      map[string]string{"key": "value"},
		})
		if err == nil {
			t.Error("Expected error for empty object key")
		}
		if !strings.Contains(err.Error(), "object key is required") {
			t.Errorf("Expected 'object key is required' error, got: %v", err)
		}
	})

	t.Run("EmptyTags", func(t *testing.T) {
		err := s3.PutObjectTagging(PutObjectTaggingInput{
			Bucket:    "bucket",
			ObjectKey: "key",
			Tags:      map[string]string{},
		})
		if err == nil {
			t.Error("Expected error for empty tags")
		}
		if !strings.Contains(err.Error(), "at least one tag is required") {
			t.Errorf("Expected 'at least one tag is required' error, got: %v", err)
		}
	})

	t.Run("TooManyTags", func(t *testing.T) {
		tags := make(map[string]string)
		for i := 0; i < 11; i++ {
			tags[fmt.Sprintf("key%d", i)] = fmt.Sprintf("value%d", i)
		}

		err := s3.PutObjectTagging(PutObjectTaggingInput{
			Bucket:    "bucket",
			ObjectKey: "key",
			Tags:      tags,
		})
		if err == nil {
			t.Error("Expected error for >10 tags")
		}
		if !strings.Contains(err.Error(), "cannot set more than 10 tags per object") {
			t.Errorf("Expected '10 tags' error, got: %v", err)
		}
	})
}

func TestGetObjectTagging_Validation(t *testing.T) {
	s3 := setupTestS3(t)

	t.Run("EmptyBucket", func(t *testing.T) {
		_, err := s3.GetObjectTagging(GetObjectTaggingInput{
			Bucket:    "",
			ObjectKey: "key",
		})
		if err == nil {
			t.Error("Expected error for empty bucket")
		}
		if !strings.Contains(err.Error(), "bucket name is required") {
			t.Errorf("Expected 'bucket name is required' error, got: %v", err)
		}
	})

	t.Run("EmptyObjectKey", func(t *testing.T) {
		_, err := s3.GetObjectTagging(GetObjectTaggingInput{
			Bucket:    "bucket",
			ObjectKey: "",
		})
		if err == nil {
			t.Error("Expected error for empty object key")
		}
		if !strings.Contains(err.Error(), "object key is required") {
			t.Errorf("Expected 'object key is required' error, got: %v", err)
		}
	})
}

func TestDeleteObjectTagging_Validation(t *testing.T) {
	s3 := setupTestS3(t)

	t.Run("EmptyBucket", func(t *testing.T) {
		err := s3.DeleteObjectTagging(DeleteObjectTaggingInput{
			Bucket:    "",
			ObjectKey: "key",
		})
		if err == nil {
			t.Error("Expected error for empty bucket")
		}
		if !strings.Contains(err.Error(), "bucket name is required") {
			t.Errorf("Expected 'bucket name is required' error, got: %v", err)
		}
	})

	t.Run("EmptyObjectKey", func(t *testing.T) {
		err := s3.DeleteObjectTagging(DeleteObjectTaggingInput{
			Bucket:    "bucket",
			ObjectKey: "",
		})
		if err == nil {
			t.Error("Expected error for empty object key")
		}
		if !strings.Contains(err.Error(), "object key is required") {
			t.Errorf("Expected 'object key is required' error, got: %v", err)
		}
	})
}

func TestGetObjectTagging_NonExistent(t *testing.T) {
	s3 := setupTestS3(t)
	testBucket := os.Getenv("AWS_S3_BUCKET")

	// Try to get tags from non-existent object
	_, err := s3.GetObjectTagging(GetObjectTaggingInput{
		Bucket:    testBucket,
		ObjectKey: "non-existent-object-12345.txt",
	})
	if err == nil {
		t.Error("Expected error for non-existent object")
	}
	t.Logf("Got expected error for non-existent object: %v", err)
}

func TestFilePut_WithTags(t *testing.T) {
	s3 := setupTestS3(t)
	testBucket := os.Getenv("AWS_S3_BUCKET")
	timestamp := time.Now().Unix()
	testKey := fmt.Sprintf("test-put-with-tags-%d.txt", timestamp)

	// Upload with tags using FilePut
	_, err := s3.FilePut(UploadInput{
		Bucket:      testBucket,
		ObjectKey:   testKey,
		ContentType: "text/plain",
		Body:        strings.NewReader("test content with tags"),
		Tags: map[string]string{
			"Method":      "FilePut",
			"Environment": "test",
		},
	})
	if err != nil {
		t.Fatalf("FilePut with tags failed: %v", err)
	}

	defer s3.FileDelete(DeleteInput{Bucket: testBucket, ObjectKey: testKey})

	// Verify tags were set
	output, err := s3.GetObjectTagging(GetObjectTaggingInput{
		Bucket:    testBucket,
		ObjectKey: testKey,
	})
	if err != nil {
		t.Fatalf("GetObjectTagging failed: %v", err)
	}

	if len(output.Tags) != 2 {
		t.Errorf("Expected 2 tags, got %d", len(output.Tags))
	}

	if output.Tags["Method"] != "FilePut" {
		t.Errorf("Expected Method=FilePut, got %v", output.Tags["Method"])
	}
	if output.Tags["Environment"] != "test" {
		t.Errorf("Expected Environment=test, got %v", output.Tags["Environment"])
	}

	t.Logf("Successfully uploaded with tags via FilePut: %+v", output.Tags)
}

func TestFileUpload_WithTags(t *testing.T) {
	// Skip: MinIO doesn't support x-amz-tagging in POST upload policies
	// This functionality works on AWS S3.
	t.Skip("MinIO limitation: tagging via POST upload not fully supported")

	s3 := setupTestS3(t)
	testBucket := os.Getenv("AWS_S3_BUCKET")
	timestamp := time.Now().Unix()
	testKey := fmt.Sprintf("test-upload-with-tags-%d.txt", timestamp)

	// Upload with tags using FileUpload
	_, err := s3.FileUpload(UploadInput{
		Bucket:      testBucket,
		ObjectKey:   testKey,
		FileName:    "test.txt",
		ContentType: "text/plain",
		Body:        strings.NewReader("test content with tags via upload"),
		Tags: map[string]string{
			"Method": "FileUpload",
			"Type":   "POST",
		},
	})
	if err != nil {
		t.Fatalf("FileUpload with tags failed: %v", err)
	}

	defer s3.FileDelete(DeleteInput{Bucket: testBucket, ObjectKey: testKey})

	// Verify tags were set
	output, err := s3.GetObjectTagging(GetObjectTaggingInput{
		Bucket:    testBucket,
		ObjectKey: testKey,
	})
	if err != nil {
		t.Fatalf("GetObjectTagging failed: %v", err)
	}

	if len(output.Tags) != 2 {
		t.Errorf("Expected 2 tags, got %d", len(output.Tags))
	}

	if output.Tags["Method"] != "FileUpload" {
		t.Errorf("Expected Method=FileUpload, got %v", output.Tags["Method"])
	}
	if output.Tags["Type"] != "POST" {
		t.Errorf("Expected Type=POST, got %v", output.Tags["Type"])
	}

	t.Logf("Successfully uploaded with tags via FileUpload: %+v", output.Tags)
}

func TestCopyObject_WithTags(t *testing.T) {
	s3 := setupTestS3(t)
	testBucket := os.Getenv("AWS_S3_BUCKET")
	timestamp := time.Now().Unix()
	sourceKey := fmt.Sprintf("test-copy-source-tags-%d.txt", timestamp)
	destKey := fmt.Sprintf("test-copy-dest-tags-%d.txt", timestamp)

	// Setup: upload source file with tags
	_, err := s3.FilePut(UploadInput{
		Bucket:      testBucket,
		ObjectKey:   sourceKey,
		ContentType: "text/plain",
		Body:        strings.NewReader("test content for copy with tags"),
		Tags: map[string]string{
			"Original": "true",
		},
	})
	if err != nil {
		t.Fatalf("Failed to upload source file: %v", err)
	}

	defer func() {
		s3.FileDelete(DeleteInput{Bucket: testBucket, ObjectKey: sourceKey})
		s3.FileDelete(DeleteInput{Bucket: testBucket, ObjectKey: destKey})
	}()

	// Copy with new tags (should replace source tags)
	_, err = s3.CopyObject(CopyObjectInput{
		SourceBucket: testBucket,
		SourceKey:    sourceKey,
		DestBucket:   testBucket,
		DestKey:      destKey,
		Tags: map[string]string{
			"Method":  "CopyObject",
			"Copied":  "true",
			"Version": "v1",
		},
	})
	if err != nil {
		t.Fatalf("CopyObject with tags failed: %v", err)
	}

	// Verify destination has new tags (not source tags)
	output, err := s3.GetObjectTagging(GetObjectTaggingInput{
		Bucket:    testBucket,
		ObjectKey: destKey,
	})
	if err != nil {
		t.Fatalf("GetObjectTagging failed: %v", err)
	}

	if len(output.Tags) != 3 {
		t.Errorf("Expected 3 tags, got %d: %+v", len(output.Tags), output.Tags)
	}

	if output.Tags["Method"] != "CopyObject" {
		t.Errorf("Expected Method=CopyObject, got %v", output.Tags["Method"])
	}
	if output.Tags["Copied"] != "true" {
		t.Errorf("Expected Copied=true, got %v", output.Tags["Copied"])
	}
	if output.Tags["Original"] != "" {
		t.Errorf("Expected original tags to be replaced, but found Original tag: %v", output.Tags["Original"])
	}

	t.Logf("Successfully copied with new tags: %+v", output.Tags)
}
