// LICENSE BSD-2-Clause-FreeBSD
// Copyright (c) 2018, Rohan Verma <hello@rohanverma.net>

package simples3

import (
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type DownloadInput struct {
	Bucket    string
	ObjectKey string
	VersionId string // Optional: Version ID of the object to download
}

// DetailsInput is passed to FileDetails as a parameter.
type DetailsInput struct {
	Bucket    string
	ObjectKey string
	VersionId string // Optional: Version ID of the object to retrieve details for
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
	// Setting key/value pairs adds tags to the object (max 10 tags)
	Tags map[string]string

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
	VersionId string // Optional: Version ID of the object to delete
}

// FileDownload makes a GET call and returns a io.ReadCloser.
// After reading the response body, ensure closing the response.
func (s3 *S3) FileDownload(u DownloadInput) (io.ReadCloser, error) {
	urlStr := s3.getURL(u.Bucket, u.ObjectKey)

	if u.VersionId != "" {
		parsed, err := url.Parse(urlStr)
		if err != nil {
			return nil, err
		}
		q := parsed.Query()
		q.Set("versionId", u.VersionId)
		parsed.RawQuery = q.Encode()
		urlStr = parsed.String()
	}

	req, err := http.NewRequest(
		http.MethodGet, urlStr, nil,
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

	if len(u.Tags) > 0 {
		req.Header.Set("x-amz-tagging", encodeTagsHeader(u.Tags))
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

	// Set tags if provided.
	if len(u.Tags) > 0 {
		uc.MetaData["x-amz-tagging"] = encodeTagsHeader(u.Tags)
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
	urlStr := s3.getURL(u.Bucket, u.ObjectKey)

	if u.VersionId != "" {
		parsed, err := url.Parse(urlStr)
		if err != nil {
			return err
		}
		q := parsed.Query()
		q.Set("versionId", u.VersionId)
		parsed.RawQuery = q.Encode()
		urlStr = parsed.String()
	}

	req, err := http.NewRequest(
		http.MethodDelete, urlStr, nil,
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
	urlStr := s3.getURL(u.Bucket, u.ObjectKey)

	if u.VersionId != "" {
		parsed, err := url.Parse(urlStr)
		if err != nil {
			return DetailsResponse{}, err
		}
		q := parsed.Query()
		q.Set("versionId", u.VersionId)
		parsed.RawQuery = q.Encode()
		urlStr = parsed.String()
	}

	req, err := http.NewRequest(
		http.MethodHead, urlStr, nil,
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

// CopyObjectInput is passed to CopyObject as a parameter.
type CopyObjectInput struct {
	// Required: Source bucket name
	SourceBucket string

	// Required: Source object key
	SourceKey string

	// Required: Destination bucket name
	DestBucket string

	// Required: Destination object key
	DestKey string

	// Optional: COPY (default) or REPLACE
	// COPY - copies metadata from source
	// REPLACE - replaces metadata with values specified in this input
	MetadataDirective string

	// Optional: Content type (only used when MetadataDirective = REPLACE)
	ContentType string

	// Optional: Custom metadata (only used when MetadataDirective = REPLACE)
	CustomMetadata map[string]string

	// Optional: Tags to set on the destination object (max 10 tags)
	// When set, uses x-amz-tagging-directive: REPLACE
	Tags map[string]string
}

// CopyObjectOutput is returned by CopyObject.
type CopyObjectOutput struct {
	ETag         string
	LastModified time.Time
}

// copyObjectResult is the internal type for XML parsing.
type copyObjectResult struct {
	ETag         string    `xml:"ETag"`
	LastModified time.Time `xml:"LastModified"`
}

// CopyObject copies an object from source to destination.
// Can copy within the same bucket or across buckets.
// This operation is server-side, avoiding download/upload cycle.
func (s3 *S3) CopyObject(input CopyObjectInput) (CopyObjectOutput, error) {
	// Validate required fields
	if input.SourceBucket == "" || input.SourceKey == "" {
		return CopyObjectOutput{}, fmt.Errorf("source bucket and key are required")
	}
	if input.DestBucket == "" || input.DestKey == "" {
		return CopyObjectOutput{}, fmt.Errorf("destination bucket and key are required")
	}

	// Renew IAM token if needed
	if err := s3.renewIAMToken(); err != nil {
		return CopyObjectOutput{}, err
	}

	// Build destination URL
	url := s3.getURL(input.DestBucket, input.DestKey)

	// Create PUT request
	req, err := http.NewRequest(http.MethodPut, url, nil)
	if err != nil {
		return CopyObjectOutput{}, err
	}

	// Set x-amz-copy-source header (URL-encoded)
	copySource := "/" + input.SourceBucket + "/" + encodePath(input.SourceKey)
	req.Header.Set("x-amz-copy-source", copySource)

	// Optional metadata directive
	if input.MetadataDirective != "" {
		req.Header.Set("x-amz-metadata-directive", input.MetadataDirective)
	}

	// If REPLACE, set content-type and custom metadata
	if input.MetadataDirective == "REPLACE" {
		if input.ContentType != "" {
			req.Header.Set("Content-Type", input.ContentType)
		}
		for k, v := range input.CustomMetadata {
			req.Header.Set("x-amz-meta-"+k, v)
		}
	}

	// Set tags if provided (with REPLACE directive to override source tags)
	// Note: x-amz-tagging-directive causes signature errors in MinIO despite being supported in code
	// See: https://github.com/minio/minio/pull/9711, https://github.com/minio/minio/pull/9478
	// Without directive: tags are copied from source (default S3 behavior)
	// With directive: signature mismatch error in MinIO (works on AWS S3)
	//
	// WORKAROUND: To support both MinIO and AWS without signature errors, we do not set
	// the tagging headers here. Instead, we apply the tags using PutObjectTagging
	// after the copy operation is complete.
	// if len(input.Tags) > 0 {
	// 	req.Header.Set("x-amz-tagging-directive", "REPLACE")
	// 	req.Header.Set("x-amz-tagging", encodeTagsHeader(input.Tags))
	// }

	// Sign the request
	if err := s3.signRequest(req); err != nil {
		return CopyObjectOutput{}, err
	}

	// Execute request
	client := s3.getClient()
	res, err := client.Do(req)
	if err != nil {
		return CopyObjectOutput{}, err
	}

	defer func() {
		res.Body.Close()
		io.Copy(io.Discard, res.Body)
	}()

	// Read response body
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return CopyObjectOutput{}, err
	}

	// Handle non-OK status codes
	if res.StatusCode != http.StatusOK {
		return CopyObjectOutput{}, fmt.Errorf("status code: %s: %s", res.Status, string(body))
	}

	// Parse XML response
	var result copyObjectResult
	if err := xml.Unmarshal(body, &result); err != nil {
		return CopyObjectOutput{}, err
	}

	// Apply tags if provided (2-step workaround)
	if len(input.Tags) > 0 {
		err := s3.PutObjectTagging(PutObjectTaggingInput{
			Bucket:    input.DestBucket,
			ObjectKey: input.DestKey,
			Tags:      input.Tags,
		})
		if err != nil {
			return CopyObjectOutput{
				ETag:         result.ETag,
				LastModified: result.LastModified,
			}, fmt.Errorf("copy succeeded but tagging failed: %v", err)
		}
	}

	return CopyObjectOutput{
		ETag:         result.ETag,
		LastModified: result.LastModified,
	}, nil
}

// DeleteObjectsInput is passed to DeleteObjects as a parameter.
type DeleteObjectsInput struct {
	// Required: The name of the bucket
	Bucket string

	// Required: List of object keys to delete (max 1000)
	Objects []string

	// Optional: Quiet mode - only return errors, not successes
	Quiet bool
}

// DeleteObjectsOutput is returned by DeleteObjects.
type DeleteObjectsOutput struct {
	Deleted []DeletedObject
	Errors  []DeleteError
}

// DeletedObject represents a successfully deleted object.
type DeletedObject struct {
	Key string `xml:"Key"`
}

// DeleteError represents a deletion error.
type DeleteError struct {
	Key     string `xml:"Key"`
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
}

// deleteRequest is the internal type for XML marshaling of the request.
type deleteRequest struct {
	XMLName xml.Name       `xml:"Delete"`
	XMLNS   string         `xml:"xmlns,attr"`
	Quiet   bool           `xml:"Quiet"`
	Objects []deleteObject `xml:"Object"`
}

// deleteObject represents an object to delete in the XML request.
type deleteObject struct {
	Key string `xml:"Key"`
}

// deleteResult is the internal type for XML parsing of the response.
type deleteResult struct {
	Deleted []DeletedObject `xml:"Deleted"`
	Errors  []DeleteError   `xml:"Error"`
}

// DeleteObjects deletes multiple objects in a single request (max 1000).
// This is more efficient than calling FileDelete multiple times.
// Returns both successful deletions and errors.
func (s3 *S3) DeleteObjects(input DeleteObjectsInput) (DeleteObjectsOutput, error) {
	// Validate required fields
	if input.Bucket == "" {
		return DeleteObjectsOutput{}, fmt.Errorf("bucket name is required")
	}
	if len(input.Objects) == 0 {
		return DeleteObjectsOutput{}, fmt.Errorf("at least one object key is required")
	}
	if len(input.Objects) > 1000 {
		return DeleteObjectsOutput{}, fmt.Errorf("cannot delete more than 1000 objects per request")
	}

	// Renew IAM token if needed
	if err := s3.renewIAMToken(); err != nil {
		return DeleteObjectsOutput{}, err
	}

	// Build XML request body
	deleteReq := deleteRequest{
		XMLNS:   "http://s3.amazonaws.com/doc/2006-03-01/",
		Quiet:   input.Quiet,
		Objects: make([]deleteObject, len(input.Objects)),
	}
	for i, key := range input.Objects {
		deleteReq.Objects[i] = deleteObject{Key: key}
	}

	xmlBody, err := xml.Marshal(deleteReq)
	if err != nil {
		return DeleteObjectsOutput{}, err
	}

	// Calculate Content-MD5 (required by S3 for this operation)
	md5Hash := md5.Sum(xmlBody)
	contentMD5 := base64.StdEncoding.EncodeToString(md5Hash[:])

	// Build URL with ?delete query parameter
	baseURL := s3.getURL(input.Bucket)
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return DeleteObjectsOutput{}, err
	}

	// Add delete query parameter (no value, just "?delete")
	parsedURL.RawQuery = "delete"

	// Create POST request
	req, err := http.NewRequest(http.MethodPost, parsedURL.String(), bytes.NewReader(xmlBody))
	if err != nil {
		return DeleteObjectsOutput{}, err
	}

	// Set ContentLength BEFORE other headers
	req.ContentLength = int64(len(xmlBody))

	// Calculate content SHA256 for x-amz-content-sha256 header
	h := sha256.New()
	h.Write(xmlBody)
	req.Header.Set("x-amz-content-sha256", fmt.Sprintf("%x", h.Sum(nil)))

	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("Content-MD5", contentMD5)
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(xmlBody)))
	req.Header.Set("Host", req.URL.Host)

	// Sign the request
	if err := s3.signRequest(req); err != nil {
		return DeleteObjectsOutput{}, err
	}

	// Execute request
	client := s3.getClient()
	res, err := client.Do(req)
	if err != nil {
		return DeleteObjectsOutput{}, err
	}

	defer func() {
		res.Body.Close()
		io.Copy(io.Discard, res.Body)
	}()

	// Read response body
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return DeleteObjectsOutput{}, err
	}

	// Handle non-OK status codes
	if res.StatusCode != http.StatusOK {
		return DeleteObjectsOutput{}, fmt.Errorf("status code: %s: %s", res.Status, string(body))
	}

	// Parse XML response
	var result deleteResult
	if err := xml.Unmarshal(body, &result); err != nil {
		return DeleteObjectsOutput{}, err
	}

	return DeleteObjectsOutput{
		Deleted: result.Deleted,
		Errors:  result.Errors,
	}, nil
}
