package simples3

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// Multipart upload constants
const (
	MinPartSize          = 5 * 1024 * 1024        // 5 MB
	MaxPartSize          = 5 * 1024 * 1024 * 1024 // 5 GB
	MaxParts             = 10000
	DefaultPartSize      = 5 * 1024 * 1024 // 5 MB
	DefaultMaxRetries    = 3
	DefaultRetryBaseWait = 100 * time.Millisecond
	DefaultRetryMaxWait  = 5 * time.Second
)

// InitiateMultipartUploadInput contains parameters for initiating a multipart upload
type InitiateMultipartUploadInput struct {
	Bucket         string            // Required: bucket name
	ObjectKey      string            // Required: object key
	ContentType    string            // Optional: content type
	CustomMetadata map[string]string // Optional: x-amz-meta-* headers
	ACL            string            // Optional: x-amz-acl
}

// InitiateMultipartUploadOutput contains the response from initiating a multipart upload
type InitiateMultipartUploadOutput struct {
	Bucket   string
	Key      string
	UploadID string
}

// initiateMultipartUploadResult is the internal XML response structure
type initiateMultipartUploadResult struct {
	XMLName  xml.Name `xml:"InitiateMultipartUploadResult"`
	Bucket   string   `xml:"Bucket"`
	Key      string   `xml:"Key"`
	UploadID string   `xml:"UploadId"`
}

// InitiateMultipartUpload initiates a multipart upload and returns an upload ID
func (s3 *S3) InitiateMultipartUpload(input InitiateMultipartUploadInput) (InitiateMultipartUploadOutput, error) {
	if input.Bucket == "" {
		return InitiateMultipartUploadOutput{}, fmt.Errorf("bucket name is required")
	}
	if input.ObjectKey == "" {
		return InitiateMultipartUploadOutput{}, fmt.Errorf("object key is required")
	}

	if err := s3.renewIAMToken(); err != nil {
		return InitiateMultipartUploadOutput{}, err
	}

	urlStr := s3.getURL(input.Bucket, input.ObjectKey) + "?uploads"

	req, err := http.NewRequest(http.MethodPost, urlStr, nil)
	if err != nil {
		return InitiateMultipartUploadOutput{}, err
	}

	// Set headers
	if input.ContentType != "" {
		req.Header.Set("Content-Type", input.ContentType)
	}

	if input.ACL != "" {
		req.Header.Set("x-amz-acl", input.ACL)
	}

	// Set custom metadata
	for k, v := range input.CustomMetadata {
		req.Header.Set(AMZMetaPrefix+k, v)
	}

	// Empty body hash for POST with no body
	req.Header.Set("x-amz-content-sha256", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")

	if err := s3.signRequest(req); err != nil {
		return InitiateMultipartUploadOutput{}, err
	}

	client := s3.getClient()
	res, err := client.Do(req)
	if err != nil {
		return InitiateMultipartUploadOutput{}, err
	}
	defer func() {
		res.Body.Close()
		io.Copy(io.Discard, res.Body)
	}()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return InitiateMultipartUploadOutput{}, err
	}

	if res.StatusCode != http.StatusOK {
		return InitiateMultipartUploadOutput{}, fmt.Errorf("status code: %s: %s", res.Status, string(body))
	}

	var result initiateMultipartUploadResult
	if err := xml.Unmarshal(body, &result); err != nil {
		return InitiateMultipartUploadOutput{}, err
	}

	return InitiateMultipartUploadOutput{
		Bucket:   result.Bucket,
		Key:      result.Key,
		UploadID: result.UploadID,
	}, nil
}

// UploadPartInput contains parameters for uploading a part
type UploadPartInput struct {
	Bucket     string    // Required: bucket name
	ObjectKey  string    // Required: object key
	UploadID   string    // Required: upload ID from InitiateMultipartUpload
	PartNumber int       // Required: part number (1-10000)
	Body       io.Reader // Required: part data
	Size       int64     // Required: size of part for Content-Length
}

