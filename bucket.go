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

// --- ACL Operations ---

// AccessControlPolicy represents the Access Control List (ACL) for a bucket or object.
type AccessControlPolicy struct {
	XMLName           xml.Name `xml:"AccessControlPolicy"`
	XMLNS             string   `xml:"xmlns,attr,omitempty"`
	Owner             Owner    `xml:"Owner"`
	AccessControlList []Grant  `xml:"AccessControlList>Grant"`
}

// Owner represents the owner of the bucket or object.
type Owner struct {
	ID          string `xml:"ID,omitempty"`
	DisplayName string `xml:"DisplayName,omitempty"`
}

// Grant represents a permission grant.
type Grant struct {
	Grantee    Grantee `xml:"Grantee"`
	Permission string  `xml:"Permission"`
}

// Grantee represents the recipient of the permission grant.
type Grantee struct {
	XMLNS        string `xml:"xmlns:xsi,attr,omitempty"`
	Type         string `xml:"xsi:type,attr"` // CanonicalUser, AmazonCustomerByEmail, Group
	ID           string `xml:"ID,omitempty"`
	DisplayName  string `xml:"DisplayName,omitempty"`
	URI          string `xml:"URI,omitempty"`          // For Group
	EmailAddress string `xml:"EmailAddress,omitempty"` // For AmazonCustomerByEmail
}

// PutBucketAclInput is passed to PutBucketAcl.
type PutBucketAclInput struct {
	// Required: The name of the bucket
	Bucket string

	// Optional: Canned ACL (private, public-read, public-read-write, authenticated-read)
	// If CannedACL is set, AccessControlPolicy is ignored.
	CannedACL string

	// Optional: Full Access Control Policy
	AccessControlPolicy *AccessControlPolicy
}

// PutBucketAcl sets the Access Control List (ACL) for a bucket.
// You can either use a CannedACL OR provide a full AccessControlPolicy.
func (s3 *S3) PutBucketAcl(input PutBucketAclInput) error {
	// Validate input
	if input.Bucket == "" {
		return fmt.Errorf("bucket name is required")
	}
	if input.CannedACL == "" && input.AccessControlPolicy == nil {
		return fmt.Errorf("either CannedACL or AccessControlPolicy must be provided")
	}

	// Renew IAM token if needed
	if err := s3.renewIAMToken(); err != nil {
		return err
	}

	// Build URL with ?acl query parameter
	baseURL := s3.getURL(input.Bucket)
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return err
	}
	parsedURL.RawQuery = "acl"

	var req *http.Request
	var bodyReader io.Reader
	var contentMD5 string
	var sha256Hash string

	if input.AccessControlPolicy != nil {
		// Use custom ACL body
		input.AccessControlPolicy.XMLNS = "http://s3.amazonaws.com/doc/2006-03-01/"
		// Ensure namespace is set on Grantees if needed, though usually top level is enough.
		// Some S3 implementations require xsi namespace.
		for i := range input.AccessControlPolicy.AccessControlList {
			input.AccessControlPolicy.AccessControlList[i].Grantee.XMLNS = "http://www.w3.org/2001/XMLSchema-instance"
		}

		xmlBody, err := xml.Marshal(input.AccessControlPolicy)
		if err != nil {
			return err
		}

		bodyReader = bytes.NewReader(xmlBody)

		// Calculate MD5
		md5Sum := md5.Sum(xmlBody)
		contentMD5 = base64.StdEncoding.EncodeToString(md5Sum[:])

		// Calculate SHA256
		h := sha256.New()
		h.Write(xmlBody)
		sha256Hash = fmt.Sprintf("%x", h.Sum(nil))

		req, err = http.NewRequest(http.MethodPut, parsedURL.String(), bodyReader)
		if err != nil {
			return err
		}
		req.ContentLength = int64(len(xmlBody))
		req.Header.Set("Content-Type", "application/xml")
		req.Header.Set("Content-MD5", contentMD5)
		req.Header.Set("x-amz-content-sha256", sha256Hash)

	} else {
		// Use Canned ACL via header
		req, err = http.NewRequest(http.MethodPut, parsedURL.String(), nil)
		if err != nil {
			return err
		}
		req.Header.Set("x-amz-acl", input.CannedACL)
		// Empty body SHA256
		h := sha256.New()
		sha256Hash = fmt.Sprintf("%x", h.Sum(nil))
		req.Header.Set("x-amz-content-sha256", sha256Hash)
	}

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

