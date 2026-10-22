package simples3

import (
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Verify client wire formats and response parsing independently of the integration
// backend. Real SeaweedFS integration tests exercise these features as well.
func TestBackendProtocol(t *testing.T) {
	acl := AccessControlPolicy{Owner: Owner{ID: "owner", DisplayName: "Owner"}, AccessControlList: []Grant{{Grantee: Grantee{Type: "CanonicalUser", ID: "owner"}, Permission: "FULL_CONTROL"}}}
	aclXML, _ := xml.Marshal(acl)
	tests := []struct {
		name, method, path, query, bodyContains, response string
		status                                            int
		headers                                           map[string]string
		call                                              func(*S3) error
	}{
		{name: "put bucket canned ACL", method: "PUT", path: "/bucket", query: "acl", headers: map[string]string{"x-amz-acl": "public-read"}, call: func(s *S3) error {
			return s.PutBucketAcl(PutBucketAclInput{Bucket: "bucket", CannedACL: "public-read"})
		}},
		{name: "put bucket custom ACL", method: "PUT", path: "/bucket", query: "acl", bodyContains: "<Permission>FULL_CONTROL</Permission>", call: func(s *S3) error {
			return s.PutBucketAcl(PutBucketAclInput{Bucket: "bucket", AccessControlPolicy: &acl})
		}},
		{name: "get bucket ACL", method: "GET", path: "/bucket", query: "acl", response: string(aclXML), call: func(s *S3) error {
			out, err := s.GetBucketAcl("bucket")
			if err == nil && (out.Owner.ID != "owner" || len(out.AccessControlList) != 1) {
				return fmt.Errorf("bad ACL response: %+v", out)
			}
			return err
		}},
		{name: "put object canned ACL", method: "PUT", path: "/bucket/key", query: "acl", headers: map[string]string{"x-amz-acl": "private"}, call: func(s *S3) error {
			return s.PutObjectAcl(PutObjectAclInput{Bucket: "bucket", ObjectKey: "key", CannedACL: "private"})
		}},
		{name: "put object custom ACL", method: "PUT", path: "/bucket/key", query: "acl", bodyContains: "<Permission>FULL_CONTROL</Permission>", call: func(s *S3) error {
			return s.PutObjectAcl(PutObjectAclInput{Bucket: "bucket", ObjectKey: "key", AccessControlPolicy: &acl})
		}},
		{name: "get object ACL", method: "GET", path: "/bucket/key", query: "acl", response: string(aclXML), call: func(s *S3) error {
			out, err := s.GetObjectAcl(GetObjectAclInput{Bucket: "bucket", ObjectKey: "key"})
			if err == nil && (out.Owner.ID != "owner" || len(out.AccessControlList) != 1) {
				return fmt.Errorf("bad ACL response: %+v", out)
			}
			return err
		}},
		{name: "put tags", method: "PUT", path: "/bucket/key", query: "tagging", bodyContains: "<Key>env</Key><Value>test</Value>", call: func(s *S3) error {
			return s.PutObjectTagging(PutObjectTaggingInput{Bucket: "bucket", ObjectKey: "key", Tags: map[string]string{"env": "test"}})
		}},
		{name: "get tags", method: "GET", path: "/bucket/key", query: "tagging", response: `<Tagging><TagSet><Tag><Key>env</Key><Value>test</Value></Tag></TagSet></Tagging>`, call: func(s *S3) error {
			out, err := s.GetObjectTagging(GetObjectTaggingInput{Bucket: "bucket", ObjectKey: "key"})
			if err == nil && out.Tags["env"] != "test" {
				return fmt.Errorf("bad tags: %+v", out)
			}
			return err
		}},
		{name: "delete tags", method: "DELETE", path: "/bucket/key", query: "tagging", status: 204, call: func(s *S3) error {
			return s.DeleteObjectTagging(DeleteObjectTaggingInput{Bucket: "bucket", ObjectKey: "key"})
		}},
		{name: "enable versioning", method: "PUT", path: "/bucket", query: "versioning", bodyContains: "<Status>Enabled</Status>", call: func(s *S3) error {
			return s.PutBucketVersioning(PutBucketVersioningInput{Bucket: "bucket", Status: "Enabled"})
		}},
		{name: "get versioning", method: "GET", path: "/bucket", query: "versioning", response: `<VersioningConfiguration><Status>Enabled</Status></VersioningConfiguration>`, call: func(s *S3) error {
			out, err := s.GetBucketVersioning("bucket")
			if err == nil && out.Status != "Enabled" {
				return fmt.Errorf("bad versioning: %+v", out)
			}
			return err
		}},
		{name: "list versions", method: "GET", path: "/bucket", query: "versions", response: `<ListVersionsResult><Version><Key>key</Key><VersionId>v1</VersionId><IsLatest>true</IsLatest></Version><DeleteMarker><Key>old</Key><VersionId>v0</VersionId></DeleteMarker></ListVersionsResult>`, call: func(s *S3) error {
			out, err := s.ListVersions(ListVersionsInput{Bucket: "bucket"})
			if err == nil && (len(out.Versions) != 1 || out.Versions[0].VersionId != "v1" || len(out.DeleteMarkers) != 1) {
				return fmt.Errorf("bad versions: %+v", out)
			}
			return err
		}},
		{name: "PUT SSE-S3", method: "PUT", path: "/bucket/key", headers: map[string]string{"x-amz-server-side-encryption": "AES256"}, call: func(s *S3) error {
			_, err := s.FilePut(UploadInput{Bucket: "bucket", ObjectKey: "key", Body: strings.NewReader("content"), ServerSideEncryption: "AES256"})
			return err
		}},
		{name: "PUT SSE-KMS", method: "PUT", path: "/bucket/key", headers: map[string]string{"x-amz-server-side-encryption": "aws:kms", "x-amz-server-side-encryption-aws-kms-key-id": "kms-key"}, call: func(s *S3) error {
			_, err := s.FilePut(UploadInput{Bucket: "bucket", ObjectKey: "key", Body: strings.NewReader("content"), ServerSideEncryption: "aws:kms", SSEKMSKeyId: "kms-key"})
			return err
		}},
		{name: "copy SSE-KMS", method: "PUT", path: "/bucket/key", headers: map[string]string{"x-amz-server-side-encryption": "aws:kms", "x-amz-server-side-encryption-aws-kms-key-id": "kms-key", "x-amz-copy-source": "/bucket/source"}, response: `<CopyObjectResult><ETag>etag</ETag></CopyObjectResult>`, call: func(s *S3) error {
			_, err := s.CopyObject(CopyObjectInput{SourceBucket: "bucket", SourceKey: "source", DestBucket: "bucket", DestKey: "key", ServerSideEncryption: "aws:kms", SSEKMSKeyId: "kms-key"})
			return err
		}},
		{name: "initiate SSE-KMS multipart", method: "POST", path: "/bucket/key", query: "uploads", headers: map[string]string{"x-amz-server-side-encryption": "aws:kms", "x-amz-server-side-encryption-aws-kms-key-id": "kms-key"}, response: `<InitiateMultipartUploadResult><UploadId>id</UploadId></InitiateMultipartUploadResult>`, call: func(s *S3) error {
			out, err := s.InitiateMultipartUpload(InitiateMultipartUploadInput{Bucket: "bucket", ObjectKey: "key", ServerSideEncryption: "aws:kms", SSEKMSKeyId: "kms-key"})
			if err == nil && out.UploadID != "id" {
				return fmt.Errorf("bad upload ID")
			}
			return err
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Method != tc.method || r.URL.Path != tc.path {
					t.Errorf("got %s %s, want %s %s", r.Method, r.URL.Path, tc.method, tc.path)
				}
				if tc.query != "" && !r.URL.Query().Has(tc.query) {
					t.Errorf("missing query %s", tc.query)
				}
				if r.Header.Get("Authorization") == "" {
					t.Error("missing signature")
				}
				for k, v := range tc.headers {
					if r.Header.Get(k) != v {
						t.Errorf("%s = %q, want %q", k, r.Header.Get(k), v)
					}
					if !strings.Contains(r.Header.Get("Authorization"), strings.ToLower(k)) {
						t.Errorf("unsigned header %s", k)
					}
				}
				body, _ := io.ReadAll(r.Body)
				if tc.bodyContains != "" && !strings.Contains(string(body), tc.bodyContains) {
					t.Errorf("unexpected XML: %s", body)
				}
				if tc.status != 0 {
					w.WriteHeader(tc.status)
				}
				io.WriteString(w, tc.response)
			}))
			defer server.Close()
			s := New("us-east-1", "access", "secret").SetEndpoint(server.URL)
			if err := tc.call(s); err != nil {
				t.Fatal(err)
			}
			if requests != 1 {
				t.Errorf("got %d requests, want 1", requests)
			}
		})
	}
}

