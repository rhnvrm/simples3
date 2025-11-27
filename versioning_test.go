package simples3

import (
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

func TestVersioning(t *testing.T) {
	s3 := setupTestS3(t)
	bucket := fmt.Sprintf("simples3-versioning-test-%d", time.Now().Unix())

	// Create bucket
	_, err := s3.CreateBucket(CreateBucketInput{Bucket: bucket})
	if err != nil {
		t.Fatalf("CreateBucket failed: %v", err)
	}
	
	// Cleanup function
	defer func() {
		// List all versions and delete markers to clean up
		listResp, err := s3.ListVersions(ListVersionsInput{Bucket: bucket})
		if err == nil {
			for _, v := range listResp.Versions {
				s3.FileDelete(DeleteInput{Bucket: bucket, ObjectKey: v.Key, VersionId: v.VersionId})
			}
			for _, d := range listResp.DeleteMarkers {
				s3.FileDelete(DeleteInput{Bucket: bucket, ObjectKey: d.Key, VersionId: d.VersionId})
			}
		}
		s3.DeleteBucket(DeleteBucketInput{Bucket: bucket})
	}()

	// 1. Enable Versioning
	err = s3.PutBucketVersioning(PutBucketVersioningInput{
		Bucket: bucket,
		Status: "Enabled",
	})
	if err != nil {
		t.Fatalf("PutBucketVersioning failed: %v", err)
	}

	// 2. Get Versioning Status
	vConf, err := s3.GetBucketVersioning(bucket)
	if err != nil {
		t.Fatalf("GetBucketVersioning failed: %v", err)
	}
	if vConf.Status != "Enabled" {
		t.Errorf("Expected versioning Enabled, got %s", vConf.Status)
	}

	key := "test-object.txt"
	
	// 3. Upload v1
	_, err = s3.FilePut(UploadInput{
		Bucket: bucket,
		ObjectKey: key,
		Body: strings.NewReader("v1"),
	})
	if err != nil {
		t.Fatalf("FilePut v1 failed: %v", err)
	}
	
	// Sleep briefly to ensure timestamp difference (MinIO sometimes has granularity issues)
	time.Sleep(1 * time.Second)

	// 4. Upload v2
	_, err = s3.FilePut(UploadInput{
		Bucket: bucket,
		ObjectKey: key,
		Body: strings.NewReader("v2"),
	})
	if err != nil {
		t.Fatalf("FilePut v2 failed: %v", err)
	}

	// 5. List Versions
	listResp, err := s3.ListVersions(ListVersionsInput{
		Bucket: bucket,
		Prefix: key,
	})
	if err != nil {
		t.Fatalf("ListVersions failed: %v", err)
	}

	if len(listResp.Versions) != 2 {
		t.Errorf("Expected 2 versions, got %d", len(listResp.Versions))
	}

	// Identify versions
	var v1Id, v2Id string
	// ListVersions returns latest first usually.
	for _, v := range listResp.Versions {
		if v.IsLatest {
			v2Id = v.VersionId
		} else {
			v1Id = v.VersionId
		}
	}

	if v1Id == "" || v2Id == "" {
		t.Fatalf("Failed to identify v1 and v2 IDs. Versions: %+v", listResp.Versions)
	}

	// 6. Download v1 specific version
	rc, err := s3.FileDownload(DownloadInput{
		Bucket: bucket,
		ObjectKey: key,
		VersionId: v1Id,
	})
	if err != nil {
		t.Fatalf("FileDownload v1 failed: %v", err)
	}
	content, _ := io.ReadAll(rc)
	rc.Close()
	if string(content) != "v1" {
		t.Errorf("Expected v1 content 'v1', got '%s'", string(content))
	}

	// 7. Download v2 specific version
	rc, err = s3.FileDownload(DownloadInput{
		Bucket: bucket,
		ObjectKey: key,
		VersionId: v2Id,
	})
	if err != nil {
		t.Fatalf("FileDownload v2 failed: %v", err)
	}
	content, _ = io.ReadAll(rc)
	rc.Close()
	if string(content) != "v2" {
		t.Errorf("Expected v2 content 'v2', got '%s'", string(content))
	}
	
	// 8. Get Details for v1
	details, err := s3.FileDetails(DetailsInput{
		Bucket: bucket,
		ObjectKey: key,
		VersionId: v1Id,
	})
	if err != nil {
		t.Fatalf("FileDetails v1 failed: %v", err)
	}
	if details.ContentLength != "2" { // "v1" is 2 bytes
		t.Errorf("Expected v1 size 2, got %s", details.ContentLength)
	}

	// 9. Delete v2 (latest)
	err = s3.FileDelete(DeleteInput{
		Bucket: bucket,
		ObjectKey: key,
		VersionId: v2Id,
	})
	if err != nil {
		t.Fatalf("FileDelete v2 failed: %v", err)
	}

	// 10. Verify current object is v1
	rc, err = s3.FileDownload(DownloadInput{
		Bucket: bucket,
		ObjectKey: key,
	})
	if err != nil {
		t.Fatalf("FileDownload current failed: %v", err)
	}
	content, _ = io.ReadAll(rc)
	rc.Close()
	if string(content) != "v1" {
		t.Errorf("Expected current content 'v1', got '%s'", string(content))
	}
}
