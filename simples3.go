// LICENSE BSD-2-Clause-FreeBSD
// Copyright (c) 2018, Rohan Verma <hello@rohanverma.net>

package simples3

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"iter"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	imdsTokenHeader        = "X-aws-ec2-metadata-token"
	imdsTokenTtlHeader     = "X-aws-ec2-metadata-token-ttl-seconds"
	metadataBaseURL        = "http://169.254.169.254/latest"
	securityCredentialsURI = "/meta-data/iam/security-credentials/"
	imdsTokenURI           = "/api/token"
	defaultIMDSTokenTTL    = "60"

	// AMZMetaPrefix to prefix metadata key.
	AMZMetaPrefix = "x-amz-meta-"
)

// S3 provides a wrapper around your S3 credentials.
type S3 struct {
	AccessKey string
	SecretKey string
	Region    string
	Client    *http.Client

	Token     string
	Endpoint  string
	URIFormat string
	initMode  string
	expiry    time.Time

	mu sync.Mutex
}

// DownloadInput is passed to FileDownload as a parameter.
type DownloadInput struct {
	Bucket    string
	ObjectKey string
}

// DetailsInput is passed to FileDetails as a parameter.
type DetailsInput struct {
	Bucket    string
	ObjectKey string
}

// DetailsResponse is returned by FileDetails.
type DetailsResponse struct {
	ContentType   string
	ContentLength string
	AcceptRanges  string
	Date          string
	Etag          string
	LastModified  string
	Server        string
	AmzID2        string
	AmzRequestID  string
	AmzMeta       map[string]string
	ExtraHeaders  map[string]string
}

// UploadInput is passed to FileUpload as a parameter.
type UploadInput struct {
	// essential fields
	Bucket      string
	ObjectKey   string
	FileName    string
	ContentType string

	// optional fields
	ContentDisposition string
	ACL                string
	// Setting key/value pairs adds user-defined metadata
	// keys to the object, prefixed with AMZMetaPrefix.
	CustomMetadata map[string]string

	Body io.ReadSeeker
}

// UploadResponse receives the following XML
// in case of success, since we set a 201 response from S3.
// Sample response:
//
//	<PostResponse>
//	  <Location>https://s3.amazonaws.com/link-to-the-file</Location>
//	  <Bucket>s3-bucket</Bucket>
//	  <Key>development/8614bd40-691b-4668-9241-3b342c6cf429/image.jpg</Key>
//	  <ETag>"32-bit-tag"</ETag>
//	</PostResponse>
type UploadResponse struct {
	Location string `xml:"Location"`
	Bucket   string `xml:"Bucket"`
	Key      string `xml:"Key"`
	ETag     string `xml:"ETag"`
}

// PutResponse is returned when the action is successful,
// and the service sends back an HTTP 200 response. The response
// returns ETag along with HTTP headers.
type PutResponse struct {
	ETag    string
	Headers http.Header
}

// DeleteInput is passed to FileDelete as a parameter.
type DeleteInput struct {
	Bucket    string
	ObjectKey string
}

// ListInput is passed to List as a parameter.
type ListInput struct {
	// Required: The name of the bucket to list objects from
	Bucket string

	// Optional: A delimiter to group objects by (commonly "/")
	Delimiter string

	// Optional: Only list objects starting with this prefix
	Prefix string

	// Optional: Maximum number of objects to return
	MaxKeys int64

	// Optional: Token for pagination from a previous request
	ContinuationToken string

	// Optional: Object key to start listing after
	StartAfter string
}

// ListResponse is returned by List.
type ListResponse struct {
	// Name of the bucket
	Name string

	// Whether the results were truncated (more results available)
	IsTruncated bool

	// Token to get the next page of results (if truncated)
	NextContinuationToken string

	// List of objects in the bucket
	Objects []Object

	// Common prefixes when using delimiter (like directories)
	CommonPrefixes []string

	// Total number of keys returned
	KeyCount int64
}

// Object represents an S3 object in a bucket.
type Object struct {
	// Name of the object
	Key string

	// Size in bytes
	Size int64

	// When the object was last modified
	LastModified string

	// Entity tag of the object
	ETag string

	// Storage class (e.g., "STANDARD")
	StorageClass string
}

// S3Error represents an S3 API error response
type S3Error struct {
	XMLName   xml.Name `xml:"Error"`
	Code      string   `xml:"Code"`
	Message   string   `xml:"Message"`
	RequestID string   `xml:"RequestId"`
	HostID    string   `xml:"HostId"`
}

