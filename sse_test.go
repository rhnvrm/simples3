package simples3

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

var testSSECKey = []byte("0123456789abcdef0123456789abcdef")

func checkSSECRequest(t *testing.T, s3 *S3, r *http.Request, key []byte, source bool) {
	t.Helper()
	headers, err := SSECustomerHeaders(key)
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range headers {
		if source {
			k = strings.Replace(k, "x-amz-", "x-amz-copy-source-", 1)
		}
		if r.Header.Get(k) != want {
			t.Errorf("incorrect %s header", k)
		}
		if !strings.Contains(r.Header.Get("Authorization"), strings.ToLower(k)) {
			t.Errorf("unsigned %s", k)
		}
	}
	// Independently compare the signature of the received request (including its
	// payload and SSE-C headers) with the signature sent by the client.
	auth := r.Header.Get("Authorization")
	clone := r.Clone(r.Context())
	signed := strings.Split(strings.Split(auth, "SignedHeaders=")[1], ",")[0]
	for k := range clone.Header {
		if !strings.Contains(";"+signed+";", ";"+strings.ToLower(k)+";") {
			clone.Header.Del(k)
		}
	}
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(body))
	clone.Body = io.NopCloser(bytes.NewReader(body))
	date, err := time.Parse(amzDateISO8601TimeFormat, r.Header.Get("X-Amz-Date"))
	if err != nil {
		t.Fatal(err)
	}
	hash := hmac.New(sha256.New, s3.signKeys(date))
	s3.writeStringToSign(hash, date, clone)
	if !strings.HasSuffix(auth, fmt.Sprintf("Signature=%x", hash.Sum(nil))) {
		t.Error("request signature does not cover received headers/body")
	}
}

func TestSSECHeaders(t *testing.T) {
	headers, err := SSECustomerHeaders(testSSECKey)
	if err != nil {
		t.Fatal(err)
	}
	if headers[sseCustomerPrefix+"key"] != "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=" {
		t.Error("incorrect key encoding")
	}
	// Digest is of raw key bytes, not their base64 representation.
	if headers[sseCustomerPrefix+"key-MD5"] != "hRasmdxgYDKV3nvbahU1MA==" {
		t.Errorf("incorrect MD5: %s", headers[sseCustomerPrefix+"key-MD5"])
	}
	for _, size := range []int{0, 16, 31, 33} {
		if _, err := SSECustomerHeaders(make([]byte, size)); err == nil {
			t.Errorf("accepted %d-byte key", size)
		}
	}
}

func TestSSECObjectOperations(t *testing.T) {
	s3 := New("us-east-1", "access", "secret")
	var stored []byte
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Reject absent/wrong keys as a real SSE-C endpoint would.
		if r.Header.Get(sseCustomerPrefix+"key") != base64.StdEncoding.EncodeToString(testSSECKey) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		checkSSECRequest(t, s3, r, testSSECKey, false)
		switch r.Method {
		case http.MethodPut:
			stored, _ = io.ReadAll(r.Body)
			w.Header().Set("ETag", `"etag"`)
		case http.MethodHead:
			w.Header().Set("Content-Length", fmt.Sprint(len(stored)))
		case http.MethodGet:
			w.Write(stored)
		}
	}))
	defer server.Close()
	s3.SetEndpoint(server.URL).SetClient(server.Client())
	if _, err := s3.FilePut(UploadInput{Bucket: "bucket", ObjectKey: "key", Body: strings.NewReader("encrypted content"), SSECustomerKey: testSSECKey}); err != nil {
		t.Fatal(err)
	}
	if _, err := s3.FileDetails(DetailsInput{Bucket: "bucket", ObjectKey: "key", SSECustomerKey: testSSECKey}); err != nil {
		t.Fatal(err)
	}
	body, err := s3.FileDownload(DownloadInput{Bucket: "bucket", ObjectKey: "key", SSECustomerKey: testSSECKey})
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	got, _ := io.ReadAll(body)
	if string(got) != "encrypted content" {
		t.Errorf("unexpected content: %q", got)
	}
	for _, key := range [][]byte{nil, bytes.Repeat([]byte{1}, 32)} {
		if _, err := s3.FileDownload(DownloadInput{Bucket: "bucket", ObjectKey: "key", SSECustomerKey: key}); err == nil {
			t.Error("download unexpectedly succeeded with missing/wrong key")
		}
		if _, err := s3.FileDetails(DetailsInput{Bucket: "bucket", ObjectKey: "key", SSECustomerKey: key}); err == nil {
			t.Error("HEAD unexpectedly succeeded with missing/wrong key")
		}
	}
}