// GetBucketAcl gets the Access Control List (ACL) for a bucket.
func (s3 *S3) GetBucketAcl(bucket string) (AccessControlPolicy, error) {
	// Validate input
	if bucket == "" {
		return AccessControlPolicy{}, fmt.Errorf("bucket name is required")
	}

	// Renew IAM token if needed
	if err := s3.renewIAMToken(); err != nil {
		return AccessControlPolicy{}, err
	}

	// Build URL with ?acl query parameter
	baseURL := s3.getURL(bucket)
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return AccessControlPolicy{}, err
	}
	parsedURL.RawQuery = "acl"

	// Create GET request
	req, err := http.NewRequest(http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return AccessControlPolicy{}, err
	}

	// Sign the request
	if err := s3.signRequest(req); err != nil {
		return AccessControlPolicy{}, err
	}

	// Execute request
	client := s3.getClient()
	res, err := client.Do(req)
	if err != nil {
		return AccessControlPolicy{}, err
	}

	defer func() {
		res.Body.Close()
		io.Copy(io.Discard, res.Body)
	}()

	// Read response body
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return AccessControlPolicy{}, err
	}

	// Handle non-OK status codes
	if res.StatusCode != http.StatusOK {
		return AccessControlPolicy{}, fmt.Errorf("status code: %s: %s", res.Status, string(body))
	}

	// Parse XML response
	var policy AccessControlPolicy
	if err := xml.Unmarshal(body, &policy); err != nil {
		return AccessControlPolicy{}, err
	}

	return policy, nil
}

// --- Lifecycle Operations ---

// LifecycleConfiguration represents the lifecycle configuration for a bucket.
type LifecycleConfiguration struct {
	XMLName xml.Name        `xml:"LifecycleConfiguration"`
	XMLNS   string          `xml:"xmlns,attr,omitempty"`
	Rules   []LifecycleRule `xml:"Rule"`
}

// LifecycleRule represents a single lifecycle rule.
type LifecycleRule struct {
	ID     string           `xml:"ID,omitempty"`
	Status string           `xml:"Status"` // Enabled or Disabled
	Filter *LifecycleFilter `xml:"Filter,omitempty"`
	// Legacy Prefix support (S3 supports both, but prefer Filter for V2)
	// If Prefix is provided here, do not use Filter.
	Prefix                         *string                                  `xml:"Prefix,omitempty"`
	Expiration                     *LifecycleExpiration                     `xml:"Expiration,omitempty"`
	Transitions                    []LifecycleTransition                    `xml:"Transition,omitempty"`
	NoncurrentVersionExpiration    *LifecycleNoncurrentVersionExpiration    `xml:"NoncurrentVersionExpiration,omitempty"`
	NoncurrentVersionTransitions   []LifecycleNoncurrentVersionTransition   `xml:"NoncurrentVersionTransition,omitempty"`
	AbortIncompleteMultipartUpload *LifecycleAbortIncompleteMultipartUpload `xml:"AbortIncompleteMultipartUpload,omitempty"`
}

// LifecycleFilter represents the filter for a lifecycle rule.
type LifecycleFilter struct {
	Prefix string `xml:"Prefix,omitempty"`
	Tag    *Tag   `xml:"Tag,omitempty"`
	And    *struct {
		Prefix string `xml:"Prefix,omitempty"`
		Tags   []Tag  `xml:"Tag,omitempty"`
	} `xml:"And,omitempty"`
}

// LifecycleExpiration represents the expiration action.
type LifecycleExpiration struct {
	Date                      string `xml:"Date,omitempty"` // ISO 8601 format
	Days                      int    `xml:"Days,omitempty"`
	ExpiredObjectDeleteMarker bool   `xml:"ExpiredObjectDeleteMarker,omitempty"`
}

// LifecycleTransition represents a transition action.
type LifecycleTransition struct {
	Date         string `xml:"Date,omitempty"`
	Days         int    `xml:"Days,omitempty"`
	StorageClass string `xml:"StorageClass"`
}

// LifecycleNoncurrentVersionExpiration represents expiration for noncurrent versions.
type LifecycleNoncurrentVersionExpiration struct {
	NoncurrentDays int `xml:"NoncurrentDays"`
}

// LifecycleNoncurrentVersionTransition represents transition for noncurrent versions.
type LifecycleNoncurrentVersionTransition struct {
	NoncurrentDays int    `xml:"NoncurrentDays"`
	StorageClass   string `xml:"StorageClass"`
}

// LifecycleAbortIncompleteMultipartUpload represents abort action for incomplete uploads.
type LifecycleAbortIncompleteMultipartUpload struct {
	DaysAfterInitiation int `xml:"DaysAfterInitiation"`
}