// UploadPartOutput contains the response from uploading a part
type UploadPartOutput struct {
	ETag       string // Required for CompleteMultipartUpload
	PartNumber int
}

// UploadPart uploads a single part for a multipart upload
func (s3 *S3) UploadPart(input UploadPartInput) (UploadPartOutput, error) {
	if input.Bucket == "" {
		return UploadPartOutput{}, fmt.Errorf("bucket name is required")
	}
	if input.ObjectKey == "" {
		return UploadPartOutput{}, fmt.Errorf("object key is required")
	}
	if input.UploadID == "" {
		return UploadPartOutput{}, fmt.Errorf("upload ID is required")
	}
	if input.PartNumber < 1 || input.PartNumber > MaxParts {
		return UploadPartOutput{}, fmt.Errorf("part number must be between 1 and %d", MaxParts)
	}
	if input.Body == nil {
		return UploadPartOutput{}, fmt.Errorf("body is required")
	}
	if input.Size <= 0 {
		return UploadPartOutput{}, fmt.Errorf("size must be greater than 0")
	}

	if err := s3.renewIAMToken(); err != nil {
		return UploadPartOutput{}, err
	}

	// Build URL with query parameters
	urlStr := s3.getURL(input.Bucket, input.ObjectKey)
	params := url.Values{}
	params.Set("partNumber", strconv.Itoa(input.PartNumber))
	params.Set("uploadId", input.UploadID)
	urlStr += "?" + params.Encode()

	// Read body and compute SHA256
	content, err := io.ReadAll(input.Body)
	if err != nil {
		return UploadPartOutput{}, err
	}

	req, err := http.NewRequest(http.MethodPut, urlStr, bytes.NewReader(content))
	if err != nil {
		return UploadPartOutput{}, err
	}

	// Set headers
	req.ContentLength = input.Size
	h := sha256.New()
	h.Write(content)
	req.Header.Set("x-amz-content-sha256", fmt.Sprintf("%x", h.Sum(nil)))

	if err := s3.signRequest(req); err != nil {
		return UploadPartOutput{}, err
	}

	client := s3.getClient()
	res, err := client.Do(req)
	if err != nil {
		return UploadPartOutput{}, err
	}
	defer func() {
		res.Body.Close()
		io.Copy(io.Discard, res.Body)
	}()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return UploadPartOutput{}, err
	}

	if res.StatusCode != http.StatusOK {
		return UploadPartOutput{}, fmt.Errorf("status code: %s: %s", res.Status, string(body))
	}

	etag := res.Header.Get("ETag")
	if etag == "" {
		return UploadPartOutput{}, fmt.Errorf("ETag not found in response")
	}

	return UploadPartOutput{
		ETag:       etag,
		PartNumber: input.PartNumber,
	}, nil
}

// uploadPartWithRetry uploads a part with retry logic
func (s3 *S3) uploadPartWithRetry(input UploadPartInput, maxRetries int) (UploadPartOutput, error) {
	if maxRetries <= 0 {
		maxRetries = DefaultMaxRetries
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			waitTime := time.Duration(math.Pow(2, float64(attempt-1))) * DefaultRetryBaseWait
			if waitTime > DefaultRetryMaxWait {
				waitTime = DefaultRetryMaxWait
			}
			time.Sleep(waitTime)
		}

		output, err := s3.UploadPart(input)
		if err == nil {
			return output, nil
		}

		lastErr = err

		// Check if error is retryable
		if !isRetryableError(err) {
			return UploadPartOutput{}, err
		}
	}

	return UploadPartOutput{}, fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}

// isRetryableError determines if an error should be retried
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	// Retry on 5xx errors, timeouts, and connection resets
	return contains(errStr, "status code: 5") ||
		contains(errStr, "timeout") ||
		contains(errStr, "connection reset") ||
		contains(errStr, "EOF")
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// CompletedPart represents a part that has been uploaded
type CompletedPart struct {
	PartNumber int
	ETag       string
}