func TestSSECCopy(t *testing.T) {
	sourceKey := bytes.Repeat([]byte{1}, 32)
	s3 := New("us-east-1", "access", "secret")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checkSSECRequest(t, s3, r, testSSECKey, false)
		checkSSECRequest(t, s3, r, sourceKey, true)
		io.WriteString(w, `<CopyObjectResult><ETag>copy-etag</ETag></CopyObjectResult>`)
	}))
	defer server.Close()
	s3.SetEndpoint(server.URL).SetClient(server.Client())
	_, err := s3.CopyObject(CopyObjectInput{SourceBucket: "bucket", SourceKey: "source", DestBucket: "bucket", DestKey: "dest", SSECustomerKey: testSSECKey, SourceSSECustomerKey: sourceKey})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSSECPost(t *testing.T) {
	s3 := New("us-east-1", "access", "secret")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1024 * 1024); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		defer r.MultipartForm.RemoveAll()
		raw, err := base64.StdEncoding.DecodeString(r.FormValue("Policy"))
		if err != nil {
			t.Error(err)
		}
		var policy PolicyJSON
		if err := json.Unmarshal(raw, &policy); err != nil {
			t.Error(err)
		}
		headers, _ := SSECustomerHeaders(testSSECKey)
		for k, v := range headers {
			if r.FormValue(k) != v {
				t.Errorf("missing form field %s", k)
			}
			found := false
			for _, condition := range policy.Conditions {
				if m, ok := condition.(map[string]interface{}); ok && m[k] == v {
					found = true
				}
			}
			if !found {
				t.Errorf("missing signed policy condition %s", k)
			}
		}
		w.WriteHeader(http.StatusCreated)
		io.WriteString(w, `<PostResponse><Key>key</Key></PostResponse>`)
	}))
	defer server.Close()
	s3.SetEndpoint(server.URL).SetClient(server.Client())
	if _, err := s3.FileUpload(UploadInput{Bucket: "bucket", ObjectKey: "key", FileName: "key", Body: strings.NewReader("content"), SSECustomerKey: testSSECKey}); err != nil {
		t.Fatal(err)
	}
}

func TestSSECMultipart(t *testing.T) {
	for _, concurrency := range []int{1, 2} {
		t.Run(fmt.Sprint(concurrency), func(t *testing.T) {
			s3 := New("us-east-1", "access", "secret")
			var mu sync.Mutex
			attempts := map[string]int{}
			complete := false
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				checkSSECRequest(t, s3, r, testSSECKey, false)
				switch {
				case r.Method == http.MethodPost && r.URL.Query().Has("uploads"):
					io.WriteString(w, `<InitiateMultipartUploadResult><UploadId>upload</UploadId></InitiateMultipartUploadResult>`)
				case r.Method == http.MethodPut:
					part := r.URL.Query().Get("partNumber")
					data, _ := io.ReadAll(r.Body)
					want := int(MinPartSize)
					if part == "2" {
						want = 1
					}
					if len(data) != want {
						t.Errorf("part %s length = %d, want %d", part, len(data), want)
					}
					mu.Lock()
					attempts[part]++
					n := attempts[part]
					mu.Unlock()
					if part == "1" && n == 1 {
						w.WriteHeader(503)
						return
					}
					w.Header().Set("ETag", `"part-etag"`)
				case r.Method == http.MethodPost:
					mu.Lock()
					complete = true
					mu.Unlock()
					io.WriteString(w, `<CompleteMultipartUploadResult><Key>key</Key><ETag>etag</ETag></CompleteMultipartUploadResult>`)
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
					w.WriteHeader(400)
				}
			}))
			defer server.Close()
			s3.SetEndpoint(server.URL).SetClient(server.Client())
			_, err := s3.FileUploadMultipart(MultipartUploadInput{Bucket: "bucket", ObjectKey: "key", Body: bytes.NewReader(bytes.Repeat([]byte{'a'}, int(MinPartSize)+1)), SSECustomerKey: testSSECKey, Concurrency: concurrency})
			if err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			defer mu.Unlock()
			if !complete || attempts["1"] != 2 || attempts["2"] != 1 {
				t.Errorf("incomplete workflow: complete=%v attempts=%v", complete, attempts)
			}
		})
	}
}

