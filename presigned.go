package simples3

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultPresignedHost = "s3.amazonaws.com" // <bucket>

	HdrXAmzSignedHeaders = "X-Amz-SignedHeaders"
)

// PresignedInput is passed to GeneratePresignedURL as a parameter.
type PresignedInput struct {
	Bucket                     string
	ObjectKey                  string
	Method                     string
	Timestamp                  time.Time
	ExtraHeaders               map[string]string
	ExpirySeconds              int
	ResponseContentDisposition string
}

// awsURIEncode encodes a string per AWS S3 requirements (space as %20, not +, as Go's url.QueryEscape does)
// (https://docs.aws.amazon.com/AmazonS3/latest/API/API_GetObject.html)
func awsURIEncode(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

// GeneratePresignedURL creates a Presigned URL that can be used
// for Authentication using Query Parameters.
// (https://docs.aws.amazon.com/AmazonS3/latest/API/sigv4-query-string-auth.html)
func (s3 *S3) GeneratePresignedURL(in PresignedInput) string {
	extraQuery := map[string]string{}
	if in.ResponseContentDisposition != "" {
		extraQuery["response-content-disposition"] = in.ResponseContentDisposition
	}

	return s3.generatePresignedObjectURL(
		in.Method,
		in.Bucket,
		in.ObjectKey,
		in.Timestamp,
		in.ExpirySeconds,
		in.ExtraHeaders,
		extraQuery,
	)
}

func (s3 *S3) generatePresignedObjectURL(method, bucket, objectKey string, timestamp time.Time, expirySeconds int, extraHeaders map[string]string, extraQuery map[string]string) string {
	if err := s3.renewIAMToken(); err != nil {
		return ""
	}

	currentTime := nowTime()
	if !timestamp.IsZero() {
		currentTime = timestamp.UTC()
	}
	amzdate := currentTime.Format(amzDateISO8601TimeFormat)
	address := s3.resolveAddress(addressingSurfacePresign, bucket, objectKey)

	// Create cred
	b := bytes.Buffer{}
	b.WriteString(s3.AccessKey)
	b.WriteRune('/')
	b.Write(s3.buildCredentialWithoutKey(currentTime))
	cred := b.Bytes()
	b.Reset()

	// Add host to Headers
	signedHeaders := map[string][]byte{}
	for k, v := range extraHeaders {
		// AWS requires header names to be lowercase per spec
		signedHeaders[strings.ToLower(k)] = []byte(v)
	}
	signedHeaders["host"] = []byte(address.host)

	// Build signed headers string
	sortedSH := make([]string, 0, len(signedHeaders))
	for name := range signedHeaders {
		sortedSH = append(sortedSH, name)
	}
	sort.Strings(sortedSH)
	signedHeadersStr := strings.Join(sortedSH, ";")

	// For URL: header names must be individually escaped, semicolons remain raw
	// AWS format: "host;x-amz-date" not "host%3Bx-amz-date"
	var signedHeadersForURL strings.Builder
	for i, name := range sortedSH {
		if i > 0 {
			signedHeadersForURL.WriteRune(';')
		}
		signedHeadersForURL.WriteString(url.QueryEscape(name))
	}

	// Start Canonical Request Formation
	h := sha256.New()       // We write the canonical request directly to the SHA256 hash.
	h.Write([]byte(method)) // HTTP Verb
	h.Write(newLine)
	h.Write([]byte(address.path)) // CanonicalURL
	h.Write(newLine)

	// Start QueryString Params (before SignedHeaders)
	queryString := map[string]string{
		"X-Amz-Algorithm":    algorithm,
		"X-Amz-Credential":   string(cred),
		"X-Amz-Date":         amzdate,
		"X-Amz-Expires":      strconv.Itoa(expirySeconds),
		HdrXAmzSignedHeaders: signedHeadersForURL.String(),
	}

	for k, v := range extraQuery {
		queryString[k] = v
	}

	// include the x-amz-security-token incase we are using IAM role or AWS STS
	if s3.Token != "" {
		queryString["X-Amz-Security-Token"] = s3.Token
	}

	// We need to have a sorted order,
	// for QueryStrings and SignedHeaders
	sortedQS := make([]string, 0, len(queryString))
	for name := range queryString {
		sortedQS = append(sortedQS, name)
	}
	sort.Strings(sortedQS)

	// Proceed to write canonical query params
	for i, k := range sortedQS {
		h.Write([]byte(awsURIEncode(k)))
		h.Write([]byte{'='})
		h.Write([]byte(awsURIEncode(queryString[k])))
		if i < len(sortedQS)-1 {
			h.Write([]byte{'&'})
		}
	}
	h.Write(newLine)
	// End QueryString Params

	// Start Canonical Headers
	for i := 0; i < len(sortedSH); i++ {
		h.Write([]byte(strings.ToLower(sortedSH[i])))
		h.Write([]byte{':'})
		h.Write([]byte(strings.TrimSpace(string(signedHeaders[sortedSH[i]]))))
		h.Write(newLine)
	}
	h.Write(newLine)
	// End Canonical Headers

	// Start Signed Headers
	h.Write([]byte(signedHeadersStr))
	h.Write(newLine)
	// End Signed Headers

	// Mention Unsigned Payload
	h.Write([]byte("UNSIGNED-PAYLOAD"))

	// Start StringToSign
	b.WriteString(algorithm)
	b.WriteRune('\n')
	b.WriteString(amzdate)
	b.WriteRune('\n')
	b.Write(s3.buildCredentialWithoutKey(currentTime))
	b.WriteRune('\n')

	hashed := hex.EncodeToString(h.Sum(nil))
	b.WriteString(hashed)

	stringToSign := b.Bytes()

	// End StringToSign

	// Start Signature Key
	sigKey := makeHMac(makeHMac(
		makeHMac(
			makeHMac(
				[]byte("AWS4"+s3.SecretKey),
				[]byte(currentTime.UTC().Format(shortTimeFormat))),
			[]byte(s3.Region)),
		[]byte("s3")),
		[]byte("aws4_request"),
	)
	// sigKey gen verified using
	// https://docs.aws.amazon.com/general/latest/gr/signature-v4-examples.html#signature-v4-examples-other
	// (TODO: add a test using the same, consolidate with signKeys())

	signedStrToSign := makeHMac(sigKey, stringToSign)
	signature := hex.EncodeToString(signedStrToSign)
	// End Signature

	// Reset Buffer to create URL
	b.Reset()

	// Start Generating URL
	b.WriteString(address.urlString())
	b.WriteRune('?')

	for i, k := range sortedQS {
		b.WriteString(awsURIEncode(k))
		b.WriteRune('=')
		b.WriteString(awsURIEncode(queryString[k]))
		if i < len(sortedQS)-1 {
			b.WriteRune('&')
		}
	}
	b.WriteString("&X-Amz-Signature=")
	b.WriteString(signature)

	return b.String()
}

// PresignedMultipartInput contains parameters for generating presigned multipart upload URLs
type PresignedMultipartInput struct {
	Bucket        string // Required: bucket name
	ObjectKey     string // Required: object key
	UploadID      string // Required: upload ID from InitiateMultipartUpload
	PartNumber    int    // Required: part number (1-10000)
	ExpirySeconds int    // Optional: default 3600
}

// GeneratePresignedUploadPartURL generates a presigned URL for uploading a specific part
// This enables browser-based multipart uploads where the frontend uploads parts directly to S3
func (s3 *S3) GeneratePresignedUploadPartURL(in PresignedMultipartInput) string {
	if in.ExpirySeconds == 0 {
		in.ExpirySeconds = 3600
	}

	return s3.generatePresignedObjectURL(
		"PUT",
		in.Bucket,
		in.ObjectKey,
		time.Time{},
		in.ExpirySeconds,
		nil,
		map[string]string{
			"partNumber": strconv.Itoa(in.PartNumber),
			"uploadId":   in.UploadID,
		},
	)
}