// PutBucketLifecycleInput is passed to PutBucketLifecycle.
type PutBucketLifecycleInput struct {
	// Required: The name of the bucket
	Bucket string

	// Required: The lifecycle configuration
	Configuration *LifecycleConfiguration
}

// PutBucketLifecycle sets the lifecycle configuration for a bucket.
func (s3 *S3) PutBucketLifecycle(input PutBucketLifecycleInput) error {
	// Validate input
	if input.Bucket == "" {
		return fmt.Errorf("bucket name is required")
	}
	if input.Configuration == nil || len(input.Configuration.Rules) == 0 {
		return fmt.Errorf("lifecycle configuration with at least one rule is required")
	}

	// Renew IAM token if needed
	if err := s3.renewIAMToken(); err != nil {
		return err
	}

	// Build URL with ?lifecycle query parameter
	baseURL := s3.getURL(input.Bucket)
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return err
	}
	parsedURL.RawQuery = "lifecycle"

	// Set XML Namespace
	input.Configuration.XMLNS = "http://s3.amazonaws.com/doc/2006-03-01/"

	xmlBody, err := xml.Marshal(input.Configuration)
	if err != nil {
		return err
	}

	// Calculate MD5
	md5Hash := md5.Sum(xmlBody)
	contentMD5 := base64.StdEncoding.EncodeToString(md5Hash[:])

	// Calculate SHA256
	h := sha256.New()
	h.Write(xmlBody)
	sha256Hash := fmt.Sprintf("%x", h.Sum(nil))

	// Create PUT request
	req, err := http.NewRequest(http.MethodPut, parsedURL.String(), bytes.NewReader(xmlBody))
	if err != nil {
		return err
	}

	req.ContentLength = int64(len(xmlBody))
	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("Content-MD5", contentMD5)
	req.Header.Set("x-amz-content-sha256", sha256Hash)
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

// GetBucketLifecycle gets the lifecycle configuration for a bucket.
func (s3 *S3) GetBucketLifecycle(bucket string) (LifecycleConfiguration, error) {
	// Validate input
	if bucket == "" {
		return LifecycleConfiguration{}, fmt.Errorf("bucket name is required")
	}

	// Renew IAM token if needed
	if err := s3.renewIAMToken(); err != nil {
		return LifecycleConfiguration{}, err
	}

	// Build URL with ?lifecycle query parameter
	baseURL := s3.getURL(bucket)
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return LifecycleConfiguration{}, err
	}
	parsedURL.RawQuery = "lifecycle"

	// Create GET request
	req, err := http.NewRequest(http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return LifecycleConfiguration{}, err
	}

	// Sign the request
	if err := s3.signRequest(req); err != nil {
		return LifecycleConfiguration{}, err
	}

	// Execute request
	client := s3.getClient()
	res, err := client.Do(req)
	if err != nil {
		return LifecycleConfiguration{}, err
	}

	defer func() {
		res.Body.Close()
		io.Copy(io.Discard, res.Body)
	}()

	// Read response body
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return LifecycleConfiguration{}, err
	}

	// Handle non-OK status codes
	if res.StatusCode != http.StatusOK {
		// S3 returns 404 if no lifecycle configuration exists
		if res.StatusCode == http.StatusNotFound {
			// Check if it's actually "NoSuchLifecycleConfiguration"
			if strings.Contains(string(body), "NoSuchLifecycleConfiguration") {
				return LifecycleConfiguration{}, fmt.Errorf("no lifecycle configuration found")
			}
		}
		return LifecycleConfiguration{}, fmt.Errorf("status code: %s: %s", res.Status, string(body))
	}

	// Parse XML response
	var config LifecycleConfiguration
	if err := xml.Unmarshal(body, &config); err != nil {
		return LifecycleConfiguration{}, err
	}

	return config, nil
}

// DeleteBucketLifecycle deletes the lifecycle configuration for a bucket.
func (s3 *S3) DeleteBucketLifecycle(input DeleteBucketInput) error {
	// Reuse DeleteBucketInput since it just needs the bucket name

	// Validate input
	if input.Bucket == "" {
		return fmt.Errorf("bucket name is required")
	}

	// Renew IAM token if needed
	if err := s3.renewIAMToken(); err != nil {
		return err
	}

	// Build URL with ?lifecycle query parameter
	baseURL := s3.getURL(input.Bucket)
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return err
	}
	parsedURL.RawQuery = "lifecycle"

	// Create DELETE request
	req, err := http.NewRequest(http.MethodDelete, parsedURL.String(), nil)
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

	// Read response body
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	// Handle non-success status codes
	// AWS returns 204 No Content on successful deletion
	if res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("status code: %s: %s", res.Status, string(body))
	}

	return nil
}