// CompleteMultipartUploadInput contains parameters for completing a multipart upload
type CompleteMultipartUploadInput struct {
	Bucket    string          // Required: bucket name
	ObjectKey string          // Required: object key
	UploadID  string          // Required: upload ID
	Parts     []CompletedPart // Required: list of parts, ordered by PartNumber
}

// CompleteMultipartUploadOutput contains the response from completing a multipart upload
type CompleteMultipartUploadOutput struct {
	Location string
	Bucket   string
	Key      string
	ETag     string
}

// completeMultipartUploadRequest is the XML request structure
type completeMultipartUploadRequest struct {
	XMLName xml.Name       `xml:"CompleteMultipartUpload"`
	XMLNS   string         `xml:"xmlns,attr"`
	Parts   []completePart `xml:"Part"`
}

type completePart struct {
	PartNumber int    `xml:"PartNumber"`
	ETag       string `xml:"ETag"`
}

// completeMultipartUploadResult is the XML response structure
type completeMultipartUploadResult struct {
	XMLName  xml.Name `xml:"CompleteMultipartUploadResult"`
	Location string   `xml:"Location"`
	Bucket   string   `xml:"Bucket"`
	Key      string   `xml:"Key"`
	ETag     string   `xml:"ETag"`
}

// CompleteMultipartUpload completes a multipart upload
func (s3 *S3) CompleteMultipartUpload(input CompleteMultipartUploadInput) (CompleteMultipartUploadOutput, error) {
	if input.Bucket == "" {
		return CompleteMultipartUploadOutput{}, fmt.Errorf("bucket name is required")
	}
	if input.ObjectKey == "" {
		return CompleteMultipartUploadOutput{}, fmt.Errorf("object key is required")
	}
	if input.UploadID == "" {
		return CompleteMultipartUploadOutput{}, fmt.Errorf("upload ID is required")
	}
	if len(input.Parts) == 0 {
		return CompleteMultipartUploadOutput{}, fmt.Errorf("parts list cannot be empty")
	}

	if err := s3.renewIAMToken(); err != nil {
		return CompleteMultipartUploadOutput{}, err
	}

	// Build URL with query parameter
	urlStr := s3.getURL(input.Bucket, input.ObjectKey) + "?uploadId=" + url.QueryEscape(input.UploadID)

	// Build XML request body
	parts := make([]completePart, len(input.Parts))
	for i, p := range input.Parts {
		parts[i] = completePart{
			PartNumber: p.PartNumber,
			ETag:       p.ETag,
		}
	}

	completeReq := completeMultipartUploadRequest{
		XMLNS: "http://s3.amazonaws.com/doc/2006-03-01/",
		Parts: parts,
	}

	xmlBody, err := xml.Marshal(completeReq)
	if err != nil {
		return CompleteMultipartUploadOutput{}, err
	}

	req, err := http.NewRequest(http.MethodPost, urlStr, bytes.NewReader(xmlBody))
	if err != nil {
		return CompleteMultipartUploadOutput{}, err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/xml")
	req.ContentLength = int64(len(xmlBody))

	h := sha256.New()
	h.Write(xmlBody)
	req.Header.Set("x-amz-content-sha256", fmt.Sprintf("%x", h.Sum(nil)))

	// Calculate Content-MD5
	md5Hash := md5.Sum(xmlBody)
	contentMD5 := base64.StdEncoding.EncodeToString(md5Hash[:])
	req.Header.Set("Content-MD5", contentMD5)

	if err := s3.signRequest(req); err != nil {
		return CompleteMultipartUploadOutput{}, err
	}

	client := s3.getClient()
	res, err := client.Do(req)
	if err != nil {
		return CompleteMultipartUploadOutput{}, err
	}
	defer func() {
		res.Body.Close()
		io.Copy(io.Discard, res.Body)
	}()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return CompleteMultipartUploadOutput{}, err
	}

	if res.StatusCode != http.StatusOK {
		return CompleteMultipartUploadOutput{}, fmt.Errorf("status code: %s: %s", res.Status, string(body))
	}

	var result completeMultipartUploadResult
	if err := xml.Unmarshal(body, &result); err != nil {
		return CompleteMultipartUploadOutput{}, err
	}

	return CompleteMultipartUploadOutput{
		Location: result.Location,
		Bucket:   result.Bucket,
		Key:      result.Key,
		ETag:     result.ETag,
	}, nil
}

