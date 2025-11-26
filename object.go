// LICENSE BSD-2-Clause-FreeBSD
// Copyright (c) 2018, Rohan Verma <hello@rohanverma.net>

package simples3

import (
	"bytes"
	"crypto/sha256"
	"encoding/xml"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
)

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
