package simples3

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"testing"
)

func TestMultipartUploadWorkflow(t *testing.T) {
	s3 := setupTestS3(t)
	bucket := os.Getenv("AWS_S3_BUCKET")
	if bucket == "" {
		t.Skip("AWS_S3_BUCKET not set")
	}

	objectKey := "test-multipart-file.bin"

	// Cleanup at the end
	defer func() {
		s3.FileDelete(DeleteInput{
			Bucket:    bucket,
			ObjectKey: objectKey,
		})
	}()

	// Test 1: Initiate multipart upload
	t.Run("InitiateMultipartUpload", func(t *testing.T) {
		output, err := s3.InitiateMultipartUpload(InitiateMultipartUploadInput{
			Bucket:      bucket,
			ObjectKey:   objectKey,
			ContentType: "application/octet-stream",
		})
		if err != nil {
			t.Fatalf("InitiateMultipartUpload failed: %v", err)
		}

		if output.Bucket != bucket {
			t.Errorf("Expected bucket %s, got %s", bucket, output.Bucket)
		}
		if output.Key != objectKey {
			t.Errorf("Expected key %s, got %s", objectKey, output.Key)
		}
		if output.UploadID == "" {
			t.Error("UploadID should not be empty")
		}

		// Cleanup: abort the upload
		err = s3.AbortMultipartUpload(AbortMultipartUploadInput{
			Bucket:    bucket,
			ObjectKey: objectKey,
			UploadID:  output.UploadID,
		})
		if err != nil {
			t.Logf("Warning: failed to abort upload: %v", err)
		}
	})

	// Test 2: Complete multipart upload workflow
	t.Run("CompleteWorkflow", func(t *testing.T) {
		// Create test data (3 parts of 5MB each)
		partSize := int64(5 * 1024 * 1024) // 5MB
		numParts := 3
		totalSize := partSize * int64(numParts)

		testData := make([]byte, totalSize)
		_, err := rand.Read(testData)
		if err != nil {
			t.Fatalf("Failed to generate test data: %v", err)
		}

		// Initiate
		initOutput, err := s3.InitiateMultipartUpload(InitiateMultipartUploadInput{
			Bucket:      bucket,
			ObjectKey:   objectKey,
			ContentType: "application/octet-stream",
		})
		if err != nil {
			t.Fatalf("InitiateMultipartUpload failed: %v", err)
		}

		// Upload parts
		var completedParts []CompletedPart
		for i := 0; i < numParts; i++ {
			start := int64(i) * partSize
			end := start + partSize
			if end > totalSize {
				end = totalSize
			}

			partData := testData[start:end]

			output, err := s3.UploadPart(UploadPartInput{
				Bucket:     bucket,
				ObjectKey:  objectKey,
				UploadID:   initOutput.UploadID,
				PartNumber: i + 1,
				Body:       bytes.NewReader(partData),
				Size:       int64(len(partData)),
			})
			if err != nil {
				s3.AbortMultipartUpload(AbortMultipartUploadInput{
					Bucket:    bucket,
					ObjectKey: objectKey,
					UploadID:  initOutput.UploadID,
				})
				t.Fatalf("UploadPart %d failed: %v", i+1, err)
			}

			if output.ETag == "" {
				t.Errorf("Part %d ETag should not be empty", i+1)
			}

			completedParts = append(completedParts, CompletedPart{
				PartNumber: output.PartNumber,
				ETag:       output.ETag,
			})
		}

		// Complete
		completeOutput, err := s3.CompleteMultipartUpload(CompleteMultipartUploadInput{
			Bucket:    bucket,
			ObjectKey: objectKey,
			UploadID:  initOutput.UploadID,
			Parts:     completedParts,
		})
		if err != nil {
			t.Fatalf("CompleteMultipartUpload failed: %v", err)
		}

		if completeOutput.Key != objectKey {
			t.Errorf("Expected key %s, got %s", objectKey, completeOutput.Key)
		}
		if completeOutput.ETag == "" {
			t.Error("ETag should not be empty")
		}

		// Verify file was uploaded correctly
		downloadedFile, err := s3.FileDownload(DownloadInput{
			Bucket:    bucket,
			ObjectKey: objectKey,
		})
		if err != nil {
			t.Fatalf("FileDownload failed: %v", err)
		}
		defer downloadedFile.Close()

		downloadedData, err := io.ReadAll(downloadedFile)
		if err != nil {
			t.Fatalf("Failed to read downloaded file: %v", err)
		}

		if !bytes.Equal(testData, downloadedData) {
			t.Error("Downloaded data does not match uploaded data")
		}
	})

	// Test 3: Abort multipart upload
	t.Run("AbortMultipartUpload", func(t *testing.T) {
		initOutput, err := s3.InitiateMultipartUpload(InitiateMultipartUploadInput{
			Bucket:    bucket,
			ObjectKey: objectKey + "-abort",
		})
		if err != nil {
			t.Fatalf("InitiateMultipartUpload failed: %v", err)
		}

		err = s3.AbortMultipartUpload(AbortMultipartUploadInput{
			Bucket:    bucket,
			ObjectKey: objectKey + "-abort",
			UploadID:  initOutput.UploadID,
		})
		if err != nil {
			t.Errorf("AbortMultipartUpload failed: %v", err)
		}
	})

	// Test 4: ListParts
	t.Run("ListParts", func(t *testing.T) {
		// Initiate
		initOutput, err := s3.InitiateMultipartUpload(InitiateMultipartUploadInput{
			Bucket:    bucket,
			ObjectKey: objectKey + "-list",
		})
		if err != nil {
			t.Fatalf("InitiateMultipartUpload failed: %v", err)
		}

		defer func() {
			s3.AbortMultipartUpload(AbortMultipartUploadInput{
				Bucket:    bucket,
				ObjectKey: objectKey + "-list",
				UploadID:  initOutput.UploadID,
			})
		}()

		// Upload 2 parts
		partSize := int64(5 * 1024 * 1024)
		testData := make([]byte, partSize)

		for i := 1; i <= 2; i++ {
			_, err := rand.Read(testData)
			if err != nil {
				t.Fatalf("Failed to generate test data: %v", err)
			}

			_, err = s3.UploadPart(UploadPartInput{
				Bucket:     bucket,
				ObjectKey:  objectKey + "-list",
				UploadID:   initOutput.UploadID,
				PartNumber: i,
				Body:       bytes.NewReader(testData),
				Size:       partSize,
			})
			if err != nil {
				t.Fatalf("UploadPart %d failed: %v", i, err)
			}
		}

		// List parts
		listOutput, err := s3.ListParts(ListPartsInput{
			Bucket:    bucket,
			ObjectKey: objectKey + "-list",
			UploadID:  initOutput.UploadID,
		})
		if err != nil {
			t.Fatalf("ListParts failed: %v", err)
		}

		if len(listOutput.Parts) != 2 {
			t.Errorf("Expected 2 parts, got %d", len(listOutput.Parts))
		}

		for i, part := range listOutput.Parts {
			if part.PartNumber != i+1 {
				t.Errorf("Expected part number %d, got %d", i+1, part.PartNumber)
			}
			if part.ETag == "" {
				t.Errorf("Part %d ETag should not be empty", i+1)
			}
			if part.Size != partSize {
				t.Errorf("Expected part size %d, got %d", partSize, part.Size)
			}
		}
	})
}

