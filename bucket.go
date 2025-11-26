// LICENSE BSD-2-Clause-FreeBSD
// Copyright (c) 2018, Rohan Verma <hello@rohanverma.net>

package simples3

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
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