func TestSSECValidation(t *testing.T) {
	s3 := New("us-east-1", "access", "secret").SetEndpoint("http://localhost:1")
	for _, tc := range []struct {
		name            string
		key             []byte
		encryption, kms string
	}{
		{"short", []byte("invalid"), "", ""}, {"long", make([]byte, 33), "", ""},
		{"SSE-S3", testSSECKey, "AES256", ""}, {"SSE-KMS", testSSECKey, "", "kms-key"},
		{"HTTP", testSSECKey, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := []func() error{
				func() error {
					_, err := s3.FilePut(UploadInput{Bucket: "b", ObjectKey: "k", Body: strings.NewReader("x"), SSECustomerKey: tc.key, ServerSideEncryption: tc.encryption, SSEKMSKeyId: tc.kms})
					return err
				},
				func() error {
					_, err := s3.FileUpload(UploadInput{Bucket: "b", ObjectKey: "k", Body: strings.NewReader("x"), SSECustomerKey: tc.key, ServerSideEncryption: tc.encryption, SSEKMSKeyId: tc.kms})
					return err
				},
				func() error {
					_, err := s3.InitiateMultipartUpload(InitiateMultipartUploadInput{Bucket: "b", ObjectKey: "k", SSECustomerKey: tc.key, ServerSideEncryption: tc.encryption, SSEKMSKeyId: tc.kms})
					return err
				},
				func() error {
					_, err := s3.CopyObject(CopyObjectInput{SourceBucket: "b", SourceKey: "s", DestBucket: "b", DestKey: "d", SSECustomerKey: tc.key, ServerSideEncryption: tc.encryption, SSEKMSKeyId: tc.kms})
					return err
				},
			}
			for _, call := range calls {
				err := call()
				if err == nil || !strings.Contains(err.Error(), "SSE-C") {
					t.Errorf("expected SSE-C validation error, got %v", err)
				}
			}
		})
	}
}

func TestSSECPresigned(t *testing.T) {
	headers, _ := SSECustomerHeaders(testSSECKey)
	s3 := New("us-east-1", "access", "secret")
	for _, endpoint := range []string{"https://example.com", "http://example.com"} {
		s3.SetEndpoint(endpoint)
		urls := []string{s3.GeneratePresignedURL(PresignedInput{Bucket: "b", ObjectKey: "k", Method: "GET", ExpirySeconds: 60, ExtraHeaders: headers}), s3.GeneratePresignedUploadPartURL(PresignedMultipartInput{Bucket: "b", ObjectKey: "k", UploadID: "upload", PartNumber: 1, ExtraHeaders: headers})}
		for _, raw := range urls {
			if strings.HasPrefix(endpoint, "http:") {
				if raw != "" {
					t.Error("accepted HTTP SSE-C presigning")
				}
				continue
			}
			u, err := url.Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			signed := u.Query().Get("X-Amz-SignedHeaders")
			for k, v := range headers {
				if !strings.Contains(signed, strings.ToLower(k)) {
					t.Errorf("unsigned header %s", k)
				}
				if strings.Contains(raw, v) || strings.Contains(raw, url.QueryEscape(v)) {
					t.Error("SSE-C value leaked into URL")
				}
			}
		}
	}
}

func TestSSECRedirects(t *testing.T) {
	for _, target := range []string{"http://example.com/key", "https://other.example/key"} {
		t.Run(target, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, target, http.StatusTemporaryRedirect)
			}))
			defer server.Close()
			called := false
			client := server.Client()
			client.CheckRedirect = func(*http.Request, []*http.Request) error { called = true; return nil }
			s3 := New("us-east-1", "access", "secret").SetEndpoint(server.URL).SetClient(client)
			if _, err := s3.FileDownload(DownloadInput{Bucket: "b", ObjectKey: "k", SSECustomerKey: testSSECKey}); err == nil {
				t.Error("allowed unsafe redirect")
			}
			if called {
				t.Error("unsafe redirect reached caller policy")
			}
		})
	}
}

