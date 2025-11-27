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
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Bucket represents a single S3 bucket.
type Bucket struct {
	Name         string    `xml:"Name"`
	CreationDate time.Time `xml:"CreationDate"`
}

// ListBucketsInput is passed to ListBuckets as a parameter.
type ListBucketsInput struct {
	// Empty - ListBuckets lists all buckets for the account
}

// ListBucketsOutput is returned by ListBuckets.
type ListBucketsOutput struct {
	Buckets []Bucket `xml:"Buckets>Bucket"`
	Owner   struct {
		ID          string `xml:"ID"`
		DisplayName string `xml:"DisplayName"`
	} `xml:"Owner"`
}

// CreateBucketInput is passed to CreateBucket as a parameter.
type CreateBucketInput struct {
	// Required: The name of the bucket to create
	Bucket string

	// Optional: AWS region for the bucket.
	// If empty, uses the region from S3 struct.
	// Note: For us-east-1, no LocationConstraint is needed.
	Region string
}

// CreateBucketOutput is returned by CreateBucket.
type CreateBucketOutput struct {
	// Location header from the response
	Location string
}

// DeleteBucketInput is passed to DeleteBucket as a parameter.
type DeleteBucketInput struct {
	// Required: The name of the bucket to delete
	// Note: The bucket must be empty before deletion
	Bucket string
}

// ListBuckets lists all S3 buckets for the AWS account.
// It makes a GET request to the S3 service endpoint (not a specific bucket).
func (s3 *S3) ListBuckets(input ListBucketsInput) (ListBucketsOutput, error) {
	// Renew IAM token if needed
	if err := s3.renewIAMToken(); err != nil {
		return ListBucketsOutput{}, err
	}

	// Build endpoint URL - ListBuckets uses the service endpoint (no bucket name)
	var endpoint string
	if len(s3.Endpoint) > 0 {
		endpoint = s3.Endpoint + "/"
	} else {
		// For AWS, use the service endpoint
		endpoint = fmt.Sprintf("https://s3.%s.amazonaws.com/", s3.Region)
	}

	// Create HTTP request
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return ListBucketsOutput{}, err
	}

	// Sign the request
	if err := s3.signRequest(req); err != nil {
		return ListBucketsOutput{}, err
	}

	// Execute request
	client := s3.getClient()
	res, err := client.Do(req)
	if err != nil {
		return ListBucketsOutput{}, err
	}

	defer func() {
		res.Body.Close()
		io.Copy(io.Discard, res.Body)
	}()

	// Read response body
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return ListBucketsOutput{}, err
	}

	// Handle non-OK status codes
	if res.StatusCode != http.StatusOK {
		return ListBucketsOutput{}, fmt.Errorf("status code: %s: %s", res.Status, string(body))
	}

	// Parse XML response
	var result ListBucketsOutput
	if err := xml.Unmarshal(body, &result); err != nil {
		return ListBucketsOutput{}, err
	}

	return result, nil
}

// CreateBucket creates a new S3 bucket.
// For regions other than us-east-1, it sends a LocationConstraint in the request body.
func (s3 *S3) CreateBucket(input CreateBucketInput) (CreateBucketOutput, error) {
	// Validate input
	if input.Bucket == "" {
		return CreateBucketOutput{}, fmt.Errorf("bucket name is required")
	}

	// Renew IAM token if needed
	if err := s3.renewIAMToken(); err != nil {
		return CreateBucketOutput{}, err
	}

	// Determine region to use
	region := input.Region
	if region == "" {
		region = s3.Region
	}

	// Build endpoint URL
	url := s3.getURL(input.Bucket)

	// Prepare request body for non-us-east-1 regions
	// AWS S3 requires LocationConstraint for regions other than us-east-1
	var body io.Reader
	if region != "us-east-1" && region != "" {
		xmlBody := fmt.Sprintf(
			"<CreateBucketConfiguration><LocationConstraint>%s</LocationConstraint></CreateBucketConfiguration>",
			region,
		)
		body = strings.NewReader(xmlBody)
	}

	// Create HTTP request
	req, err := http.NewRequest(http.MethodPut, url, body)
	if err != nil {
		return CreateBucketOutput{}, err
	}

	// Sign the request
	if err := s3.signRequest(req); err != nil {
		return CreateBucketOutput{}, err
	}

	// Execute request
	client := s3.getClient()
	res, err := client.Do(req)
	if err != nil {
		return CreateBucketOutput{}, err
	}

	defer func() {
		res.Body.Close()
		io.Copy(io.Discard, res.Body)
	}()

	// Read response body for error messages
	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return CreateBucketOutput{}, err
	}

	// Handle non-success status codes
	// AWS returns 200 OK or 201 Created on success
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		return CreateBucketOutput{}, fmt.Errorf("status code: %s: %s", res.Status, string(bodyBytes))
	}

	// Extract location from response header
	location := res.Header.Get("Location")

	return CreateBucketOutput{Location: location}, nil
}

// DeleteBucket deletes an empty S3 bucket.
// The bucket must be empty (no objects) before it can be deleted.
// Returns an error if the bucket is not empty or does not exist.
func (s3 *S3) DeleteBucket(input DeleteBucketInput) error {
	// Validate input
	if input.Bucket == "" {
		return fmt.Errorf("bucket name is required")
	}

	// Renew IAM token if needed
	if err := s3.renewIAMToken(); err != nil {
		return err
	}

	// Build endpoint URL
	url := s3.getURL(input.Bucket)

	// Create HTTP request
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return err
	}

	// Sign the request
	if err := s3.signRequest(req); err != nil {
		return err
	}

	// Execute request
	client := s3.getClient()
	res, err := client.Do(req)
	if err != nil {
		return err
	}

	defer func() {
		res.Body.Close()
		io.Copy(io.Discard, res.Body)
	}()

	// Read response body for error messages
	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	// Handle non-success status codes
	// AWS returns 204 No Content on successful deletion
	if res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("status code: %s: %s", res.Status, string(bodyBytes))
	}

	return nil
}

