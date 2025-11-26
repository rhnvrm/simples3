package simples3

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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