// AbortMultipartUploadInput contains parameters for aborting a multipart upload
type AbortMultipartUploadInput struct {
	Bucket    string // Required: bucket name
	ObjectKey string // Required: object key
	UploadID  string // Required: upload ID
}

// AbortMultipartUpload aborts a multipart upload and cleans up parts
func (s3 *S3) AbortMultipartUpload(input AbortMultipartUploadInput) error {
	if input.Bucket == "" {
		return fmt.Errorf("bucket name is required")
	}
	if input.ObjectKey == "" {
		return fmt.Errorf("object key is required")
	}
	if input.UploadID == "" {
		return fmt.Errorf("upload ID is required")
	}

	if err := s3.renewIAMToken(); err != nil {
		return err
	}

	// Build URL with query parameter
	urlStr := s3.getURL(input.Bucket, input.ObjectKey) + "?uploadId=" + url.QueryEscape(input.UploadID)

	req, err := http.NewRequest(http.MethodDelete, urlStr, nil)
	if err != nil {
		return err
	}

	// Empty body hash
	req.Header.Set("x-amz-content-sha256", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")

	if err := s3.signRequest(req); err != nil {
		return err
	}

	client := s3.getClient()
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		res.Body.Close()
		io.Copy(io.Discard, res.Body)
	}()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	if res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("status code: %s: %s", res.Status, string(body))
	}

	return nil
}

// ListPartsInput contains parameters for listing parts
type ListPartsInput struct {
	Bucket           string // Required: bucket name
	ObjectKey        string // Required: object key
	UploadID         string // Required: upload ID
	MaxParts         int    // Optional: default 1000, max 1000
	PartNumberMarker int    // Optional: for pagination
}

// Part represents an uploaded part
type Part struct {
	PartNumber   int
	ETag         string
	Size         int64
	LastModified time.Time
}

// ListPartsOutput contains the response from listing parts
type ListPartsOutput struct {
	Bucket               string
	Key                  string
	UploadID             string
	Parts                []Part
	IsTruncated          bool
	NextPartNumberMarker int
	MaxParts             int
}

// listPartsResult is the XML response structure
type listPartsResult struct {
	XMLName              xml.Name   `xml:"ListPartsResult"`
	Bucket               string     `xml:"Bucket"`
	Key                  string     `xml:"Key"`
	UploadID             string     `xml:"UploadId"`
	Parts                []partInfo `xml:"Part"`
	IsTruncated          bool       `xml:"IsTruncated"`
	NextPartNumberMarker int        `xml:"NextPartNumberMarker"`
	MaxParts             int        `xml:"MaxParts"`
}

type partInfo struct {
	PartNumber   int       `xml:"PartNumber"`
	ETag         string    `xml:"ETag"`
	Size         int64     `xml:"Size"`
	LastModified time.Time `xml:"LastModified"`
}