func TestSSECReadAndMultipartValidation(t *testing.T) {
	s3 := New("us-east-1", "access", "secret").SetEndpoint("http://localhost:1")
	for _, key := range [][]byte{[]byte("short"), testSSECKey} {
		calls := []func() error{
			func() error {
				_, err := s3.FileDownload(DownloadInput{Bucket: "b", ObjectKey: "k", SSECustomerKey: key})
				return err
			},
			func() error {
				_, err := s3.FileDetails(DetailsInput{Bucket: "b", ObjectKey: "k", SSECustomerKey: key})
				return err
			},
			func() error {
				_, err := s3.UploadPart(UploadPartInput{Bucket: "b", ObjectKey: "k", UploadID: "id", PartNumber: 1, Body: strings.NewReader("x"), Size: 1, SSECustomerKey: key})
				return err
			},
			func() error {
				_, err := s3.CompleteMultipartUpload(CompleteMultipartUploadInput{Bucket: "b", ObjectKey: "k", UploadID: "id", Parts: []CompletedPart{{PartNumber: 1, ETag: "etag"}}, SSECustomerKey: key})
				return err
			},
			func() error {
				_, err := s3.FileUploadMultipart(MultipartUploadInput{Bucket: "b", ObjectKey: "k", Body: strings.NewReader("x"), SSECustomerKey: key})
				return err
			},
			func() error {
				_, err := s3.CopyObject(CopyObjectInput{SourceBucket: "b", SourceKey: "s", DestBucket: "b", DestKey: "d", SourceSSECustomerKey: key})
				return err
			},
		}
		for _, call := range calls {
			err := call()
			if err == nil || !strings.Contains(err.Error(), "SSE-C") {
				t.Errorf("expected SSE-C validation error, got %v", err)
			}
		}
	}
}

func TestSSECSameOriginRedirectPolicy(t *testing.T) {
	called := false
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/b/k" {
			http.Redirect(w, r, "/final", http.StatusTemporaryRedirect)
			return
		}
		if r.Header.Get(sseCustomerPrefix+"key") == "" {
			t.Error("lost key on same-origin redirect")
		}
		io.WriteString(w, "content")
	}))
	defer server.Close()
	client := server.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { called = true; return nil }
	s3 := New("us-east-1", "access", "secret").SetEndpoint(server.URL).SetClient(client)
	body, err := s3.FileDownload(DownloadInput{Bucket: "b", ObjectKey: "k", SSECustomerKey: testSSECKey})
	if err != nil {
		t.Fatal(err)
	}
	body.Close()
	if !called {
		t.Error("caller redirect policy was not called")
	}
}

func TestSSECLoopbackProxySignature(t *testing.T) {
	s3 := New("us-east-1", "access", "secret")
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checkSSECRequest(t, s3, r, testSSECKey, false)
		w.Header().Set("ETag", "etag")
	}))
	defer backend.Close()
	s3.SetEndpoint(backend.URL)
	configureSSECLoopbackProxy(t, s3)
	if _, err := s3.FilePut(UploadInput{Bucket: "bucket", ObjectKey: "key", Body: strings.NewReader("content"), SSECustomerKey: testSSECKey}); err != nil {
		t.Fatal(err)
	}
}

func TestSSECCanonicalHeaderPrefixOrder(t *testing.T) {
	req, err := http.NewRequest(http.MethodPut, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Amz-Server-Side-Encryption-Customer-Key", "key")
	req.Header.Set("X-Amz-Server-Side-Encryption-Customer-Key-MD5", "checksum")
	var canonical bytes.Buffer
	writeHeader(&canonical, req)
	want := "x-amz-server-side-encryption-customer-key:key\nx-amz-server-side-encryption-customer-key-md5:checksum"
	if canonical.String() != want {
		t.Errorf("canonical headers = %q, want %q", canonical.String(), want)
	}
}