// Error returns a string representation of the S3Error
func (e S3Error) Error() string {
	return fmt.Sprintf("S3 Error: %s - %s", e.Code, e.Message)
}

// IAMResponse is used by NewUsingIAM to auto
// detect the credentials.
type IAMResponse struct {
	Code            string    `json:"Code"`
	LastUpdated     string    `json:"LastUpdated"`
	Type            string    `json:"Type"`
	AccessKeyID     string    `json:"AccessKeyId"`
	SecretAccessKey string    `json:"SecretAccessKey"`
	Token           string    `json:"Token"`
	Expiration      time.Time `json:"Expiration"`
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
	return newUsingIAM(
		&http.Client{
			// Set a timeout of 3 seconds for AWS IAM Calls.
			Timeout: time.Second * 3, //nolint:gomnd
		}, metadataBaseURL, region)
}

// fetchIMDSToken retrieves an IMDSv2 token from the
// EC2 instance metadata service. It returns a token and boolean,
// only if IMDSv2 is enabled in the EC2 instance metadata
// configuration, otherwise returns an error.
func fetchIMDSToken(cl *http.Client, baseURL string) (string, bool, error) {
	req, err := http.NewRequest(http.MethodPut, baseURL+imdsTokenURI, nil)
	if err != nil {
		return "", false, err
	}

	// Set the token TTL to 60 seconds.
	req.Header.Set(imdsTokenTtlHeader, defaultIMDSTokenTTL)

	resp, err := cl.Do(req)
	if err != nil {
		return "", false, err
	}

	defer func() {
		resp.Body.Close()
		io.Copy(io.Discard, resp.Body)
	}()

	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("failed to request IMDSv2 token: %s", resp.Status)
	}

	token, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false, err
	}

	return string(token), true, nil
}

// fetchIAMData fetches the IAM data from the given URL.
// In case of a normal AWS setup, baseURL would be metadataBaseURL.
// You can use this method, to manually fetch IAM data from a custom
// endpoint and pass it to SetIAMData.
func fetchIAMData(cl *http.Client, baseURL string) (IAMResponse, error) {
	token, useIMDSv2, err := fetchIMDSToken(cl, baseURL)
	if err != nil {
		return IAMResponse{}, fmt.Errorf("error fetching IMDSv2 token: %w", err)
	}

	url := baseURL + securityCredentialsURI

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return IAMResponse{}, fmt.Errorf("error creating imdsv2 token request: %w", err)
	}

	if useIMDSv2 {
		req.Header.Set(imdsTokenHeader, token)
	}

	resp, err := cl.Do(req)
	if err != nil {
		return IAMResponse{}, err
	}

	if resp.StatusCode != http.StatusOK {
		return IAMResponse{}, fmt.Errorf("error fetching IAM data: %s", resp.Status)
	}

	role, err := io.ReadAll(resp.Body)
	if err != nil {
		return IAMResponse{}, err
	}

	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	req, err = http.NewRequest(http.MethodGet, url+string(role), nil)
	if err != nil {
		return IAMResponse{}, fmt.Errorf("error creating role request: %w", err)
	}
	if useIMDSv2 {
		req.Header.Set(imdsTokenHeader, token)
	}

	resp, err = cl.Do(req)
	if err != nil {
		return IAMResponse{}, fmt.Errorf("error fetching role data: %w", err)
	}

	defer func() {
		// Drain and close the body to let the Transport reuse the connection
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return IAMResponse{}, fmt.Errorf("error fetching role data, got non 200 code: %s", resp.Status)
	}

	var jResp IAMResponse
	jsonString, err := io.ReadAll(resp.Body)
	if err != nil {
		return IAMResponse{}, fmt.Errorf("error reading role data: %w", err)
	}

	if err := json.Unmarshal(jsonString, &jResp); err != nil {
		return IAMResponse{}, fmt.Errorf("error unmarshalling role data: %w (%s)", err, jsonString)
	}

	return jResp, nil
}

func newUsingIAM(cl *http.Client, baseURL, region string) (*S3, error) {
	// Get the IAM role
	iamResp, err := fetchIAMData(cl, baseURL)
	if err != nil {
		return nil, fmt.Errorf("error fetching IAM data: %w", err)
	}

	return &S3{
		Region:    region,
		AccessKey: iamResp.AccessKeyID,
		SecretKey: iamResp.SecretAccessKey,
		Token:     iamResp.Token,

		URIFormat: "https://s3.%s.amazonaws.com/%s",
		initMode:  "iam",
		expiry:    iamResp.Expiration,
	}, nil
}