// VersioningConfiguration represents the versioning configuration of a bucket.
type VersioningConfiguration struct {
	Status    string `xml:"Status"`              // Enabled or Suspended
	MfaDelete string `xml:"MfaDelete,omitempty"` // Enabled or Disabled
}

// PutBucketVersioningInput is passed to PutBucketVersioning as a parameter.
type PutBucketVersioningInput struct {
	// Required: The name of the bucket
	Bucket string

	// Required: Versioning status (Enabled or Suspended)
	Status string

	// Optional: MFA Delete status (Enabled or Disabled)
	// Note: Requires MFA authentication in the request, which is not yet supported
	MfaDelete string
}

// GetBucketVersioningOutput is returned by GetBucketVersioning.
type GetBucketVersioningOutput struct {
	Status    string
	MfaDelete string
}

// versioningConfigurationXML is the internal type for XML marshaling.
type versioningConfigurationXML struct {
	XMLName   xml.Name `xml:"VersioningConfiguration"`
	XMLNS     string   `xml:"xmlns,attr"`
	Status    string   `xml:"Status"`
	MfaDelete string   `xml:"MfaDelete,omitempty"`
}

// PutBucketVersioning sets the versioning configuration for a bucket.
func (s3 *S3) PutBucketVersioning(input PutBucketVersioningInput) error {
	// Validate input
	if input.Bucket == "" {
		return fmt.Errorf("bucket name is required")
	}
	if input.Status != "Enabled" && input.Status != "Suspended" {
		return fmt.Errorf("status must be 'Enabled' or 'Suspended'")
	}

	// Renew IAM token if needed
	if err := s3.renewIAMToken(); err != nil {
		return err
	}

	// Build XML request body
	config := versioningConfigurationXML{
		XMLNS:     "http://s3.amazonaws.com/doc/2006-03-01/",
		Status:    input.Status,
		MfaDelete: input.MfaDelete,
	}

	xmlBody, err := xml.Marshal(config)
	if err != nil {
		return err
	}

	// Calculate Content-MD5
	md5Hash := md5.Sum(xmlBody)
	contentMD5 := base64.StdEncoding.EncodeToString(md5Hash[:])

	// Build URL with ?versioning query parameter
	baseURL := s3.getURL(input.Bucket)
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return err
	}
	parsedURL.RawQuery = "versioning"

	// Create PUT request
	req, err := http.NewRequest(http.MethodPut, parsedURL.String(), bytes.NewReader(xmlBody))
	if err != nil {
		return err
	}

	// Set headers
	req.ContentLength = int64(len(xmlBody))
	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("Content-MD5", contentMD5)

	// Calculate SHA256
	h := sha256.New()
	h.Write(xmlBody)
	req.Header.Set("x-amz-content-sha256", fmt.Sprintf("%x", h.Sum(nil)))
	req.Header.Set("Host", req.URL.Host)

	// Sign the request
	if err := s3.signRequest(req); err != nil {
		return err
	}

	// Execute request
	client := s3.getClient()
	res, err := client.Do(req)
	if err != nil {
		return err
	}

	defer func() {
		res.Body.Close()
		io.Copy(io.Discard, res.Body)
	}()

	// Read response body
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	// Handle non-OK status codes
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("status code: %s: %s", res.Status, string(body))
	}

	return nil
}

// GetBucketVersioning gets the versioning configuration for a bucket.
func (s3 *S3) GetBucketVersioning(bucket string) (GetBucketVersioningOutput, error) {
	// Validate input
	if bucket == "" {
		return GetBucketVersioningOutput{}, fmt.Errorf("bucket name is required")
	}

	// Renew IAM token if needed
	if err := s3.renewIAMToken(); err != nil {
		return GetBucketVersioningOutput{}, err
	}

	// Build URL with ?versioning query parameter
	baseURL := s3.getURL(bucket)
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return GetBucketVersioningOutput{}, err
	}
	parsedURL.RawQuery = "versioning"

	// Create GET request
	req, err := http.NewRequest(http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return GetBucketVersioningOutput{}, err
	}

	// Sign the request
	if err := s3.signRequest(req); err != nil {
		return GetBucketVersioningOutput{}, err
	}

	// Execute request
	client := s3.getClient()
	res, err := client.Do(req)
	if err != nil {
		return GetBucketVersioningOutput{}, err
	}

	defer func() {
		res.Body.Close()
		io.Copy(io.Discard, res.Body)
	}()

	// Read response body
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return GetBucketVersioningOutput{}, err
	}

	// Handle non-OK status codes
	if res.StatusCode != http.StatusOK {
		return GetBucketVersioningOutput{}, fmt.Errorf("status code: %s: %s", res.Status, string(body))
	}

	// Parse XML response
	var config VersioningConfiguration
	if err := xml.Unmarshal(body, &config); err != nil {
		return GetBucketVersioningOutput{}, err
	}

	return GetBucketVersioningOutput{
		Status:    config.Status,
		MfaDelete: config.MfaDelete,
	}, nil
}