func TestFileUploadMultipart(t *testing.T) {
	s3 := setupTestS3(t)
	bucket := os.Getenv("AWS_S3_BUCKET")
	if bucket == "" {
		t.Skip("AWS_S3_BUCKET not set")
	}

	objectKey := "test-file-upload-multipart.bin"

	defer func() {
		s3.FileDelete(DeleteInput{
			Bucket:    bucket,
			ObjectKey: objectKey,
		})
	}()

	// Test 1: Sequential upload
	t.Run("SequentialUpload", func(t *testing.T) {
		// Create 15MB test file (3 parts of 5MB each)
		testData := make([]byte, 15*1024*1024)
		_, err := rand.Read(testData)
		if err != nil {
			t.Fatalf("Failed to generate test data: %v", err)
		}

		progressCalls := 0
		output, err := s3.FileUploadMultipart(MultipartUploadInput{
			Bucket:      bucket,
			ObjectKey:   objectKey,
			Body:        bytes.NewReader(testData),
			ContentType: "application/octet-stream",
			PartSize:    5 * 1024 * 1024,
			OnProgress: func(info ProgressInfo) {
				progressCalls++
				t.Logf("Progress: %d/%d bytes (%d/%d parts) @ %d B/s",
					info.UploadedBytes, info.TotalBytes,
					info.CurrentPart, info.TotalParts,
					info.BytesPerSecond)
			},
		})
		if err != nil {
			t.Fatalf("FileUploadMultipart failed: %v", err)
		}

		if output.Key != objectKey {
			t.Errorf("Expected key %s, got %s", objectKey, output.Key)
		}
		if output.ETag == "" {
			t.Error("ETag should not be empty")
		}
		if progressCalls == 0 {
			t.Error("Progress callback should have been called")
		}

		// Verify uploaded file
		downloadedFile, err := s3.FileDownload(DownloadInput{
			Bucket:    bucket,
			ObjectKey: objectKey,
		})
		if err != nil {
			t.Fatalf("FileDownload failed: %v", err)
		}
		defer downloadedFile.Close()

		downloadedData, err := io.ReadAll(downloadedFile)
		if err != nil {
			t.Fatalf("Failed to read downloaded file: %v", err)
		}

		if !bytes.Equal(testData, downloadedData) {
			t.Error("Downloaded data does not match uploaded data")
		}
	})

	// Test 2: Parallel upload
	t.Run("ParallelUpload", func(t *testing.T) {
		objectKeyParallel := objectKey + "-parallel"

		defer func() {
			s3.FileDelete(DeleteInput{
				Bucket:    bucket,
				ObjectKey: objectKeyParallel,
			})
		}()

		// Create 20MB test file (4 parts of 5MB each)
		testData := make([]byte, 20*1024*1024)
		_, err := rand.Read(testData)
		if err != nil {
			t.Fatalf("Failed to generate test data: %v", err)
		}

		output, err := s3.FileUploadMultipart(MultipartUploadInput{
			Bucket:      bucket,
			ObjectKey:   objectKeyParallel,
			Body:        bytes.NewReader(testData),
			ContentType: "application/octet-stream",
			PartSize:    5 * 1024 * 1024,
			Concurrency: 3, // Upload 3 parts in parallel
		})
		if err != nil {
			t.Fatalf("FileUploadMultipart (parallel) failed: %v", err)
		}

		if output.Key != objectKeyParallel {
			t.Errorf("Expected key %s, got %s", objectKeyParallel, output.Key)
		}

		// Verify uploaded file
		downloadedFile, err := s3.FileDownload(DownloadInput{
			Bucket:    bucket,
			ObjectKey: objectKeyParallel,
		})
		if err != nil {
			t.Fatalf("FileDownload failed: %v", err)
		}
		defer downloadedFile.Close()

		downloadedData, err := io.ReadAll(downloadedFile)
		if err != nil {
			t.Fatalf("Failed to read downloaded file: %v", err)
		}

		if !bytes.Equal(testData, downloadedData) {
			t.Error("Downloaded data does not match uploaded data")
		}
	})

	// Test 3: Small file (single part)
	t.Run("SmallFile", func(t *testing.T) {
		objectKeySmall := objectKey + "-small"

		defer func() {
			s3.FileDelete(DeleteInput{
				Bucket:    bucket,
				ObjectKey: objectKeySmall,
			})
		}()

		// Create 3MB test file (smaller than partSize)
		testData := make([]byte, 3*1024*1024)
		_, err := rand.Read(testData)
		if err != nil {
			t.Fatalf("Failed to generate test data: %v", err)
		}

		output, err := s3.FileUploadMultipart(MultipartUploadInput{
			Bucket:      bucket,
			ObjectKey:   objectKeySmall,
			Body:        bytes.NewReader(testData),
			ContentType: "application/octet-stream",
			PartSize:    5 * 1024 * 1024,
		})
		if err != nil {
			t.Fatalf("FileUploadMultipart (small file) failed: %v", err)
		}

		if output.Key != objectKeySmall {
			t.Errorf("Expected key %s, got %s", objectKeySmall, output.Key)
		}

		// Verify uploaded file
		downloadedFile, err := s3.FileDownload(DownloadInput{
			Bucket:    bucket,
			ObjectKey: objectKeySmall,
		})
		if err != nil {
			t.Fatalf("FileDownload failed: %v", err)
		}
		defer downloadedFile.Close()

		downloadedData, err := io.ReadAll(downloadedFile)
		if err != nil {
			t.Fatalf("Failed to read downloaded file: %v", err)
		}

		if !bytes.Equal(testData, downloadedData) {
			t.Error("Downloaded data does not match uploaded data")
		}
	})
}