// setIAMData sets the IAM data on the S3 instance.
func (s3 *S3) SetIAMData(iamResp IAMResponse) {
	s3.AccessKey = iamResp.AccessKeyID
	s3.SecretKey = iamResp.SecretAccessKey
	s3.Token = iamResp.Token
}

func (s3 *S3) getClient() *http.Client {
	if s3.Client == nil {
		return http.DefaultClient
	}
	return s3.Client
}

// getURL constructs a URL for a given path, with multiple optional
// arguments as individual subfolders, based on the endpoint
// specified in s3 struct.
func (s3 *S3) getURL(path string, args ...string) (uri string) {
	if len(args) > 0 {
		path += "/" + strings.Join(args, "/")
	}
	// need to encode special characters in the path part of the URL
	encodedPath := encodePath(path)

	if len(s3.Endpoint) > 0 {
		uri = s3.Endpoint + "/" + encodedPath
	} else {
		uri = fmt.Sprintf(s3.URIFormat, s3.Region, encodedPath)
	}

	return uri
}

// SetEndpoint can be used to the set a custom endpoint for
// using an alternate instance compatible with the s3 API.
// If no protocol is included in the URI, defaults to HTTPS.
func (s3 *S3) SetEndpoint(uri string) *S3 {
	if len(uri) > 0 {
		if !strings.HasPrefix(uri, "http") {
			uri = "https://" + uri
		}

		// make sure there is no trailing slash
		if uri[len(uri)-1] == '/' {
			uri = uri[:len(uri)-1]
		}
		s3.Endpoint = uri
	}
	return s3
}

// SetToken can be used to set a Temporary Security Credential token obtained from
// using an IAM role or AWS STS.
func (s3 *S3) SetToken(token string) *S3 {
	if token != "" {
		s3.Token = token
	}
	return s3
}

func detectFileSize(body io.Seeker) (int64, error) {
	pos, err := body.Seek(0, 1)
	if err != nil {
		return -1, err
	}
	defer body.Seek(pos, 0)

	n, err := body.Seek(0, 2) //nolint:gomnd
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

func (s3 *S3) signRequest(req *http.Request) error {
	var (
		err error

		date = req.Header.Get("Date")
		t    = time.Now().UTC()
	)

	if date != "" {
		t, err = time.Parse(http.TimeFormat, date)
		if err != nil {
			return err
		}
	}
	req.Header.Set("Date", t.Format(amzDateISO8601TimeFormat))
	req.Header.Set("X-Amz-Date", t.Format(amzDateISO8601TimeFormat))

	if s3.Token != "" {
		req.Header.Set("X-Amz-Security-Token", s3.Token)
	}

	// The x-amz-content-sha256 header is required for all AWS
	// Signature Version 4 requests. It provides a hash of the
	// request payload. If there is no payload, you must provide
	// the hash of an empty string.

	if req.Header.Get("x-amz-content-sha256") == "" {
		emptyhash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
		req.Header.Set("x-amz-content-sha256", emptyhash)
	}

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
	return nil
}

func (s3 *S3) renewIAMToken() error {
	if s3.initMode != "iam" {
		return nil
	}

	if time.Since(s3.expiry) < 0 {
		return nil
	}

	s3.mu.Lock()
	defer s3.mu.Unlock()
	iamResp, err := fetchIAMData(s3.getClient(), metadataBaseURL)
	if err != nil {
		return fmt.Errorf("error fetching IAM data: %w", err)
	}

	s3.expiry = iamResp.Expiration
	s3.Token = iamResp.Token
	s3.AccessKey = iamResp.AccessKeyID
	s3.SecretKey = iamResp.SecretAccessKey

	return nil
}

// FileDownload makes a GET call and returns a io.ReadCloser.
// After reading the response body, ensure closing the response.
func (s3 *S3) FileDownload(u DownloadInput) (io.ReadCloser, error) {
	req, err := http.NewRequest(
		http.MethodGet, s3.getURL(u.Bucket, u.ObjectKey), nil,
	)
	if err != nil {
		return nil, err
	}

	if err := s3.renewIAMToken(); err != nil {
		return nil, err
	}
	if err := s3.signRequest(req); err != nil {
		return nil, err
	}

	res, err := s3.getClient().Do(req)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code: %s", res.Status)
	}

	return res.Body, nil
}

