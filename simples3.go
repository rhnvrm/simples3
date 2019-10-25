// LICENSE MIT
// Copyright (c) 2018, Rohan Verma <hello@rohanverma.net>

package simples3

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

const (
	securityCredentialsURL = "http://169.254.169.254/latest/meta-data/iam/security-credentials/"
)

// S3 provides a wrapper around your S3 credentials.
type S3 struct {
	AccessKey string
	SecretKey string
	Region    string
	Client    *http.Client

	Endpoint  string
	URIFormat string
}

// UploadInput is passed to FileUpload as a parameter.
type UploadInput struct {
	Bucket      string
	ObjectKey   string
	FileName    string
	ContentType string
	Body        io.ReadSeeker
}

// UploadResponse receives the following XML
// in case of success, since we set a 201 response from S3.
// Sample response:
// <PostResponse>
//     <Location>https://s3.amazonaws.com/link-to-the-file</Location>
//     <Bucket>s3-bucket</Bucket>
//     <Key>development/8614bd40-691b-4668-9241-3b342c6cf429/image.jpg</Key>
//     <ETag>"32-bit-tag"</ETag>
// </PostResponse>
type UploadResponse struct {
	Location string `xml:"Location"`
	Bucket   string `xml:"Bucket"`
	Key      string `xml:"Key"`
	ETag     string `xml:"ETag"`
}

// DeleteInput is passed to FileDelete as a parameter.
type DeleteInput struct {
	Bucket    string
	ObjectKey string
}

// IAMResponse is used by NewUsingIAM to auto
// detect the credentials
type IAMResponse struct {
	Code            string `json:"Code"`
	LastUpdated     string `json:"LastUpdated"`
	Type            string `json:"Type"`
	AccessKeyID     string `json:"AccessKeyId"`
	SecretAccessKey string `json:"SecretAccessKey"`
	Token           string `json:"Token"`
	Expiration      string `json:"Expiration"`
}

// New returns an instance of S3.
func New(region, accessKey, secretKey string) *S3 {
	return &S3{
		Region:    region,
		AccessKey: accessKey,
		SecretKey: secretKey,

		URIFormat: "https://s3.%s.amazonaws.com/%s",
	}
}

// NewUsingIAM automatically generates an Instance of S3
// using instance metatdata.
func NewUsingIAM(region string) (*S3, error) {
	return newUsingIAMImpl(securityCredentialsURL, region)
}

func newUsingIAMImpl(baseURL, region string) (*S3, error) {
	// Get the IAM role
	resp, err := http.Get(baseURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, errors.New(http.StatusText(resp.StatusCode))
	}

	role, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	resp, err = http.Get(baseURL + "/" + string(role))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, errors.New(http.StatusText(resp.StatusCode))
	}

	var jsonResp IAMResponse
	jsonString, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(jsonString, &jsonResp); err != nil {
		return nil, err
	}

	return &S3{
		Region:    region,
		AccessKey: jsonResp.AccessKeyID,
		SecretKey: jsonResp.SecretAccessKey,

		URIFormat: "https://s3.%s.amazonaws.com/%s",
	}, nil
}

func (s3 *S3) getClient() *http.Client {
	if s3.Client == nil {
		return http.DefaultClient
	}
	return s3.Client
}

func (s3 *S3) getURL(bucket string, args ...string) (uri string) {
	if len(s3.Endpoint) > 0 {
		uri = defaultProtocol + s3.Endpoint + "/" + bucket
	} else {
		uri = fmt.Sprintf(s3.URIFormat, s3.Region, bucket)
	}

	if len(args) > 0 {
		uri = uri + "/" + strings.Join(args, "/")
	}
	return
}

// SetEndpoint ...
func (s3 *S3) SetEndpoint(uri string) *S3 {
	if len(uri) > 0 {
		s3.Endpoint = uri
	}
	return s3
}

func detectFileSize(body io.Seeker) (int64, error) {
	pos, err := body.Seek(0, 1)
	if err != nil {
		return -1, err
	}
	defer body.Seek(pos, 0)

	n, err := body.Seek(0, 2)
	if err != nil {
		return -1, err
	}
	return n, nil
}

