package simples3

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestS3_NewUsingIAM(t *testing.T) {
	unsetEnvForTest(t, ecsContainerCredentialsEnv)

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

func TestFetchIAMDataForEcs(t *testing.T) {
	oldBaseURL := ecsContainerCredentialsBaseURL
	ecsContainerCredentialsBaseURL = ""
	t.Cleanup(func() {
		ecsContainerCredentialsBaseURL = oldBaseURL
	})

	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Fatalf("expected GET request, got %s", r.Method)
			}
			if r.URL.EscapedPath() != "/ecs/creds" {
				t.Fatalf("unexpected path: %s", r.URL.EscapedPath())
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, `{"AccessKeyId":"ecs-access","SecretAccessKey":"ecs-secret","Token":"ecs-token","Expiration":"2018-12-24T16:24:59Z"}`)
		}))
		defer server.Close()

		ecsContainerCredentialsBaseURL = server.URL
		os.Setenv(ecsContainerCredentialsEnv, "/ecs/creds")
		defer os.Unsetenv(ecsContainerCredentialsEnv)

		resp, err := fetchIAMDataForEcs(server.Client())
		if err != nil {
			t.Fatalf("fetchIAMDataForEcs() error = %v", err)
		}

		if resp.AccessKeyID != "ecs-access" || resp.SecretAccessKey != "ecs-secret" || resp.Token != "ecs-token" {
			t.Fatalf("unexpected ECS credentials: %+v", resp)
		}
	})

	t.Run("non-200 does not fall back to IMDS", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			io.WriteString(w, `forbidden`)
		}))
		defer server.Close()

		ecsContainerCredentialsBaseURL = server.URL
		os.Setenv(ecsContainerCredentialsEnv, "/ecs/creds")
		defer os.Unsetenv(ecsContainerCredentialsEnv)

		_, err := fetchIAMData(server.Client(), "http://should-not-be-used")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error fetching IAM ECS data") {
			t.Fatalf("expected ECS error, got %v", err)
		}
	})
}

func TestFetchIAMDataFallsBackToIMDSWhenECSIsUnavailable(t *testing.T) {
	unsetEnvForTest(t, ecsContainerCredentialsEnv)

	var (
		iam           = `test-new-s3-using-iam`
		resp          = `{"Code":"Success","LastUpdated":"2018-12-24T10:18:01Z","Type":"AWS-HMAC","AccessKeyId":"abc","SecretAccessKey":"abc","Token":"abc","Expiration":"2018-12-24T16:24:59Z"}`
		respIMDSToken = `AQAEAJWopi8yvjKYXyWJbzESE0cms-OoTnptJzS3M9g5iNcl06UEkQ==`
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			if r.URL.EscapedPath() != imdsTokenURI {
				t.Fatalf("unexpected token path: %s", r.URL.EscapedPath())
			}
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, respIMDSToken)
		case http.MethodGet:
			if r.Header.Get(imdsTokenHeader) == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			switch r.URL.EscapedPath() {
			case securityCredentialsURI:
				w.WriteHeader(http.StatusOK)
				io.WriteString(w, iam)
			case securityCredentialsURI + iam:
				w.WriteHeader(http.StatusOK)
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, resp)
			default:
				t.Fatalf("unexpected IMDS path: %s", r.URL.EscapedPath())
			}
		default:
			t.Fatalf("unexpected method: %s", r.Method)
		}
	}))
	defer server.Close()

	respData, err := fetchIAMData(server.Client(), server.URL)
	if err != nil {
		t.Fatalf("fetchIAMData() error = %v", err)
	}

	if respData.AccessKeyID != "abc" || respData.SecretAccessKey != "abc" || respData.Token != "abc" {
		t.Fatalf("unexpected IMDS credentials: %+v", respData)
	}
	if respData.Code != "Success" || respData.Type != "AWS-HMAC" {
		t.Fatalf("expected legacy IAMResponse fields to remain populated, got %+v", respData)
	}
}

func unsetEnvForTest(t *testing.T, key string) {
	t.Helper()

	oldValue, hadValue := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("failed to unset %s: %v", key, err)
	}

	t.Cleanup(func() {
		if hadValue {
			if err := os.Setenv(key, oldValue); err != nil {
				t.Fatalf("failed to restore %s: %v", key, err)
			}
			return
		}
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("failed to cleanup %s: %v", key, err)
		}
	})
}