// FilePut makes a PUT call to S3.
func (s3 *S3) FilePut(u UploadInput) (PutResponse, error) {
	fSize, err := detectFileSize(u.Body)
	if err != nil {
		return PutResponse{}, err
	}

	content := make([]byte, fSize)
	_, err = u.Body.Read(content)
	if err != nil {
		return PutResponse{}, err
	}
	u.Body.Seek(0, 0)

	req, er := http.NewRequest(http.MethodPut, s3.getURL(u.Bucket, u.ObjectKey), u.Body)
	if er != nil {
		return PutResponse{}, err
	}

	if u.ContentType == "" {
		u.ContentType = "application/octet-stream"
	}

	h := sha256.New()
	h.Write(content)
	req.Header.Set("x-amz-content-sha256", fmt.Sprintf("%x", h.Sum(nil)))

	req.Header.Set("Content-Type", u.ContentType)
	req.Header.Set("Content-Length", fmt.Sprintf("%d", fSize))
	req.Header.Set("Host", req.URL.Host)

	for k, v := range u.CustomMetadata {
		req.Header.Set("x-amz-meta-"+k, v)
	}

	if u.ContentDisposition != "" {
		req.Header.Set("Content-Disposition", u.ContentDisposition)
	}

	if u.ACL != "" {
		req.Header.Set("x-amz-acl", u.ACL)
	}

	req.ContentLength = fSize

	if err := s3.renewIAMToken(); err != nil {
		return PutResponse{}, err
	}
	if err := s3.signRequest(req); err != nil {
		return PutResponse{}, err
	}

	// debug(httputil.DumpRequest(req, true))
	// Submit the request
	client := s3.getClient()
	res, err := client.Do(req)
	if err != nil {
		return PutResponse{}, err
	}

	defer func() {
		res.Body.Close()
		io.Copy(io.Discard, res.Body)
	}()

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return PutResponse{}, err
	}

	// Check the response
	if res.StatusCode != http.StatusOK {
		return PutResponse{}, fmt.Errorf("status code: %s: %q", res.Status, data)
	}

	return PutResponse{
		ETag:    res.Header.Get("ETag"),
		Headers: res.Header.Clone(),
	}, nil
}

// FileUpload makes a POST call with the file written as multipart
// and on successful upload, checks for 200 OK.
func (s3 *S3) FileUpload(u UploadInput) (UploadResponse, error) {
	fSize, err := detectFileSize(u.Body)
	if err != nil {
		return UploadResponse{}, err
	}

	uc := UploadConfig{
		UploadURL:          s3.getURL(u.Bucket),
		BucketName:         u.Bucket,
		ObjectKey:          u.ObjectKey,
		ContentType:        u.ContentType,
		ContentDisposition: u.ContentDisposition,
		ACL:                u.ACL,
		FileSize:           fSize,
		MetaData: map[string]string{
			"success_action_status": "201", // returns XML doc on success
		},
	}

	// Set custom metadata.
	for k, v := range u.CustomMetadata {
		if !strings.HasPrefix(k, AMZMetaPrefix) {
			k = AMZMetaPrefix + k
		}

		uc.MetaData[k] = v
	}

	if err := s3.renewIAMToken(); err != nil {
		return UploadResponse{}, err
	}

	policies, err := s3.CreateUploadPolicies(uc)
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
	req, err := http.NewRequest(http.MethodPost, policies.URL, &b)
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

	defer func() {
		res.Body.Close()
		io.Copy(io.Discard, res.Body)
	}()

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return UploadResponse{}, err
	}

	// Check the response
	if res.StatusCode != http.StatusCreated {
		return UploadResponse{}, fmt.Errorf("status code: %s: %q", res.Status, data)
	}

	var ur UploadResponse
	_ = xml.Unmarshal(data, &ur)
	return ur, nil
}

// FileDelete makes a DELETE call with the file written as multipart
// and on successful upload, checks for 204 No Content.
func (s3 *S3) FileDelete(u DeleteInput) error {
	req, err := http.NewRequest(
		http.MethodDelete, s3.getURL(u.Bucket, u.ObjectKey), nil,
	)
	if err != nil {
		return err
	}

	if err := s3.renewIAMToken(); err != nil {
		return err
	}

	if err := s3.signRequest(req); err != nil {
		return err
	}

	// Submit the request
	client := s3.getClient()
	res, err := client.Do(req)
	if err != nil {
		return err
	}

	defer func() {
		res.Body.Close()
		io.Copy(io.Discard, res.Body)
	}()

	// Check the response
	if res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("status code: %s", res.Status)
	}

	return nil
}

