package simples3

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

type tConfig struct {
	AccessKey string
	SecretKey string
	Endpoint  string
	Region    string
}

func TestS3_FileUploadPostAndPut(t *testing.T) {
	testTxt, err := os.Open("testdata/test.txt")
	if err != nil {
		return
	}
	defer testTxt.Close()
	// Note: cannot re-use the same file descriptor due to seeking!
	testTxtSpecialChars, err := os.Open("testdata/test.txt")
	if err != nil {
		return
	}
	defer testTxtSpecialChars.Close()
	testPng, err := os.Open("testdata/avatar.png")
	if err != nil {
		return
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
	testTxtData, err := ioutil.ReadAll(testTxt)
	if err != nil {
		t.Fatal(err)
	}

	testPng, err := os.Open("testdata/avatar.png")
	if err != nil {
		t.Fatal(err)
	}
	defer testPng.Close()
	testPngData, err := ioutil.ReadAll(testPng)
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

			got, err := ioutil.ReadAll(resp)
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

func TestS3_NewUsingIAM(t *testing.T) {
	var (
		iam  = `test-new-s3-using-iam`
		resp = `{"Code" : "Success","LastUpdated" : "2018-12-24T10:18:01Z",
				"Type" : "AWS-HMAC","AccessKeyId" : "abc",
				"SecretAccessKey" : "abc","Token" : "abc",
				"Expiration" : "2018-12-24T16:24:59Z"}`
		respIMDSToken = `AQAEAJWopi8yvjKYXyWJbzESE0cms-OoTnptJzS3M9g5iNcl06UEkQ==`
	)

	tsFail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer tsFail.Close()

	genServerHandlerFunc := func(failIMDS bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case "GET":
				if !failIMDS {
					// check if token is present
					if r.Header.Get(imdsTokenHeader) == "" {
						w.WriteHeader(http.StatusUnauthorized)
						return
					}
				}

				url := securityCredentialsURI
				if r.URL.EscapedPath() == url {
					w.WriteHeader(http.StatusOK)
					io.WriteString(w, iam)
				}
				if r.URL.EscapedPath() == url+iam {
					w.WriteHeader(http.StatusOK)
					w.Header().Set("Content-Type", "application/json")
					io.WriteString(w, resp)
				}
			case "PUT":
				if failIMDS {
					w.WriteHeader(http.StatusNotFound)
					return
				}

				if r.URL.EscapedPath() == imdsTokenURI {
					if r.Header.Get(imdsTokenTtlHeader) != "60" {
						w.WriteHeader(http.StatusBadRequest)
						return
					}

					w.WriteHeader(http.StatusOK)
					io.WriteString(w, respIMDSToken)
				}
			default:
				t.Errorf("Expected 'GET' or 'PUT' request, got '%s'", r.Method)
			}
		}
	}

	ts := httptest.NewServer(http.HandlerFunc(genServerHandlerFunc(false)))
	defer ts.Close()

	tsFailIMDS := httptest.NewServer(http.HandlerFunc(genServerHandlerFunc(true)))
	defer tsFailIMDS.Close()

	cl := &http.Client{Timeout: 1 * time.Second}

	// Test for timeout.
	_, err := newUsingIAM(cl, tsFail.URL, "abc")
	if err == nil {
		t.Errorf("Expected error, got nil")
	} else {
		var timeoutError net.Error

		if errors.As(err, &timeoutError) && !timeoutError.Timeout() {
			t.Errorf("newUsingIAM() timeout check. got error = %v", err)
		}
	}

	// Test for successful IAM fetch.
	s3, err := newUsingIAM(cl, ts.URL, "abc")
	if err != nil {
		t.Errorf("newUsingIAM() error = %v", err)
	}

	if s3 == nil {
		t.Errorf("newUsingIAM() got = %v", s3)
	}

	if s3.AccessKey != "abc" || s3.SecretKey != "abc" || s3.Region != "abc" {
		t.Errorf("S3.FileDelete() got = %v", s3)
	}

	// Test for failed IMDS token fetch.
	_, err = newUsingIAM(cl, tsFailIMDS.URL, "abc")
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
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
	s3 := New("us-east-1", "AccessKey", "SuperSecretKey")

	type args struct {
		bucket string
		params []string
	}

	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "getURL: basic test",
			args: args{
				bucket: "xyz",
			},
			want: "https://s3.us-east-1.amazonaws.com/xyz",
		},
		{
			name: "getURL: multiple parameters",
			args: args{
				bucket: "xyz",
				params: []string{"hello", "world"},
			},
			want: "https://s3.us-east-1.amazonaws.com/xyz/hello/world",
		},
		{
			name: "getURL: special characters",
			args: args{
				bucket: "xyz",
				params: []string{"hello, world!", "#!@$%^&*(1).txt"},
			},
			want: "https://s3.us-east-1.amazonaws.com/xyz/hello%2C%20world%21/%23%21%40%24%25%5E%26%2A%281%29.txt",
		},
	}

	for _, testcase := range tests {
		tt := testcase
		t.Run(tt.name, func(t *testing.T) {
			url := s3.getURL(tt.args.bucket, tt.args.params...)
			if url != tt.want {
				t.Errorf("S3.getURL() got = %v, want %v", url, tt.want)
			}
		})
	}
}