// ListParts lists the parts that have been uploaded for a multipart upload
func (s3 *S3) ListParts(input ListPartsInput) (ListPartsOutput, error) {
	if input.Bucket == "" {
		return ListPartsOutput{}, fmt.Errorf("bucket name is required")
	}
	if input.ObjectKey == "" {
		return ListPartsOutput{}, fmt.Errorf("object key is required")
	}
	if input.UploadID == "" {
		return ListPartsOutput{}, fmt.Errorf("upload ID is required")
	}

	if err := s3.renewIAMToken(); err != nil {
		return ListPartsOutput{}, err
	}

	// Build URL with query parameters
	urlStr := s3.getURL(input.Bucket, input.ObjectKey)
	params := url.Values{}
	params.Set("uploadId", input.UploadID)
	if input.MaxParts > 0 {
		params.Set("max-parts", strconv.Itoa(input.MaxParts))
	}
	if input.PartNumberMarker > 0 {
		params.Set("part-number-marker", strconv.Itoa(input.PartNumberMarker))
	}
	urlStr += "?" + params.Encode()

	req, err := http.NewRequest(http.MethodGet, urlStr, nil)
	if err != nil {
		return ListPartsOutput{}, err
	}

	// Empty body hash
	req.Header.Set("x-amz-content-sha256", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")

	if err := s3.signRequest(req); err != nil {
		return ListPartsOutput{}, err
	}

	client := s3.getClient()
	res, err := client.Do(req)
	if err != nil {
		return ListPartsOutput{}, err
	}
	defer func() {
		res.Body.Close()
		io.Copy(io.Discard, res.Body)
	}()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return ListPartsOutput{}, err
	}

	if res.StatusCode != http.StatusOK {
		return ListPartsOutput{}, fmt.Errorf("status code: %s: %s", res.Status, string(body))
	}

	var result listPartsResult
	if err := xml.Unmarshal(body, &result); err != nil {
		return ListPartsOutput{}, err
	}

	parts := make([]Part, len(result.Parts))
	for i, p := range result.Parts {
		parts[i] = Part{
			PartNumber:   p.PartNumber,
			ETag:         p.ETag,
			Size:         p.Size,
			LastModified: p.LastModified,
		}
	}

	return ListPartsOutput{
		Bucket:               result.Bucket,
		Key:                  result.Key,
		UploadID:             result.UploadID,
		Parts:                parts,
		IsTruncated:          result.IsTruncated,
		NextPartNumberMarker: result.NextPartNumberMarker,
		MaxParts:             result.MaxParts,
	}, nil
}

// ProgressFunc is called during upload with progress information
type ProgressFunc func(info ProgressInfo)

// ProgressInfo contains progress information for a multipart upload
type ProgressInfo struct {
	TotalBytes     int64 // Total bytes to upload (if known)
	UploadedBytes  int64 // Bytes uploaded so far
	CurrentPart    int   // Current part number
	TotalParts     int   // Total parts (if known)
	BytesPerSecond int64 // Current upload speed
}

// MultipartUploadInput contains parameters for the high-level multipart upload
type MultipartUploadInput struct {
	Bucket         string            // Required: bucket name
	ObjectKey      string            // Required: object key
	Body           io.Reader         // Required: file/data to upload
	ContentType    string            // Optional: content type
	CustomMetadata map[string]string // Optional: x-amz-meta-* headers
	ACL            string            // Optional: x-amz-acl
	PartSize       int64             // Optional: default 5MB, min 5MB
	MaxRetries     int               // Optional: default 3
	Concurrency    int               // Optional: default 1 (sequential)
	OnProgress     ProgressFunc      // Optional: progress callback
}

// MultipartUploadOutput contains the response from a multipart upload
type MultipartUploadOutput struct {
	Location string
	Bucket   string
	Key      string
	ETag     string
	UploadID string
}