func (s3 *S3) FileDetails(u DetailsInput) (DetailsResponse, error) {
	req, err := http.NewRequest(
		http.MethodHead, s3.getURL(u.Bucket, u.ObjectKey), nil,
	)
	if err != nil {
		return DetailsResponse{}, err
	}

	if err := s3.renewIAMToken(); err != nil {
		return DetailsResponse{}, err
	}

	if err := s3.signRequest(req); err != nil {
		return DetailsResponse{}, err
	}

	res, err := s3.getClient().Do(req)
	if err != nil {
		return DetailsResponse{}, err
	}

	defer func() {
		res.Body.Close()
		io.Copy(io.Discard, res.Body)
	}()

	if res.StatusCode != http.StatusOK {
		return DetailsResponse{}, fmt.Errorf("status code: %s", res.Status)
	}

	var out DetailsResponse
	for k, v := range res.Header {
		lk := strings.ToLower(k)

		switch lk {
		case "content-type":
			out.ContentType = getFirstString(v)
		case "content-length":
			out.ContentLength = getFirstString(v)
		case "accept-ranges":
			out.AcceptRanges = getFirstString(v)
		case "date":
			out.Date = getFirstString(v)
		case "etag":
			out.Etag = getFirstString(v)
		case "last-modified":
			out.LastModified = getFirstString(v)
		case "server":
			out.Server = getFirstString(v)
		case "x-amz-id-2":
			out.AmzID2 = getFirstString(v)
		case "x-amz-request-id":
			out.AmzRequestID = getFirstString(v)
		default:
			if strings.HasPrefix(lk, AMZMetaPrefix) {
				if out.AmzMeta == nil {
					out.AmzMeta = map[string]string{}
				}

				out.AmzMeta[k] = getFirstString(v)
			} else {
				if out.ExtraHeaders == nil {
					out.ExtraHeaders = map[string]string{}
				}

				out.ExtraHeaders[k] = getFirstString(v)
			}
		}
	}

	return out, nil
}

// List implements a simple S3 object listing API
func (s3 *S3) List(input ListInput) (ListResponse, error) {
	// Input validation
	if input.Bucket == "" {
		return ListResponse{}, fmt.Errorf("bucket name cannot be empty")
	}
	if input.MaxKeys < 0 {
		return ListResponse{}, fmt.Errorf("MaxKeys cannot be negative")
	}
	if input.MaxKeys > 1000 {
		return ListResponse{}, fmt.Errorf("MaxKeys cannot exceed 1000")
	}

	// Build query parameters - ListObjectsV2 uses query params, not path
	query := url.Values{}
	query.Set("list-type", "2") // Required parameter

	// Add optional parameters if they exist
	if input.ContinuationToken != "" {
		query.Set("continuation-token", input.ContinuationToken)
	}
	if input.Delimiter != "" {
		query.Set("delimiter", input.Delimiter)
	}
	if input.MaxKeys > 0 {
		query.Set("max-keys", fmt.Sprintf("%d", input.MaxKeys))
	}
	if input.Prefix != "" {
		query.Set("prefix", input.Prefix)
	}
	if input.StartAfter != "" {
		query.Set("start-after", input.StartAfter)
	}

	// Build base URL
	baseURL := s3.getURL(input.Bucket)

	// Parse URL and add query parameters
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return ListResponse{}, err
	}
	parsedURL.RawQuery = query.Encode()

	// Create HTTP request
	req, err := http.NewRequest(http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return ListResponse{}, err
	}

	// Apply AWS V4 signing
	if err := s3.renewIAMToken(); err != nil {
		return ListResponse{}, err
	}
	if err := s3.signRequest(req); err != nil {
		return ListResponse{}, err
	}

	// Execute request
	client := s3.getClient()
	res, err := client.Do(req)
	if err != nil {
		return ListResponse{}, err
	}

	// Close response body when done
	defer res.Body.Close()

	// Read response body once
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return ListResponse{}, err
	}

	// Handle response status codes
	if res.StatusCode != http.StatusOK {
		var s3Err S3Error
		if xmlErr := xml.Unmarshal(body, &s3Err); xmlErr == nil {
			return ListResponse{}, fmt.Errorf("S3 Error: %s - %s", s3Err.Code, s3Err.Message)
		}
		return ListResponse{}, fmt.Errorf("status code: %s: %s", res.Status, string(body))
	}

	// Parse XML response using internal struct
	var xmlResult struct {
		XMLName               xml.Name `xml:"ListBucketResult"`
		Name                  string   `xml:"Name"`
		IsTruncated           bool     `xml:"IsTruncated"`
		NextContinuationToken string   `xml:"NextContinuationToken"`
		KeyCount              int64    `xml:"KeyCount"`
		Contents              []struct {
			Key          string `xml:"Key"`
			LastModified string `xml:"LastModified"`
			ETag         string `xml:"ETag"`
			Size         int64  `xml:"Size"`
			StorageClass string `xml:"StorageClass"`
		} `xml:"Contents"`
		CommonPrefixes []struct {
			Prefix string `xml:"Prefix"`
		} `xml:"CommonPrefixes"`
	}

	if err := xml.Unmarshal(body, &xmlResult); err != nil {
		return ListResponse{}, err
	}

	// Convert to simple response format
	response := ListResponse{
		Name:                  xmlResult.Name,
		IsTruncated:           xmlResult.IsTruncated,
		NextContinuationToken: xmlResult.NextContinuationToken,
		KeyCount:              xmlResult.KeyCount,
	}

	// Convert objects
	for _, obj := range xmlResult.Contents {
		response.Objects = append(response.Objects, Object{
			Key:          obj.Key,
			Size:         obj.Size,
			LastModified: obj.LastModified,
			ETag:         obj.ETag,
			StorageClass: obj.StorageClass,
		})
	}

	// Convert common prefixes
	for _, prefix := range xmlResult.CommonPrefixes {
		response.CommonPrefixes = append(response.CommonPrefixes, prefix.Prefix)
	}

	return response, nil
}