func TestBackendProtocolUploadPolicy(t *testing.T) {
	for _, encryption := range []string{"AES256", "aws:kms"} {
		t.Run(encryption, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				defer r.MultipartForm.RemoveAll()
				if r.FormValue("x-amz-server-side-encryption") != encryption {
					t.Error("missing encryption field")
				}
				policyBytes, err := base64.StdEncoding.DecodeString(r.FormValue("Policy"))
				if err != nil {
					t.Error(err)
				}
				var policy PolicyJSON
				if err := json.Unmarshal(policyBytes, &policy); err != nil {
					t.Error(err)
				}
				fields := map[string]string{"x-amz-server-side-encryption": encryption, "x-amz-tagging": "env=test"}
				if encryption == "aws:kms" {
					fields["x-amz-server-side-encryption-aws-kms-key-id"] = "kms-key"
				}
				for k, v := range fields {
					if r.FormValue(k) != v {
						t.Errorf("missing form field %s", k)
					}
					found := false
					for _, c := range policy.Conditions {
						if m, ok := c.(map[string]interface{}); ok && m[k] == v {
							found = true
						}
					}
					if !found {
						t.Errorf("missing signed condition %s", k)
					}
				}
				w.WriteHeader(201)
				io.WriteString(w, `<PostResponse><Key>key</Key></PostResponse>`)
			}))
			defer server.Close()
			s := New("us-east-1", "access", "secret").SetEndpoint(server.URL)
			kms := ""
			if encryption == "aws:kms" {
				kms = "kms-key"
			}
			_, err := s.FileUpload(UploadInput{Bucket: "bucket", ObjectKey: "key", FileName: "key", Body: strings.NewReader("content"), ServerSideEncryption: encryption, SSEKMSKeyId: kms, Tags: map[string]string{"env": "test"}})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBackendProtocolVersionRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("versionId") != "v1" {
			t.Error("missing versionId")
		}
		if r.Method == http.MethodDelete {
			w.WriteHeader(204)
			return
		}
		w.Header().Set("Content-Length", "2")
		if r.Method == http.MethodGet {
			io.WriteString(w, "v1")
		}
	}))
	defer server.Close()
	s := New("us-east-1", "access", "secret").SetEndpoint(server.URL)
	body, err := s.FileDownload(DownloadInput{Bucket: "bucket", ObjectKey: "key", VersionId: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(body)
	body.Close()
	if string(got) != "v1" {
		t.Errorf("bad version content: %q", got)
	}
	details, err := s.FileDetails(DetailsInput{Bucket: "bucket", ObjectKey: "key", VersionId: "v1"})
	if err != nil || details.ContentLength != "2" {
		t.Fatalf("bad version details: %+v, %v", details, err)
	}
	if err := s.FileDelete(DeleteInput{Bucket: "bucket", ObjectKey: "key", VersionId: "v1"}); err != nil {
		t.Fatal(err)
	}
}
