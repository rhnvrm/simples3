package simples3

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestS3_FileUploadPostAndPut(t *testing.T) {
	testTxt, err := os.Open("testdata/test.txt")
	if err != nil {
		t.Fatalf("Failed to open test.txt: %v", err)
	}
	defer testTxt.Close()
	// Note: cannot re-use the same file descriptor due to seeking!
	testTxtSpecialChars, err := os.Open("testdata/test.txt")
	if err != nil {
		t.Fatalf("Failed to open test.txt for special chars: %v", err)
	}
	defer testTxtSpecialChars.Close()
	testPng, err := os.Open("testdata/avatar.png")
	if err != nil {
		t.Fatalf("Failed to open avatar.png: %v", err)
	}
	defer testPng.Close()

	type args struct {
		u UploadInput
	}
	tests := []struct {
		name        string
		fields      tConfig
		args        args
		testDetails bool
		wantErr     bool
	}{
		{
			name: "Upload test.txt",
			fields: tConfig{
				AccessKey: os.Getenv("AWS_S3_ACCESS_KEY"),
				SecretKey: os.Getenv("AWS_S3_SECRET_KEY"),
				Endpoint:  os.Getenv("AWS_S3_ENDPOINT"),
				Region:    os.Getenv("AWS_S3_REGION"),
			},
			args: args{
				UploadInput{
					Bucket:      os.Getenv("AWS_S3_BUCKET"),
					ObjectKey:   "test.txt",
					ContentType: "text/plain",
					FileName:    "test.txt",
					Body:        testTxt,
				},
			},
			wantErr: false,
		},
		{
			name: "Upload test.txt with custom metadata",
			fields: tConfig{
				AccessKey: os.Getenv("AWS_S3_ACCESS_KEY"),
				SecretKey: os.Getenv("AWS_S3_SECRET_KEY"),
				Endpoint:  os.Getenv("AWS_S3_ENDPOINT"),
				Region:    os.Getenv("AWS_S3_REGION"),
			},
			args: args{
				UploadInput{
					Bucket:      os.Getenv("AWS_S3_BUCKET"),
					ObjectKey:   "test_metadata.txt",
					ContentType: "text/plain",
					FileName:    "test.txt",
					Body:        testTxt,
					CustomMetadata: map[string]string{
						"test-metadata": "foo-bar",
					},
				},
			},
			wantErr:     false,
			testDetails: true,
		},
		{
			name: "Upload avatar.png",
			fields: tConfig{
				AccessKey: os.Getenv("AWS_S3_ACCESS_KEY"),
				SecretKey: os.Getenv("AWS_S3_SECRET_KEY"),
				Endpoint:  os.Getenv("AWS_S3_ENDPOINT"),
				Region:    os.Getenv("AWS_S3_REGION"),
			},
			args: args{
				UploadInput{
					Bucket:      os.Getenv("AWS_S3_BUCKET"),
					ObjectKey:   "xyz/image.png",
					ContentType: "image/png",
					FileName:    "avatar.png",
					Body:        testPng,
				},
			},
			wantErr: false,
		},
		{
			name: "Upload special filename txt",
			fields: tConfig{
				AccessKey: os.Getenv("AWS_S3_ACCESS_KEY"),
				SecretKey: os.Getenv("AWS_S3_SECRET_KEY"),
				Endpoint:  os.Getenv("AWS_S3_ENDPOINT"),
				Region:    os.Getenv("AWS_S3_REGION"),
			},
			args: args{
				UploadInput{
					Bucket:      os.Getenv("AWS_S3_BUCKET"),
					ObjectKey:   "xyz/example file%with$special&chars(1)?.txt",
					ContentType: "text/plain",
					FileName:    "example file%with$special&chars(1)?.txt",
					Body:        testTxtSpecialChars,
				},
			},
			wantErr: false,
		},
	}
	for _, testcase := range tests {
		tt := testcase
		t.Run(tt.name+"_post", func(t *testing.T) {
			s3 := New(tt.fields.Region, tt.fields.AccessKey, tt.fields.SecretKey)
			s3.SetEndpoint(tt.fields.Endpoint)

			resp, err := s3.FileUpload(tt.args.u)
			if (err != nil) != tt.wantErr {
				t.Errorf("S3.FileUpload() error = %v, wantErr %v", err, tt.wantErr)
			}

			// reset file, to reuse in further tests.
			tt.args.u.Body.Seek(0, 0)

			// check for empty response
			if (resp == UploadResponse{}) {
				t.Errorf("S3.FileUpload() returned empty response, %v", resp)
			}

			if tt.testDetails {
				dResp, err := s3.FileDetails(DetailsInput{
					Bucket:    tt.args.u.Bucket,
					ObjectKey: tt.args.u.ObjectKey,
				})

				if (err != nil) != tt.wantErr {
					t.Errorf("S3.FileDetails() error = %v, wantErr %v", err, tt.wantErr)
				}

				if len(dResp.AmzMeta) != len(tt.args.u.CustomMetadata) {
					t.Errorf("S3.FileDetails() returned incorrect metadata, got: %#v", dResp)
				}
			}
		})
		t.Run(tt.name+"_put", func(t *testing.T) {
			s3 := New(tt.fields.Region, tt.fields.AccessKey, tt.fields.SecretKey)
			s3.SetEndpoint(tt.fields.Endpoint)

			resp, err := s3.FilePut(tt.args.u)
			if (err != nil) != tt.wantErr {
				t.Errorf("S3.FileUpload() error = %v, wantErr %v", err, tt.wantErr)
			}

			// reset file, to reuse in further tests.
			tt.args.u.Body.Seek(0, 0)

			// check for empty response
			if resp.ETag == "" {
				t.Errorf("S3.FileUpload() returned empty response, %v", resp)
			}

			if tt.testDetails {
				dResp, err := s3.FileDetails(DetailsInput{
					Bucket:    tt.args.u.Bucket,
					ObjectKey: tt.args.u.ObjectKey,
				})

				if (err != nil) != tt.wantErr {
					t.Errorf("S3.FileUpload() error = %v, wantErr %v", err, tt.wantErr)
				}

				if len(dResp.AmzMeta) != len(tt.args.u.CustomMetadata) {
					t.Errorf("S3.FileDetails() returned incorrect metadata, got: %#v", dResp)
				}
			}
		})
	}
}