// FileUploadMultipart handles the entire multipart upload workflow
func (s3 *S3) FileUploadMultipart(input MultipartUploadInput) (MultipartUploadOutput, error) {
	if input.Bucket == "" {
		return MultipartUploadOutput{}, fmt.Errorf("bucket name is required")
	}
	if input.ObjectKey == "" {
		return MultipartUploadOutput{}, fmt.Errorf("object key is required")
	}
	if input.Body == nil {
		return MultipartUploadOutput{}, fmt.Errorf("body is required")
	}

	// Set defaults
	partSize := input.PartSize
	if partSize == 0 {
		partSize = DefaultPartSize
	}
	if partSize < MinPartSize {
		return MultipartUploadOutput{}, fmt.Errorf("part size must be at least %d bytes", MinPartSize)
	}

	maxRetries := input.MaxRetries
	if maxRetries == 0 {
		maxRetries = DefaultMaxRetries
	}

	concurrency := input.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}

	// Initiate multipart upload
	initOutput, err := s3.InitiateMultipartUpload(InitiateMultipartUploadInput{
		Bucket:         input.Bucket,
		ObjectKey:      input.ObjectKey,
		ContentType:    input.ContentType,
		CustomMetadata: input.CustomMetadata,
		ACL:            input.ACL,
	})
	if err != nil {
		return MultipartUploadOutput{}, err
	}

	// Read entire body to determine size and split into parts
	bodyData, err := io.ReadAll(input.Body)
	if err != nil {
		s3.AbortMultipartUpload(AbortMultipartUploadInput{
			Bucket:    input.Bucket,
			ObjectKey: input.ObjectKey,
			UploadID:  initOutput.UploadID,
		})
		return MultipartUploadOutput{}, err
	}

	totalSize := int64(len(bodyData))
	totalParts := int(math.Ceil(float64(totalSize) / float64(partSize)))

	if totalParts > MaxParts {
		s3.AbortMultipartUpload(AbortMultipartUploadInput{
			Bucket:    input.Bucket,
			ObjectKey: input.ObjectKey,
			UploadID:  initOutput.UploadID,
		})
		return MultipartUploadOutput{}, fmt.Errorf("file too large: requires %d parts, maximum is %d", totalParts, MaxParts)
	}

	startTime := time.Now()
	var uploadedBytes int64

	// Upload parts
	var completedParts []CompletedPart
	var err2 error

	if concurrency <= 1 {
		// Sequential upload
		completedParts, err2 = s3.uploadPartsSequential(bodyData, partSize, totalParts, initOutput.UploadID, input.Bucket, input.ObjectKey, maxRetries, input.OnProgress, &uploadedBytes, totalSize, startTime)
	} else {
		// Parallel upload
		completedParts, err2 = s3.uploadPartsParallel2(bodyData, partSize, totalParts, initOutput.UploadID, input.Bucket, input.ObjectKey, maxRetries, concurrency, input.OnProgress, &uploadedBytes, totalSize, startTime)
	}

	if err2 != nil {
		s3.AbortMultipartUpload(AbortMultipartUploadInput{
			Bucket:    input.Bucket,
			ObjectKey: input.ObjectKey,
			UploadID:  initOutput.UploadID,
		})
		return MultipartUploadOutput{}, err2
	}

	// Complete multipart upload
	completeOutput, err := s3.CompleteMultipartUpload(CompleteMultipartUploadInput{
		Bucket:    input.Bucket,
		ObjectKey: input.ObjectKey,
		UploadID:  initOutput.UploadID,
		Parts:     completedParts,
	})
	if err != nil {
		s3.AbortMultipartUpload(AbortMultipartUploadInput{
			Bucket:    input.Bucket,
			ObjectKey: input.ObjectKey,
			UploadID:  initOutput.UploadID,
		})
		return MultipartUploadOutput{}, err
	}

	return MultipartUploadOutput{
		Location: completeOutput.Location,
		Bucket:   completeOutput.Bucket,
		Key:      completeOutput.Key,
		ETag:     completeOutput.ETag,
		UploadID: initOutput.UploadID,
	}, nil
}

// uploadPartsSequential uploads parts sequentially
func (s3 *S3) uploadPartsSequential(bodyData []byte, partSize int64, totalParts int, uploadID, bucket, objectKey string, maxRetries int, onProgress ProgressFunc, uploadedBytes *int64, totalSize int64, startTime time.Time) ([]CompletedPart, error) {
	completedParts := make([]CompletedPart, 0, totalParts)

	for partNum := 1; partNum <= totalParts; partNum++ {
		start := int64(partNum-1) * partSize
		end := start + partSize
		if end > totalSize {
			end = totalSize
		}

		partData := bodyData[start:end]

		output, err := s3.uploadPartWithRetry(UploadPartInput{
			Bucket:     bucket,
			ObjectKey:  objectKey,
			UploadID:   uploadID,
			PartNumber: partNum,
			Body:       bytes.NewReader(partData),
			Size:       int64(len(partData)),
		}, maxRetries)

		if err != nil {
			return nil, err
		}

		completedParts = append(completedParts, CompletedPart{
			PartNumber: output.PartNumber,
			ETag:       output.ETag,
		})

		*uploadedBytes += int64(len(partData))

		// Call progress callback
		if onProgress != nil {
			elapsed := time.Since(startTime).Seconds()
			bytesPerSecond := int64(0)
			if elapsed > 0 {
				bytesPerSecond = int64(float64(*uploadedBytes) / elapsed)
			}

			onProgress(ProgressInfo{
				TotalBytes:     totalSize,
				UploadedBytes:  *uploadedBytes,
				CurrentPart:    partNum,
				TotalParts:     totalParts,
				BytesPerSecond: bytesPerSecond,
			})
		}
	}

	return completedParts, nil
}

