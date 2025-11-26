package simples3

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestListAll(t *testing.T) {
	// Setup test environment
	s3 := New(
		os.Getenv("AWS_S3_REGION"),
		os.Getenv("AWS_S3_ACCESS_KEY"),
		os.Getenv("AWS_S3_SECRET_KEY"),
	)
	s3.SetEndpoint(os.Getenv("AWS_S3_ENDPOINT"))

	// Test cases
	tests := []struct {
		name     string
		setup    func() // Setup function to prepare test data
		input    ListInput
		validate func([]Object, error)
		cleanup  func() // Cleanup function
	}{
		{
			name: "Empty Bucket Iterator",
			input: ListInput{
				Bucket: os.Getenv("AWS_S3_BUCKET"),
			},
			validate: func(objects []Object, err error) {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
				if len(objects) != 0 {
					t.Errorf("Expected empty bucket, got %d objects", len(objects))
				}
			},
		},
		{
			name: "Basic Iterator Listing",
			setup: func() {
				// Upload test files
				uploadTestFiles(t, s3, os.Getenv("AWS_S3_BUCKET"), []string{"iter_file1.txt", "iter_file2.txt", "iter_file3.txt"})
			},
			input: ListInput{
				Bucket: os.Getenv("AWS_S3_BUCKET"),
			},
			validate: func(objects []Object, err error) {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
				if len(objects) != 3 {
					t.Errorf("Expected 3 objects, got %d", len(objects))
				}
				// Check that all expected files are present
				expectedKeys := map[string]bool{
					"iter_file1.txt": false,
					"iter_file2.txt": false,
					"iter_file3.txt": false,
				}
				for _, obj := range objects {
					if _, exists := expectedKeys[obj.Key]; exists {
						expectedKeys[obj.Key] = true
					}
				}
				for key, found := range expectedKeys {
					if !found {
						t.Errorf("Expected object %s not found in iterator results", key)
					}
				}
			},
			cleanup: func() {
				cleanupTestFiles(t, s3, os.Getenv("AWS_S3_BUCKET"), []string{"iter_file1.txt", "iter_file2.txt", "iter_file3.txt"})
			},
		},
		{
			name: "Iterator with Prefix",
			setup: func() {
				uploadTestFiles(t, s3, os.Getenv("AWS_S3_BUCKET"), []string{
					"iter_docs/file1.txt",
					"iter_docs/file2.txt",
					"iter_images/image1.jpg",
					"iter_images/image2.jpg",
				})
			},
			input: ListInput{
				Bucket: os.Getenv("AWS_S3_BUCKET"),
				Prefix: "iter_docs/",
			},
			validate: func(objects []Object, err error) {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
				if len(objects) != 2 {
					t.Errorf("Expected 2 objects with iter_docs/ prefix, got %d", len(objects))
				}
				for _, obj := range objects {
					if !strings.HasPrefix(obj.Key, "iter_docs/") {
						t.Errorf("Object %s doesn't have expected prefix", obj.Key)
					}
				}
			},
			cleanup: func() {
				cleanupTestFiles(t, s3, os.Getenv("AWS_S3_BUCKET"), []string{
					"iter_docs/file1.txt",
					"iter_docs/file2.txt",
					"iter_images/image1.jpg",
					"iter_images/image2.jpg",
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup()
			}

			// Collect all objects from the iterator and check for errors
			var objects []Object
			seq, finish := s3.ListAll(tt.input)
			for obj := range seq {
				objects = append(objects, obj)
			}

			// Check for any iteration errors
			if err := finish(); err != nil {
				t.Fatalf("Iterator error: %v", err)
			}

			if tt.validate != nil {
				tt.validate(objects, nil) // No error from iterator
			}

			if tt.cleanup != nil {
				tt.cleanup()
			}
		})
	}
}

func TestList(t *testing.T) {
	// Setup test environment
	s3 := New(
		os.Getenv("AWS_S3_REGION"),
		os.Getenv("AWS_S3_ACCESS_KEY"),
		os.Getenv("AWS_S3_SECRET_KEY"),
	)
	s3.SetEndpoint(os.Getenv("AWS_S3_ENDPOINT"))

	// Test cases
	tests := []struct {
		name     string
		setup    func() // Setup function to prepare test data
		input    ListInput
		validate func(ListResponse, error)
		cleanup  func() // Cleanup function
	}{
		{
			name: "Empty Bucket",
			input: ListInput{
				Bucket: os.Getenv("AWS_S3_BUCKET"),
			},
			validate: func(result ListResponse, err error) {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
				if len(result.Objects) != 0 {
					t.Errorf("Expected empty bucket, got %d objects", len(result.Objects))
				}
			},
		},
		{
			name: "Basic Listing",
			setup: func() {
				// Upload test files
				uploadTestFiles(t, s3, os.Getenv("AWS_S3_BUCKET"), []string{"file1.txt", "file2.txt", "file3.txt"})
			},
			input: ListInput{
				Bucket: os.Getenv("AWS_S3_BUCKET"),
			},
			validate: func(result ListResponse, err error) {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
				if len(result.Objects) != 3 {
					t.Errorf("Expected 3 objects, got %d", len(result.Objects))
				}
			},
			cleanup: func() {
				cleanupTestFiles(t, s3, os.Getenv("AWS_S3_BUCKET"), []string{"file1.txt", "file2.txt", "file3.txt"})
			},
		},
		{
			name: "Pagination Test",
			setup: func() {
				// Upload many files for pagination testing
				var filenames []string
				for i := 1; i <= 25; i++ {
					filenames = append(filenames, fmt.Sprintf("file%03d.txt", i))
				}
				uploadTestFiles(t, s3, os.Getenv("AWS_S3_BUCKET"), filenames)
			},
			input: ListInput{
				Bucket:  os.Getenv("AWS_S3_BUCKET"),
				MaxKeys: 10,
			},
			validate: func(result ListResponse, err error) {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
				if len(result.Objects) != 10 {
					t.Errorf("Expected 10 objects, got %d", len(result.Objects))
				}
				if !result.IsTruncated {
					t.Errorf("Expected IsTruncated=true for pagination")
				}
				if result.NextContinuationToken == "" {
					t.Errorf("Expected NextContinuationToken for pagination: %v", result.NextContinuationToken)
				}
			},
			cleanup: func() {
				var filenames []string
				for i := 1; i <= 25; i++ {
					filenames = append(filenames, fmt.Sprintf("file%03d.txt", i))
				}
				cleanupTestFiles(t, s3, os.Getenv("AWS_S3_BUCKET"), filenames)
			},
		},
		{
			name: "Prefix Filtering",
			setup: func() {
				uploadTestFiles(t, s3, os.Getenv("AWS_S3_BUCKET"), []string{
					"documents/file1.txt",
					"documents/file2.txt",
					"images/image1.jpg",
					"images/image2.jpg",
				})
			},
			input: ListInput{
				Bucket: os.Getenv("AWS_S3_BUCKET"),
				Prefix: "documents/",
			},
			validate: func(result ListResponse, err error) {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
				if len(result.Objects) != 2 {
					t.Errorf("Expected 2 objects with documents/ prefix, got %d", len(result.Objects))
				}
				for _, obj := range result.Objects {
					if !strings.HasPrefix(obj.Key, "documents/") {
						t.Errorf("Object %s doesn't have expected prefix", obj.Key)
					}
				}
			},
			cleanup: func() {
				cleanupTestFiles(t, s3, os.Getenv("AWS_S3_BUCKET"), []string{
					"documents/file1.txt",
					"documents/file2.txt",
					"images/image1.jpg",
					"images/image2.jpg",
				})
			},
		},
		{
			name: "Delimiter Grouping",
			setup: func() {
				uploadTestFiles(t, s3, os.Getenv("AWS_S3_BUCKET"), []string{
					"documents/important/file1.txt",
					"documents/backup/file2.txt",
					"images/landscape/image1.jpg",
					"images/portrait/image2.jpg",
				})
			},
			input: ListInput{
				Bucket:    os.Getenv("AWS_S3_BUCKET"),
				Delimiter: "/",
				Prefix:    "documents/",
			},
			validate: func(result ListResponse, err error) {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
				// Should return common prefixes instead of individual files
				if len(result.CommonPrefixes) != 2 {
					t.Errorf("Expected 2 common prefixes, got %d", len(result.CommonPrefixes))
				}
			},
			cleanup: func() {
				cleanupTestFiles(t, s3, os.Getenv("AWS_S3_BUCKET"), []string{
					"documents/important/file1.txt",
					"documents/backup/file2.txt",
					"images/landscape/image1.jpg",
					"images/portrait/image2.jpg",
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup()
			}

			result, err := s3.List(tt.input)

			if tt.validate != nil {
				tt.validate(result, err)
			}

			if tt.cleanup != nil {
				tt.cleanup()
			}
		})
	}
}
