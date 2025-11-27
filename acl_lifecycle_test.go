package simples3

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestBucketAcl(t *testing.T) {
	s3 := setupTestS3(t)
	bucketName := fmt.Sprintf("simples3-test-acl-%d", time.Now().UnixNano())

	// Create bucket
	_, err := s3.CreateBucket(CreateBucketInput{Bucket: bucketName})
	if err != nil {
		t.Fatalf("Failed to create bucket: %v", err)
	}
	defer s3.DeleteBucket(DeleteBucketInput{Bucket: bucketName})

	// 1. Test Canned ACL
	t.Run("CannedACL", func(t *testing.T) {
		err := s3.PutBucketAcl(PutBucketAclInput{
			Bucket:    bucketName,
			CannedACL: "public-read",
		})
		if err != nil {
			if strings.HasPrefix(err.Error(), "status code: 501 Not Implemented") {
				t.Skipf("Skipping CannedACL test: backend returned 501 Not Implemented")
			}
			t.Fatalf("PutBucketAcl (canned) failed: %v", err)
		}

		// Verify
		acl, err := s3.GetBucketAcl(bucketName)
		if err != nil {
			t.Fatalf("GetBucketAcl failed: %v", err)
		}

		if len(acl.AccessControlList) == 0 {
			t.Errorf("Expected ACL grants, got none")
		}
		t.Logf("Bucket ACL Owner: %s", acl.Owner.DisplayName)
	})

	// 2. Test Custom ACL (AccessControlPolicy)
	// We need the owner ID first
	acl, err := s3.GetBucketAcl(bucketName)
	if err != nil {
		t.Fatalf("Failed to get initial ACL: %v", err)
	}

	t.Run("CustomACL", func(t *testing.T) {
		// Create a policy that gives FULL_CONTROL to the owner
		policy := &AccessControlPolicy{
			Owner: acl.Owner,
			AccessControlList: []Grant{
				{
					Grantee: Grantee{
						Type:        "CanonicalUser",
						ID:          acl.Owner.ID,
						DisplayName: acl.Owner.DisplayName,
					},
					Permission: "FULL_CONTROL",
				},
			},
		}

		err := s3.PutBucketAcl(PutBucketAclInput{
			Bucket:              bucketName,
			AccessControlPolicy: policy,
		})
		if err != nil {
			t.Fatalf("PutBucketAcl (custom) failed: %v", err)
		}

		// Verify
		newAcl, err := s3.GetBucketAcl(bucketName)
		if err != nil {
			t.Fatalf("GetBucketAcl failed: %v", err)
		}

		if len(newAcl.AccessControlList) != 1 {
			t.Errorf("Expected 1 grant, got %d", len(newAcl.AccessControlList))
		}
		if newAcl.AccessControlList[0].Permission != "FULL_CONTROL" {
			t.Errorf("Expected FULL_CONTROL, got %s", newAcl.AccessControlList[0].Permission)
		}
	})
}