func TestS3_FileDownload(t *testing.T) {
	testTxt, err := os.Open("testdata/test.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer testTxt.Close()
	testTxtData, err := io.ReadAll(testTxt)
	if err != nil {
		t.Fatal(err)
	}

	testPng, err := os.Open("testdata/avatar.png")
	if err != nil {
		t.Fatal(err)
	}
	defer testPng.Close()
	testPngData, err := io.ReadAll(testPng)
	if err != nil {
		t.Fatal(err)
	}

	type args struct {
		u DownloadInput
	}
	tests := []struct {
		name         string
		fields       tConfig
		args         args
		wantErr      bool
		wantResponse []byte
	}{
		{
			name: "txt",
			fields: tConfig{
				AccessKey: os.Getenv("AWS_S3_ACCESS_KEY"),
				SecretKey: os.Getenv("AWS_S3_SECRET_KEY"),
				Endpoint:  os.Getenv("AWS_S3_ENDPOINT"),
				Region:    os.Getenv("AWS_S3_REGION"),
			},
			args: args{
				u: DownloadInput{
					Bucket:    os.Getenv("AWS_S3_BUCKET"),
					ObjectKey: "test.txt",
				},
			},
			wantErr:      false,
			wantResponse: testTxtData,
		},
		{
			name: "png",
			fields: tConfig{
				AccessKey: os.Getenv("AWS_S3_ACCESS_KEY"),
				SecretKey: os.Getenv("AWS_S3_SECRET_KEY"),
				Endpoint:  os.Getenv("AWS_S3_ENDPOINT"),
				Region:    os.Getenv("AWS_S3_REGION"),
			},
			args: args{
				u: DownloadInput{
					Bucket:    os.Getenv("AWS_S3_BUCKET"),
					ObjectKey: "xyz/image.png",
				},
			},
			wantErr:      false,
			wantResponse: testPngData,
		},
		{
			name: "txt-special-filename",
			fields: tConfig{
				AccessKey: os.Getenv("AWS_S3_ACCESS_KEY"),
				SecretKey: os.Getenv("AWS_S3_SECRET_KEY"),
				Endpoint:  os.Getenv("AWS_S3_ENDPOINT"),
				Region:    os.Getenv("AWS_S3_REGION"),
			},
			args: args{
				u: DownloadInput{
					Bucket:    os.Getenv("AWS_S3_BUCKET"),
					ObjectKey: "xyz/example file%with$special&chars(1)?.txt",
				},
			},
			wantErr:      false,
			wantResponse: testTxtData,
		},
	}

	for _, testcase := range tests {
		tt := testcase
		t.Run(tt.name, func(t *testing.T) {
			s3 := New(tt.fields.Region, tt.fields.AccessKey, tt.fields.SecretKey)
			s3.SetEndpoint(tt.fields.Endpoint)
			resp, err := s3.FileDownload(tt.args.u)
			if (err != nil) != tt.wantErr {
				t.Fatalf("S3.FileDownload() error = %v, wantErr %v", err, tt.wantErr)
			}

			got, err := io.ReadAll(resp)
			if err != nil {
				t.Fatalf("error = %v", err)
			}

			resp.Close()

			if !bytes.Equal(got, tt.wantResponse) {
				t.Fatalf("S3.FileDownload() = %v, want %v", got, tt.wantResponse)
			}
		})
	}
}

func TestS3_FileDelete(t *testing.T) {
	type args struct {
		u DeleteInput
	}
	tests := []struct {
		name    string
		fields  tConfig
		args    args
		wantErr bool
	}{
		{
			name: "Delete test.txt",
			fields: tConfig{
				AccessKey: os.Getenv("AWS_S3_ACCESS_KEY"),
				SecretKey: os.Getenv("AWS_S3_SECRET_KEY"),
				Endpoint:  os.Getenv("AWS_S3_ENDPOINT"),
				Region:    os.Getenv("AWS_S3_REGION"),
			},
			args: args{
				DeleteInput{
					Bucket:    os.Getenv("AWS_S3_BUCKET"),
					ObjectKey: "test.txt",
				},
			},
			wantErr: false,
		},
		{
			name: "Delete test_metadata.txt",
			fields: tConfig{
				AccessKey: os.Getenv("AWS_S3_ACCESS_KEY"),
				SecretKey: os.Getenv("AWS_S3_SECRET_KEY"),
				Endpoint:  os.Getenv("AWS_S3_ENDPOINT"),
				Region:    os.Getenv("AWS_S3_REGION"),
			},
			args: args{
				DeleteInput{
					Bucket:    os.Getenv("AWS_S3_BUCKET"),
					ObjectKey: "test_metadata.txt",
				},
			},
			wantErr: false,
		},
		{
			name: "Delete avatar.png",
			fields: tConfig{
				AccessKey: os.Getenv("AWS_S3_ACCESS_KEY"),
				SecretKey: os.Getenv("AWS_S3_SECRET_KEY"),
				Endpoint:  os.Getenv("AWS_S3_ENDPOINT"),
				Region:    os.Getenv("AWS_S3_REGION"),
			},
			args: args{
				DeleteInput{
					Bucket:    os.Getenv("AWS_S3_BUCKET"),
					ObjectKey: "xyz/image.png",
				},
			},
			wantErr: false,
		},
		{
			name: "Delete special filename txt",
			fields: tConfig{
				AccessKey: os.Getenv("AWS_S3_ACCESS_KEY"),
				SecretKey: os.Getenv("AWS_S3_SECRET_KEY"),
				Endpoint:  os.Getenv("AWS_S3_ENDPOINT"),
				Region:    os.Getenv("AWS_S3_REGION"),
			},
			args: args{
				DeleteInput{
					Bucket:    os.Getenv("AWS_S3_BUCKET"),
					ObjectKey: "xyz/example file%with$special&chars(1)?.txt",
				},
			},
			wantErr: false,
		},
	}
	for _, testcase := range tests {
		tt := testcase
		t.Run(tt.name, func(t *testing.T) {
			s3 := New(tt.fields.Region, tt.fields.AccessKey, tt.fields.SecretKey)
			s3.SetEndpoint(tt.fields.Endpoint)
			if err := s3.FileDelete(tt.args.u); (err != nil) != tt.wantErr {
				t.Errorf("S3.FileDelete() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCopyObject(t *testing.T) {
	s3 := setupTestS3(t)
	testBucket := os.Getenv("AWS_S3_BUCKET")

	// Setup: upload source file
	sourceKey := fmt.Sprintf("test-copy-source-%d.txt", time.Now().Unix())
	destKey := fmt.Sprintf("test-copy-dest-%d.txt", time.Now().Unix())

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

	t.Run("CopyWithinBucket", func(t *testing.T) {
		output, err := s3.CopyObject(CopyObjectInput{
			SourceBucket: testBucket,
			SourceKey:    sourceKey,
			DestBucket:   testBucket,
			DestKey:      destKey,
		})
		if err != nil {
			t.Fatalf("CopyObject failed: %v", err)
		}
		if output.ETag == "" {
			t.Error("Expected non-empty ETag")
		}
		t.Logf("Copied to %s, ETag: %s, LastModified: %s", destKey, output.ETag, output.LastModified)
	})

	t.Run("VerifyCopiedObject", func(t *testing.T) {
		details, err := s3.FileDetails(DetailsInput{
			Bucket:    testBucket,
			ObjectKey: destKey,
		})
		if err != nil {
			t.Errorf("Copied object not found: %v", err)
			return
		}
		t.Logf("Copied object details: size=%s, type=%s", details.ContentLength, details.ContentType)

		// Verify content matches
		body, err := s3.FileDownload(DownloadInput{
			Bucket:    testBucket,
			ObjectKey: destKey,
		})
		if err != nil {
			t.Errorf("Failed to download copied object: %v", err)
			return
		}
		defer body.Close()

		content, err := io.ReadAll(body)
		if err != nil {
			t.Errorf("Failed to read copied object: %v", err)
			return
		}

		if string(content) != "test content for copy" {
			t.Errorf("Copied content mismatch: got %q, want %q", string(content), "test content for copy")
		}
	})
}

func TestCopyObject_EmptySource(t *testing.T) {
	s3 := setupTestS3(t)

	_, err := s3.CopyObject(CopyObjectInput{
		SourceBucket: "",
		SourceKey:    "",
		DestBucket:   "dest",
		DestKey:      "key",
	})
	if err == nil {
		t.Error("Expected error for empty source bucket and key")
	}
	if !strings.Contains(err.Error(), "source bucket and key are required") {
		t.Errorf("Expected 'source bucket and key are required' error, got: %v", err)
	}
}

func TestCopyObject_EmptyDest(t *testing.T) {
	s3 := setupTestS3(t)

	_, err := s3.CopyObject(CopyObjectInput{
		SourceBucket: "source",
		SourceKey:    "key",
		DestBucket:   "",
		DestKey:      "",
	})
	if err == nil {
		t.Error("Expected error for empty destination bucket and key")
	}
	if !strings.Contains(err.Error(), "destination bucket and key are required") {
		t.Errorf("Expected 'destination bucket and key are required' error, got: %v", err)
	}
}

func TestDeleteObjects(t *testing.T) {
	s3 := setupTestS3(t)
	testBucket := os.Getenv("AWS_S3_BUCKET")
	timestamp := time.Now().Unix()

	// Setup: upload multiple files
	keys := []string{
		fmt.Sprintf("batch-delete-%d-1.txt", timestamp),
		fmt.Sprintf("batch-delete-%d-2.txt", timestamp),
		fmt.Sprintf("batch-delete-%d-3.txt", timestamp),
	}

	for _, key := range keys {
		_, err := s3.FilePut(UploadInput{
			Bucket:      testBucket,
			ObjectKey:   key,
			ContentType: "text/plain",
			Body:        strings.NewReader("test"),
		})
		if err != nil {
			t.Fatalf("Failed to upload %s: %v", key, err)
		}
	}

	t.Run("DeleteMultiple", func(t *testing.T) {
		output, err := s3.DeleteObjects(DeleteObjectsInput{
			Bucket:  testBucket,
			Objects: keys,
			Quiet:   false,
		})
		if err != nil {
			t.Fatalf("DeleteObjects failed: %v", err)
		}

		if len(output.Deleted) != len(keys) {
			t.Errorf("Expected %d deleted, got %d", len(keys), len(output.Deleted))
		}
		if len(output.Errors) > 0 {
			t.Errorf("Unexpected errors: %+v", output.Errors)
		}
		t.Logf("Successfully deleted %d objects", len(output.Deleted))
		for _, deleted := range output.Deleted {
			t.Logf("  - Deleted: %s", deleted.Key)
		}
	})
}

func TestDeleteObjects_Validation(t *testing.T) {
	s3 := setupTestS3(t)

	t.Run("EmptyBucket", func(t *testing.T) {
		_, err := s3.DeleteObjects(DeleteObjectsInput{
			Bucket:  "",
			Objects: []string{"key"},
		})
		if err == nil {
			t.Error("Expected error for empty bucket")
		}
		if !strings.Contains(err.Error(), "bucket name is required") {
			t.Errorf("Expected 'bucket name is required' error, got: %v", err)
		}
	})

	t.Run("EmptyObjects", func(t *testing.T) {
		_, err := s3.DeleteObjects(DeleteObjectsInput{
			Bucket:  "bucket",
			Objects: []string{},
		})
		if err == nil {
			t.Error("Expected error for empty objects")
		}
		if !strings.Contains(err.Error(), "at least one object key is required") {
			t.Errorf("Expected 'at least one object key is required' error, got: %v", err)
		}
	})

	t.Run("TooManyObjects", func(t *testing.T) {
		keys := make([]string, 1001)
		for i := range keys {
			keys[i] = fmt.Sprintf("key-%d", i)
		}

		_, err := s3.DeleteObjects(DeleteObjectsInput{
			Bucket:  "bucket",
			Objects: keys,
		})
		if err == nil {
			t.Error("Expected error for >1000 objects")
		}
		if !strings.Contains(err.Error(), "cannot delete more than 1000 objects per request") {
			t.Errorf("Expected '1000 objects' error, got: %v", err)
		}
	})
}
