package simples3

import (
	"bytes"
	"io"
	"os"
	"testing"
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