func TestObjectAcl(t *testing.T) {
	s3 := setupTestS3(t)
	bucketName := fmt.Sprintf("simples3-test-obj-acl-%d", time.Now().UnixNano())
	objectKey := "test-object.txt"

	// Create bucket
	_, err := s3.CreateBucket(CreateBucketInput{Bucket: bucketName})
	if err != nil {
		t.Fatalf("Failed to create bucket: %v", err)
	}
	defer s3.DeleteBucket(DeleteBucketInput{Bucket: bucketName})

	// Upload object
	_, err = s3.FileUpload(UploadInput{
		Bucket:      bucketName,
		ObjectKey:   objectKey,
		Body:        bytes.NewReader([]byte("test content")),
		ContentType: "text/plain",
	})
	if err != nil {
		t.Fatalf("Failed to upload object: %v", err)
	}
	defer s3.FileDelete(DeleteInput{Bucket: bucketName, ObjectKey: objectKey})

	// 1. Test Canned ACL
	t.Run("CannedACL", func(t *testing.T) {
		err := s3.PutObjectAcl(PutObjectAclInput{
			Bucket:    bucketName,
			ObjectKey: objectKey,
			CannedACL: "public-read",
		})
		if err != nil {
			if strings.HasPrefix(err.Error(), "status code: 501 Not Implemented") {
				t.Skipf("Skipping CannedACL test: backend returned 501 Not Implemented")
			}
			t.Fatalf("PutObjectAcl (canned) failed: %v", err)
		}

		// Verify
		acl, err := s3.GetObjectAcl(GetObjectAclInput{
			Bucket:    bucketName,
			ObjectKey: objectKey,
		})
		if err != nil {
			t.Fatalf("GetObjectAcl failed: %v", err)
		}

		if len(acl.AccessControlList) == 0 {
			t.Errorf("Expected ACL grants, got none")
		}
	})

	// 2. Test Custom ACL (AccessControlPolicy)
	// We need the owner ID first
	acl, err := s3.GetObjectAcl(GetObjectAclInput{
		Bucket:    bucketName,
		ObjectKey: objectKey,
	})
	if err != nil {
		t.Fatalf("Failed to get initial Object ACL: %v", err)
	}

	t.Run("CustomACL", func(t *testing.T) {
		// Create a policy that gives FULL_CONTROL to the owner
		policy := &AccessControlPolicy{
			Owner: acl.Owner,
			AccessControlList: []Grant{
				{
					Grantee: Grantee{
						Type:        "CanonicalUser",
						ID:          acl.Owner.ID,
						DisplayName: acl.Owner.DisplayName,
					},
					Permission: "FULL_CONTROL",
				},
			},
		}

		err := s3.PutObjectAcl(PutObjectAclInput{
			Bucket:              bucketName,
			ObjectKey:           objectKey,
			AccessControlPolicy: policy,
		})
		if err != nil {
			t.Fatalf("PutObjectAcl (custom) failed: %v", err)
		}

		// Verify
		newAcl, err := s3.GetObjectAcl(GetObjectAclInput{
			Bucket:    bucketName,
			ObjectKey: objectKey,
		})
		if err != nil {
			t.Fatalf("GetObjectAcl failed: %v", err)
		}

		if len(newAcl.AccessControlList) != 1 {
			t.Errorf("Expected 1 grant, got %d", len(newAcl.AccessControlList))
		}
		if newAcl.AccessControlList[0].Permission != "FULL_CONTROL" {
			t.Errorf("Expected FULL_CONTROL, got %s", newAcl.AccessControlList[0].Permission)
		}
	})
}

func TestBucketLifecycle(t *testing.T) {
	s3 := setupTestS3(t)
	bucketName := fmt.Sprintf("simples3-test-lifecycle-%d", time.Now().UnixNano())

	// Create bucket
	_, err := s3.CreateBucket(CreateBucketInput{Bucket: bucketName})
	if err != nil {
		t.Fatalf("Failed to create bucket: %v", err)
	}
	defer s3.DeleteBucket(DeleteBucketInput{Bucket: bucketName})

	// 1. Test PutLifecycle
	t.Run("PutLifecycle", func(t *testing.T) {
		config := &LifecycleConfiguration{
			Rules: []LifecycleRule{
				{
					ID:     "Rule1",
					Status: "Enabled",
					Filter: &LifecycleFilter{
						Prefix: "logs/",
					},
					Expiration: &LifecycleExpiration{
						Days: 30,
					},
				},
			},
		}

		err := s3.PutBucketLifecycle(PutBucketLifecycleInput{
			Bucket:        bucketName,
			Configuration: config,
		})
		if err != nil {
			t.Fatalf("PutBucketLifecycle failed: %v", err)
		}

		// Verify
		fetchedConfig, err := s3.GetBucketLifecycle(bucketName)
		if err != nil {
			t.Fatalf("GetBucketLifecycle failed: %v", err)
		}

		if len(fetchedConfig.Rules) != 1 {
			t.Errorf("Expected 1 rule, got %d", len(fetchedConfig.Rules))
		}
		if fetchedConfig.Rules[0].ID != "Rule1" {
			t.Errorf("Expected Rule1, got %s", fetchedConfig.Rules[0].ID)
		}
		if fetchedConfig.Rules[0].Filter == nil || fetchedConfig.Rules[0].Filter.Prefix != "logs/" {
			t.Errorf("Expected prefix 'logs/', got %v", fetchedConfig.Rules[0].Filter)
		}
	})

	// 2. Test DeleteLifecycle
	t.Run("DeleteLifecycle", func(t *testing.T) {
		err := s3.DeleteBucketLifecycle(DeleteBucketInput{
			Bucket: bucketName,
		})
		if err != nil {
			t.Fatalf("DeleteBucketLifecycle failed: %v", err)
		}

		// Verify
		_, err = s3.GetBucketLifecycle(bucketName)
		if err == nil {
			t.Errorf("Expected error after deleting lifecycle, got nil")
		} else {
			// Should verify it's a 404/NoSuchLifecycleConfiguration error
			// The exact error message depends on implementation, but we expect an error
			t.Logf("Got expected error: %v", err)
		}
	})
}