// partData represents a part to be uploaded
type partData struct {
	partNumber int
	data       []byte
}

// uploadPartsParallel2 uploads parts in parallel using a worker pool
func (s3 *S3) uploadPartsParallel2(bodyData []byte, partSize int64, totalParts int, uploadID, bucket, objectKey string, maxRetries, concurrency int, onProgress ProgressFunc, uploadedBytes *int64, totalSize int64, startTime time.Time) ([]CompletedPart, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create channels
	partsChan := make(chan partData, concurrency)
	resultsChan := make(chan CompletedPart, totalParts)
	errChan := make(chan error, concurrency)

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case part, ok := <-partsChan:
					if !ok {
						return
					}

					output, err := s3.uploadPartWithRetry(UploadPartInput{
						Bucket:     bucket,
						ObjectKey:  objectKey,
						UploadID:   uploadID,
						PartNumber: part.partNumber,
						Body:       bytes.NewReader(part.data),
						Size:       int64(len(part.data)),
					}, maxRetries)

					if err != nil {
						select {
						case errChan <- err:
							cancel()
						default:
						}
						return
					}

					resultsChan <- CompletedPart{
						PartNumber: output.PartNumber,
						ETag:       output.ETag,
					}
				}
			}
		}()
	}

	// Send parts to workers
	go func() {
		for partNum := 1; partNum <= totalParts; partNum++ {
			start := int64(partNum-1) * partSize
			end := start + partSize
			if end > totalSize {
				end = totalSize
			}

			select {
			case <-ctx.Done():
				return
			case partsChan <- partData{
				partNumber: partNum,
				data:       bodyData[start:end],
			}:
			}
		}
		close(partsChan)
	}()

	// Collect results
	go func() {
		wg.Wait()
		close(resultsChan)
		close(errChan)
	}()

	completedParts := make([]CompletedPart, 0, totalParts)
	partsCompleted := 0

	for {
		select {
		case err := <-errChan:
			if err != nil {
				cancel()
				return nil, err
			}
		case part, ok := <-resultsChan:
			if !ok {
				// Sort by part number
				for i := 0; i < len(completedParts); i++ {
					for j := i + 1; j < len(completedParts); j++ {
						if completedParts[i].PartNumber > completedParts[j].PartNumber {
							completedParts[i], completedParts[j] = completedParts[j], completedParts[i]
						}
					}
				}
				return completedParts, nil
			}

			completedParts = append(completedParts, part)
			partsCompleted++
			*uploadedBytes = int64(partsCompleted) * partSize
			if *uploadedBytes > totalSize {
				*uploadedBytes = totalSize
			}

			// Call progress callback
			if onProgress != nil {
				elapsed := time.Since(startTime).Seconds()
				bytesPerSecond := int64(0)
				if elapsed > 0 {
					bytesPerSecond = int64(float64(*uploadedBytes) / elapsed)
				}

				onProgress(ProgressInfo{
					TotalBytes:     totalSize,
					UploadedBytes:  *uploadedBytes,
					CurrentPart:    partsCompleted,
					TotalParts:     totalParts,
					BytesPerSecond: bytesPerSecond,
				})
			}
		}
	}
}
