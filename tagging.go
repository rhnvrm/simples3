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
)

// Tag represents a single S3 object tag with a key-value pair.
type Tag struct {
	Key   string `xml:"Key"`
	Value string `xml:"Value"`
}

// tagging is the internal type for XML marshaling/unmarshaling.
type tagging struct {
	XMLName xml.Name `xml:"Tagging"`
	XMLNS   string   `xml:"xmlns,attr,omitempty"`
	TagSet  tagSet   `xml:"TagSet"`
}

// tagSet is the internal type for XML marshaling/unmarshaling.
type tagSet struct {
	Tags []Tag `xml:"Tag"`
}

// PutObjectTaggingInput is passed to PutObjectTagging as a parameter.
type PutObjectTaggingInput struct {
	// Required: The name of the bucket
	Bucket string

	// Required: The object key
	ObjectKey string

	// Required: Tags to set on the object (max 10 tags)
	Tags map[string]string
}

// GetObjectTaggingInput is passed to GetObjectTagging as a parameter.
type GetObjectTaggingInput struct {
	// Required: The name of the bucket
	Bucket string

	// Required: The object key
	ObjectKey string
}

// GetObjectTaggingOutput is returned by GetObjectTagging.
type GetObjectTaggingOutput struct {
	Tags map[string]string
}

// DeleteObjectTaggingInput is passed to DeleteObjectTagging as a parameter.
type DeleteObjectTaggingInput struct {
	// Required: The name of the bucket
	Bucket string

	// Required: The object key
	ObjectKey string
}

// PutObjectTagging sets tags on an existing S3 object.
// Replaces all existing tags with the provided tags.
// S3 allows up to 10 tags per object.
func (s3 *S3) PutObjectTagging(input PutObjectTaggingInput) error {
	// Validate required fields
	if input.Bucket == "" {
		return fmt.Errorf("bucket name is required")
	}
	if input.ObjectKey == "" {
		return fmt.Errorf("object key is required")
	}
	if len(input.Tags) == 0 {
		return fmt.Errorf("at least one tag is required")
	}
	if len(input.Tags) > 10 {
		return fmt.Errorf("cannot set more than 10 tags per object")
	}

	// Renew IAM token if needed
	if err := s3.renewIAMToken(); err != nil {
		return err
	}

	// Build XML request body
	tagsXML := tagging{
		XMLNS: "http://s3.amazonaws.com/doc/2006-03-01/",
		TagSet: tagSet{
			Tags: make([]Tag, 0, len(input.Tags)),
		},
	}
	for k, v := range input.Tags {
		tagsXML.TagSet.Tags = append(tagsXML.TagSet.Tags, Tag{Key: k, Value: v})
	}

	xmlBody, err := xml.Marshal(tagsXML)
	if err != nil {
		return err
	}

	// Calculate Content-MD5 (required by S3 for this operation)
	md5Hash := md5.Sum(xmlBody)
	contentMD5 := base64.StdEncoding.EncodeToString(md5Hash[:])

	// Build URL with ?tagging query parameter
	baseURL := s3.getURL(input.Bucket, input.ObjectKey)
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return err
	}
	query := url.Values{}
	query.Set("tagging", "")
	parsedURL.RawQuery = query.Encode()

	// Create PUT request
	req, err := http.NewRequest(http.MethodPut, parsedURL.String(), bytes.NewReader(xmlBody))
	if err != nil {
		return err
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

// GetObjectTagging retrieves the tags associated with an S3 object.
func (s3 *S3) GetObjectTagging(input GetObjectTaggingInput) (GetObjectTaggingOutput, error) {
	// Validate required fields
	if input.Bucket == "" {
		return GetObjectTaggingOutput{}, fmt.Errorf("bucket name is required")
	}
	if input.ObjectKey == "" {
		return GetObjectTaggingOutput{}, fmt.Errorf("object key is required")
	}

	// Renew IAM token if needed
	if err := s3.renewIAMToken(); err != nil {
		return GetObjectTaggingOutput{}, err
	}

	// Build URL with ?tagging query parameter
	baseURL := s3.getURL(input.Bucket, input.ObjectKey)
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return GetObjectTaggingOutput{}, err
	}
	parsedURL.RawQuery = "tagging"

	// Create GET request
	req, err := http.NewRequest(http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return GetObjectTaggingOutput{}, err
	}

	// Sign the request
	if err := s3.signRequest(req); err != nil {
		return GetObjectTaggingOutput{}, err
	}

	// Execute request
	client := s3.getClient()
	res, err := client.Do(req)
	if err != nil {
		return GetObjectTaggingOutput{}, err
	}

	defer func() {
		res.Body.Close()
		io.Copy(io.Discard, res.Body)
	}()

	// Read response body
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return GetObjectTaggingOutput{}, err
	}

	// Handle non-OK status codes
	if res.StatusCode != http.StatusOK {
		return GetObjectTaggingOutput{}, fmt.Errorf("status code: %s: %s", res.Status, string(body))
	}

	// Parse XML response
	var result tagging
	if err := xml.Unmarshal(body, &result); err != nil {
		return GetObjectTaggingOutput{}, err
	}

	// Convert to map
	output := GetObjectTaggingOutput{
		Tags: make(map[string]string, len(result.TagSet.Tags)),
	}
	for _, tag := range result.TagSet.Tags {
		output.Tags[tag.Key] = tag.Value
	}

	return output, nil
}

// DeleteObjectTagging removes all tags from an S3 object.
func (s3 *S3) DeleteObjectTagging(input DeleteObjectTaggingInput) error {
	// Validate required fields
	if input.Bucket == "" {
		return fmt.Errorf("bucket name is required")
	}
	if input.ObjectKey == "" {
		return fmt.Errorf("object key is required")
	}

	// Renew IAM token if needed
	if err := s3.renewIAMToken(); err != nil {
		return err
	}

	// Build URL with ?tagging query parameter
	baseURL := s3.getURL(input.Bucket, input.ObjectKey)
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return err
	}
	query := url.Values{}
	query.Set("tagging", "")
	parsedURL.RawQuery = query.Encode()

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

	// Read response body for error messages
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