// SetClient can be used to set the http client to be
// used by the package. If client passed is nil,
// http.DefaultClient is used.
func (s3 *S3) SetClient(client *http.Client) *S3 {
	if client != nil {
		s3.Client = client
	} else {
		s3.Client = http.DefaultClient
	}
	return s3
}

// FileUpload makes a POST call with the file written as multipart
// and on successful upload, checks for 200 OK.
func (s3 *S3) FileUpload(u UploadInput) (UploadResponse, error) {
	fSize, err := detectFileSize(u.Body)
	if err != nil {
		return UploadResponse{}, err
	}
	policies, err := s3.CreateUploadPolicies(UploadConfig{
		UploadURL:   s3.getURL(u.Bucket),
		BucketName:  u.Bucket,
		ObjectKey:   u.ObjectKey,
		ContentType: u.ContentType,
		FileSize:    fSize,
		MetaData: map[string]string{
			"success_action_status": "201", // returns XML doc on success
		},
	})

	if err != nil {
		return UploadResponse{}, err
	}

	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	for k, v := range policies.Form {
		if err = w.WriteField(k, v); err != nil {
			return UploadResponse{}, err
		}
	}

	fw, err := w.CreateFormFile("file", u.FileName)
	if err != nil {
		return UploadResponse{}, err
	}
	if _, err = io.Copy(fw, u.Body); err != nil {
		return UploadResponse{}, err
	}

	// Don't forget to close the multipart writer.
	// If you don't close it, your request will be missing the terminating boundary.
	if err := w.Close(); err != nil {
		return UploadResponse{}, err
	}

	// Now that you have a form, you can submit it to your handler.
	req, err := http.NewRequest("POST", policies.URL, &b)
	if err != nil {
		return UploadResponse{}, err
	}
	// Don't forget to set the content type, this will contain the boundary.
	req.Header.Set("Content-Type", w.FormDataContentType())

	// Submit the request
	client := s3.getClient()
	res, err := client.Do(req)
	if err != nil {
		return UploadResponse{}, err
	}
	defer res.Body.Close()

	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return UploadResponse{}, err
	}
	// Check the response
	if res.StatusCode != 201 {
		return UploadResponse{}, fmt.Errorf("status code: %s: %q", res.Status, data)
	}

	var ur UploadResponse
	xml.Unmarshal(data, &ur)
	return ur, nil
}

// FileDelete makes a DELETE call with the file written as multipart
// and on successful upload, checks for 204 No Content.
func (s3 *S3) FileDelete(u DeleteInput) error {
	req, err := http.NewRequest(
		"DELETE", s3.getURL(u.Bucket, u.ObjectKey), nil,
	)
	if err != nil {
		return err
	}

	date := req.Header.Get("Date")
	t := time.Now().UTC()
	if date != "" {
		t, err = time.Parse(http.TimeFormat, date)
		if err != nil {
			return err
		}
	}
	req.Header.Set("Date", t.Format(amzDateISO8601TimeFormat))

	// The x-amz-content-sha256 header is required for all AWS
	// Signature Version 4 requests. It provides a hash of the
	// request payload. If there is no payload, you must provide
	// the hash of an empty string.
	emptyhash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	req.Header.Set("x-amz-content-sha256", emptyhash)

	k := s3.signKeys(t)
	h := hmac.New(sha256.New, k)

	s3.writeStringToSign(h, t, req)

	auth := bytes.NewBufferString(algorithm)
	auth.Write([]byte(" Credential=" + s3.AccessKey + "/" + s3.creds(t)))
	auth.Write([]byte{',', ' '})
	auth.Write([]byte("SignedHeaders="))
	writeHeaderList(auth, req)
	auth.Write([]byte{',', ' '})
	auth.Write([]byte("Signature=" + fmt.Sprintf("%x", h.Sum(nil))))

	req.Header.Set("Authorization", auth.String())

	// Submit the request
	client := s3.getClient()
	res, err := client.Do(req)
	if err != nil {
		return err
	}

	// Check the response
	if res.StatusCode != 204 {
		return fmt.Errorf("status code: %s", res.Status)
	}

	return nil
}