func TestMultipartValidation(t *testing.T) {
	s3 := setupTestS3(t)
	bucket := os.Getenv("AWS_S3_BUCKET")

	tests := []struct {
		name      string
		fn        func() error
		expectErr bool
	}{
		{
			name: "InitiateMultipartUpload_NoBucket",
			fn: func() error {
				_, err := s3.InitiateMultipartUpload(InitiateMultipartUploadInput{
					ObjectKey: "test",
				})
				return err
			},
			expectErr: true,
		},
		{
			name: "InitiateMultipartUpload_NoKey",
			fn: func() error {
				_, err := s3.InitiateMultipartUpload(InitiateMultipartUploadInput{
					Bucket: bucket,
				})
				return err
			},
			expectErr: true,
		},
		{
			name: "UploadPart_NoBucket",
			fn: func() error {
				_, err := s3.UploadPart(UploadPartInput{
					ObjectKey:  "test",
					UploadID:   "test",
					PartNumber: 1,
					Body:       bytes.NewReader([]byte("test")),
					Size:       4,
				})
				return err
			},
			expectErr: true,
		},
		{
			name: "UploadPart_InvalidPartNumber",
			fn: func() error {
				_, err := s3.UploadPart(UploadPartInput{
					Bucket:     bucket,
					ObjectKey:  "test",
					UploadID:   "test",
					PartNumber: 0,
					Body:       bytes.NewReader([]byte("test")),
					Size:       4,
				})
				return err
			},
			expectErr: true,
		},
		{
			name: "UploadPart_PartNumberTooLarge",
			fn: func() error {
				_, err := s3.UploadPart(UploadPartInput{
					Bucket:     bucket,
					ObjectKey:  "test",
					UploadID:   "test",
					PartNumber: MaxParts + 1,
					Body:       bytes.NewReader([]byte("test")),
					Size:       4,
				})
				return err
			},
			expectErr: true,
		},
		{
			name: "CompleteMultipartUpload_NoParts",
			fn: func() error {
				_, err := s3.CompleteMultipartUpload(CompleteMultipartUploadInput{
					Bucket:    bucket,
					ObjectKey: "test",
					UploadID:  "test",
					Parts:     []CompletedPart{},
				})
				return err
			},
			expectErr: true,
		},
		{
			name: "FileUploadMultipart_PartSizeTooSmall",
			fn: func() error {
				_, err := s3.FileUploadMultipart(MultipartUploadInput{
					Bucket:    bucket,
					ObjectKey: "test",
					Body:      bytes.NewReader([]byte("test")),
					PartSize:  1024, // Less than MinPartSize
				})
				return err
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if tt.expectErr && err == nil {
				t.Error("Expected error but got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestGeneratePresignedUploadPartURL(t *testing.T) {
	s3 := setupTestS3(t)
	bucket := os.Getenv("AWS_S3_BUCKET")
	if bucket == "" {
		t.Skip("AWS_S3_BUCKET not set")
	}

	url := s3.GeneratePresignedUploadPartURL(PresignedMultipartInput{
		Bucket:        bucket,
		ObjectKey:     "test-presigned.bin",
		UploadID:      "test-upload-id",
		PartNumber:    1,
		ExpirySeconds: 3600,
	})

	if url == "" {
		t.Error("Presigned URL should not be empty")
	}

	// Check that URL contains required parameters
	requiredParams := []string{
		"partNumber=1",
		"uploadId=test-upload-id",
		"X-Amz-Algorithm=AWS4-HMAC-SHA256",
		"X-Amz-Signature=",
	}

	for _, param := range requiredParams {
		if !contains(url, param) {
			t.Errorf("URL should contain %s", param)
		}
	}

	t.Logf("Generated presigned URL: %s", url)
}

func BenchmarkFileUploadMultipart(b *testing.B) {
	s3 := setupTestS3(&testing.T{})
	bucket := os.Getenv("AWS_S3_BUCKET")
	if bucket == "" {
		b.Skip("AWS_S3_BUCKET not set")
	}

	// Create 10MB test data
	testData := make([]byte, 10*1024*1024)
	rand.Read(testData)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		objectKey := fmt.Sprintf("bench-multipart-%d.bin", i)

		_, err := s3.FileUploadMultipart(MultipartUploadInput{
			Bucket:      bucket,
			ObjectKey:   objectKey,
			Body:        bytes.NewReader(testData),
			ContentType: "application/octet-stream",
			PartSize:    5 * 1024 * 1024,
		})
		if err != nil {
			b.Fatalf("FileUploadMultipart failed: %v", err)
		}

		// Cleanup
		s3.FileDelete(DeleteInput{
			Bucket:    bucket,
			ObjectKey: objectKey,
		})
	}
}