// ListAll returns an iterator that yields all objects in the bucket,
// automatically handling pagination. It also returns a finish callback
// that should be called after iteration to check for any errors.
func (s3 *S3) ListAll(input ListInput) (iter.Seq[Object], func() error) {
	var iterErr error

	seq := func(yield func(Object) bool) {
		currentInput := input

		for {
			response, err := s3.List(currentInput)
			if err != nil {
				iterErr = err
				return
			}

			// Yield each object
			for _, obj := range response.Objects {
				if !yield(obj) {
					return // Early termination requested
				}
			}

			// Check if there are more results
			if !response.IsTruncated || response.NextContinuationToken == "" {
				return // No more results
			}

			// Prepare for next page
			currentInput.ContinuationToken = response.NextContinuationToken
		}
	}

	finish := func() error {
		return iterErr
	}

	return seq, finish
}

func getFirstString(s []string) string {
	if len(s) > 0 {
		return s[0]
	}

	return ""
}

// if object matches reserved string, no need to encode them.
var reservedObjectNames = regexp.MustCompile("^[a-zA-Z0-9-_.~/]+$")

// encodePath encode the strings from UTF-8 byte representations to HTML hex escape sequences
//
// This is necessary since regular url.Parse() and url.Encode() functions do not support UTF-8
// non english characters cannot be parsed due to the nature in which url.Encode() is written
//
// This function on the other hand is a direct replacement for url.Encode() technique to support
// pretty much every UTF-8 character.
// adapted from
// https://github.com/minio/minio-go/blob/fe1f3855b146c1b6ce4199740d317e44cf9e85c2/pkg/s3utils/utils.go#L285
func encodePath(pathName string) string {
	if reservedObjectNames.MatchString(pathName) {
		return pathName
	}
	var encodedPathname strings.Builder
	for _, s := range pathName {
		if 'A' <= s && s <= 'Z' || 'a' <= s && s <= 'z' || '0' <= s && s <= '9' { // §2.3 Unreserved characters (mark)
			encodedPathname.WriteRune(s)
			continue
		}
		switch s {
		case '-', '_', '.', '~', '/': // §2.3 Unreserved characters (mark)
			encodedPathname.WriteRune(s)
			continue
		default:
			lenR := utf8.RuneLen(s)
			if lenR < 0 {
				// if utf8 cannot convert, return the same string as is
				return pathName
			}
			u := make([]byte, lenR)
			utf8.EncodeRune(u, s)
			for _, r := range u {
				hex := hex.EncodeToString([]byte{r})
				encodedPathname.WriteString("%" + strings.ToUpper(hex))
			}
		}
	}
	return encodedPathname.String()
}
