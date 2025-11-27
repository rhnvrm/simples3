// LICENSE BSD-2-Clause-FreeBSD
// Copyright (c) 2018, Rohan Verma <hello@rohanverma.net>

package simples3

import (
	"encoding/xml"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/url"
)

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

// ListVersionsInput is passed to ListVersions as a parameter.
type ListVersionsInput struct {
	// Required: The name of the bucket to list versions from
	Bucket string

	// Optional: A delimiter to group objects by (commonly "/")
	Delimiter string

	// Optional: Only list objects starting with this prefix
	Prefix string

	// Optional: Maximum number of objects to return
	MaxKeys int64

	// Optional: Key to start listing after (used for pagination)
	KeyMarker string

	// Optional: Version ID to start listing after (used for pagination)
	VersionIdMarker string
}

// ObjectVersion represents a version of an object.
type ObjectVersion struct {
	ETag         string `xml:"ETag"`
	IsLatest     bool   `xml:"IsLatest"`
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	Size         int64  `xml:"Size"`
	StorageClass string `xml:"StorageClass"`
	VersionId    string `xml:"VersionId"`
	Owner        struct {
		ID          string `xml:"ID"`
		DisplayName string `xml:"DisplayName"`
	} `xml:"Owner"`
}

// DeleteMarker represents a delete marker for an object.
type DeleteMarker struct {
	IsLatest     bool   `xml:"IsLatest"`
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	VersionId    string `xml:"VersionId"`
	Owner        struct {
		ID          string `xml:"ID"`
		DisplayName string `xml:"DisplayName"`
	} `xml:"Owner"`
}

// ListVersionsResponse is returned by ListVersions.
type ListVersionsResponse struct {
	Name                string          `xml:"Name"`
	Prefix              string          `xml:"Prefix"`
	KeyMarker           string          `xml:"KeyMarker"`
	VersionIdMarker     string          `xml:"VersionIdMarker"`
	MaxKeys             int64           `xml:"MaxKeys"`
	Delimiter           string          `xml:"Delimiter"`
	IsTruncated         bool            `xml:"IsTruncated"`
	NextKeyMarker       string          `xml:"NextKeyMarker"`
	NextVersionIdMarker string          `xml:"NextVersionIdMarker"`
	Versions            []ObjectVersion `xml:"Version"`
	DeleteMarkers       []DeleteMarker  `xml:"DeleteMarker"`
	CommonPrefixes      []string
}

// ListVersions lists object versions in a bucket.
func (s3 *S3) ListVersions(input ListVersionsInput) (ListVersionsResponse, error) {
	// Input validation
	if input.Bucket == "" {
		return ListVersionsResponse{}, fmt.Errorf("bucket name cannot be empty")
	}
	if input.MaxKeys < 0 {
		return ListVersionsResponse{}, fmt.Errorf("MaxKeys cannot be negative")
	}
	if input.MaxKeys > 1000 {
		return ListVersionsResponse{}, fmt.Errorf("MaxKeys cannot exceed 1000")
	}

	// Build query parameters
	query := url.Values{}
	query.Set("versions", "") // Enables version listing

	// Add optional parameters if they exist
	if input.Delimiter != "" {
		query.Set("delimiter", input.Delimiter)
	}
	if input.Prefix != "" {
		query.Set("prefix", input.Prefix)
	}
	if input.MaxKeys > 0 {
		query.Set("max-keys", fmt.Sprintf("%d", input.MaxKeys))
	}
	if input.KeyMarker != "" {
		query.Set("key-marker", input.KeyMarker)
	}
	if input.VersionIdMarker != "" {
		query.Set("version-id-marker", input.VersionIdMarker)
	}

	// Build base URL
	baseURL := s3.getURL(input.Bucket)

	// Parse URL and add query parameters
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return ListVersionsResponse{}, err
	}
	parsedURL.RawQuery = query.Encode()

	// Create HTTP request
	req, err := http.NewRequest(http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return ListVersionsResponse{}, err
	}

	// Apply AWS V4 signing
	if err := s3.renewIAMToken(); err != nil {
		return ListVersionsResponse{}, err
	}
	if err := s3.signRequest(req); err != nil {
		return ListVersionsResponse{}, err
	}

	// Execute request
	client := s3.getClient()
	res, err := client.Do(req)
	if err != nil {
		return ListVersionsResponse{}, err
	}

	// Close response body when done
	defer res.Body.Close()

	// Read response body once
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return ListVersionsResponse{}, err
	}

	// Handle response status codes
	if res.StatusCode != http.StatusOK {
		var s3Err S3Error
		if xmlErr := xml.Unmarshal(body, &s3Err); xmlErr == nil {
			return ListVersionsResponse{}, fmt.Errorf("S3 Error: %s - %s", s3Err.Code, s3Err.Message)
		}
		return ListVersionsResponse{}, fmt.Errorf("status code: %s: %s", res.Status, string(body))
	}

	// Parse XML response using internal struct for CommonPrefixes handling
	var xmlResult struct {
		XMLName             xml.Name        `xml:"ListVersionsResult"`
		Name                string          `xml:"Name"`
		Prefix              string          `xml:"Prefix"`
		KeyMarker           string          `xml:"KeyMarker"`
		VersionIdMarker     string          `xml:"VersionIdMarker"`
		MaxKeys             int64           `xml:"MaxKeys"`
		Delimiter           string          `xml:"Delimiter"`
		IsTruncated         bool            `xml:"IsTruncated"`
		NextKeyMarker       string          `xml:"NextKeyMarker"`
		NextVersionIdMarker string          `xml:"NextVersionIdMarker"`
		Versions            []ObjectVersion `xml:"Version"`
		DeleteMarkers       []DeleteMarker  `xml:"DeleteMarker"`
		CommonPrefixes      []struct {
			Prefix string `xml:"Prefix"`
		} `xml:"CommonPrefixes"`
	}

	if err := xml.Unmarshal(body, &xmlResult); err != nil {
		return ListVersionsResponse{}, err
	}

	// Convert to simple response format
	response := ListVersionsResponse{
		Name:                xmlResult.Name,
		Prefix:              xmlResult.Prefix,
		KeyMarker:           xmlResult.KeyMarker,
		VersionIdMarker:     xmlResult.VersionIdMarker,
		MaxKeys:             xmlResult.MaxKeys,
		Delimiter:           xmlResult.Delimiter,
		IsTruncated:         xmlResult.IsTruncated,
		NextKeyMarker:       xmlResult.NextKeyMarker,
		NextVersionIdMarker: xmlResult.NextVersionIdMarker,
		Versions:            xmlResult.Versions,
		DeleteMarkers:       xmlResult.DeleteMarkers,
	}

	// Convert common prefixes
	for _, prefix := range xmlResult.CommonPrefixes {
		response.CommonPrefixes = append(response.CommonPrefixes, prefix.Prefix)
	}

	return response, nil
}
