package simples3

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func setupTestS3(t *testing.T) *S3 {
	t.Helper()

	s3 := New(
		os.Getenv("AWS_S3_REGION"),
		os.Getenv("AWS_S3_ACCESS_KEY"),
		os.Getenv("AWS_S3_SECRET_KEY"),
	)

	if endpoint := os.Getenv("AWS_S3_ENDPOINT"); endpoint != "" {
		s3.SetEndpoint(endpoint)
	}

	return s3
}

func TestListBuckets(t *testing.T) {
	s3 := setupTestS3(t)

	result, err := s3.ListBuckets(ListBucketsInput{})
	if err != nil {
		t.Fatalf("ListBuckets failed: %v", err)
	}

	// ListBuckets should succeed even if there are no buckets
	t.Logf("Found %d bucket(s)", len(result.Buckets))
	for _, bucket := range result.Buckets {
		t.Logf("  - %s (created: %s)", bucket.Name, bucket.CreationDate)
	}

	// Verify Owner information is present
	if result.Owner.ID != "" {
		t.Logf("Owner ID: %s", result.Owner.ID)
	}
	if result.Owner.DisplayName != "" {
		t.Logf("Owner DisplayName: %s", result.Owner.DisplayName)
	}
}

func TestCreateAndDeleteBucket(t *testing.T) {
	s3 := setupTestS3(t)

	// Generate a unique bucket name using timestamp
	testBucket := fmt.Sprintf("simples3-test-%d", time.Now().Unix())

	t.Run("CreateBucket", func(t *testing.T) {
		output, err := s3.CreateBucket(CreateBucketInput{
			Bucket: testBucket,
		})
		if err != nil {
			t.Fatalf("CreateBucket failed: %v", err)
		}
		t.Logf("Created bucket: %s", testBucket)
		if output.Location != "" {
			t.Logf("Location: %s", output.Location)
		}
	})

	// Give S3/MinIO a moment to propagate the bucket creation
	time.Sleep(100 * time.Millisecond)

	t.Run("VerifyBucketExists", func(t *testing.T) {
		result, err := s3.ListBuckets(ListBucketsInput{})
		if err != nil {
			t.Fatalf("ListBuckets failed: %v", err)
		}

		found := false
		for _, bucket := range result.Buckets {
			if bucket.Name == testBucket {
				found = true
				t.Logf("Verified bucket exists: %s (created: %s)", bucket.Name, bucket.CreationDate)
				break
			}
		}

		if !found {
			t.Errorf("Created bucket %s not found in ListBuckets", testBucket)
		}
	})

	t.Run("DeleteBucket", func(t *testing.T) {
		err := s3.DeleteBucket(DeleteBucketInput{
			Bucket: testBucket,
		})
		if err != nil {
			t.Fatalf("DeleteBucket failed: %v", err)
		}
		t.Logf("Deleted bucket: %s", testBucket)
	})

	// Give S3/MinIO a moment to propagate the bucket deletion
	time.Sleep(100 * time.Millisecond)

	t.Run("VerifyBucketDeleted", func(t *testing.T) {
		result, err := s3.ListBuckets(ListBucketsInput{})
		if err != nil {
			t.Fatalf("ListBuckets failed: %v", err)
		}

		found := false
		for _, bucket := range result.Buckets {
			if bucket.Name == testBucket {
				found = true
				break
			}
		}

		if found {
			t.Errorf("Deleted bucket %s still appears in ListBuckets", testBucket)
		} else {
			t.Logf("Verified bucket deleted: %s", testBucket)
		}
	})
}

func TestCreateBucket_WithRegion(t *testing.T) {
	s3 := setupTestS3(t)

	// Generate a unique bucket name
	testBucket := fmt.Sprintf("simples3-test-region-%d", time.Now().Unix())

	// Create bucket with explicit region
	region := os.Getenv("AWS_S3_REGION")
	if region == "" {
		region = "us-east-1"
	}

	output, err := s3.CreateBucket(CreateBucketInput{
		Bucket: testBucket,
		Region: region,
	})
	if err != nil {
		t.Fatalf("CreateBucket with region failed: %v", err)
	}
	t.Logf("Created bucket with region %s: %s", region, testBucket)
	if output.Location != "" {
		t.Logf("Location: %s", output.Location)
	}

	// Cleanup
	defer func() {
		time.Sleep(100 * time.Millisecond)
		err := s3.DeleteBucket(DeleteBucketInput{
			Bucket: testBucket,
		})
		if err != nil {
			t.Logf("Failed to cleanup bucket %s: %v", testBucket, err)
		}
	}()

	// Verify bucket exists
	result, err := s3.ListBuckets(ListBucketsInput{})
	if err != nil {
		t.Fatalf("ListBuckets failed: %v", err)
	}

	found := false
	for _, bucket := range result.Buckets {
		if bucket.Name == testBucket {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Created bucket %s not found in ListBuckets", testBucket)
	}
}

func TestCreateBucket_EmptyName(t *testing.T) {
	s3 := setupTestS3(t)

	_, err := s3.CreateBucket(CreateBucketInput{
		Bucket: "",
	})
	if err == nil {
		t.Error("Expected error when creating bucket with empty name, got nil")
	}
	if err != nil && err.Error() != "bucket name is required" {
		t.Errorf("Expected 'bucket name is required' error, got: %v", err)
	}
}

func TestDeleteBucket_EmptyName(t *testing.T) {
	s3 := setupTestS3(t)

	err := s3.DeleteBucket(DeleteBucketInput{
		Bucket: "",
	})
	if err == nil {
		t.Error("Expected error when deleting bucket with empty name, got nil")
	}
	if err != nil && err.Error() != "bucket name is required" {
		t.Errorf("Expected 'bucket name is required' error, got: %v", err)
	}
}

func TestDeleteBucket_NonExistent(t *testing.T) {
	s3 := setupTestS3(t)

	// Try to delete a bucket that doesn't exist
	nonExistentBucket := fmt.Sprintf("nonexistent-bucket-%d", time.Now().Unix())
	err := s3.DeleteBucket(DeleteBucketInput{
		Bucket: nonExistentBucket,
	})
	if err == nil {
		t.Error("Expected error when deleting non-existent bucket, got nil")
	} else {
		t.Logf("Got expected error for non-existent bucket: %v", err)
	}
}

func TestCreateBucket_Duplicate(t *testing.T) {
	s3 := setupTestS3(t)

	testBucket := fmt.Sprintf("simples3-test-dup-%d", time.Now().Unix())

	// Create bucket first time
	_, err := s3.CreateBucket(CreateBucketInput{
		Bucket: testBucket,
	})
	if err != nil {
		t.Fatalf("First CreateBucket failed: %v", err)
	}

	defer func() {
		time.Sleep(100 * time.Millisecond)
		s3.DeleteBucket(DeleteBucketInput{
			Bucket: testBucket,
		})
	}()

	time.Sleep(100 * time.Millisecond)

	// Try to create the same bucket again
	_, err = s3.CreateBucket(CreateBucketInput{
		Bucket: testBucket,
	})
	if err == nil {
		t.Error("Expected error when creating duplicate bucket, got nil")
	} else {
		t.Logf("Got expected error for duplicate bucket: %v", err)
	}
}
