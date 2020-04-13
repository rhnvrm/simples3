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
	defaultProtocol      = "https://"         // <bucket>
)

// PresignedInput is passed to GeneratePresignedURL as a parameter.
type PresignedInput struct {
	Bucket        string
	ObjectKey     string
	Method        string
	Timestamp     time.Time
	ExtraHeaders  map[string]string
	ExpirySeconds int
	Protocol      string
	Endpoint      string
}

// GeneratePresignedURL creates a Presigned URL that can be used
// for Authentication using Query Parameters.
// (https://docs.aws.amazon.com/AmazonS3/latest/API/sigv4-query-string-auth.html)
func (s3 *S3) GeneratePresignedURL(in PresignedInput) string {
	var (
		nowTime = nowTime()

		protocol = defaultProtocol
		endpoint = defaultPresignedHost
	)
	if !in.Timestamp.IsZero() {
		nowTime = in.Timestamp.UTC()
	}
	amzdate := nowTime.Format(amzDateISO8601TimeFormat)

	// Create cred
	b := bytes.Buffer{}
	b.WriteString(s3.AccessKey)
	b.WriteRune('/')
	b.Write(s3.buildCredentialWithoutKey(nowTime))
	cred := b.Bytes()
	b.Reset()

	// Set the protocol as default if not provided.
	if in.Protocol != "" {
		protocol = in.Protocol
	}
	if in.Endpoint != "" {
		endpoint = in.Endpoint
	}

	// Add host to Headers
	signedHeaders := map[string][]byte{}
	for k, v := range in.ExtraHeaders {
		signedHeaders[k] = []byte(v)
	}
	host := bytes.Buffer{}
	host.WriteString(in.Bucket)
	host.WriteRune('.')
	host.WriteString(endpoint)
	signedHeaders["host"] = host.Bytes()

	// Start Canonical Request Formation
	h := sha256.New()          // We write the canonical request directly to the SHA256 hash.
	h.Write([]byte(in.Method)) // HTTP Verb
	h.Write(newLine)
	h.Write([]byte{'/'})
	h.Write([]byte(in.ObjectKey)) // CanonicalURL
	h.Write(newLine)

	// Start QueryString Params (before SignedHeaders)
	queryString := map[string]string{
		"X-Amz-Algorithm":  algorithm,
		"X-Amz-Credential": string(cred),
		"X-Amz-Date":       amzdate,
		"X-Amz-Expires":    strconv.Itoa(in.ExpirySeconds),
	}
	//  include the x-amz-security-token incase we are using IAM role or AWS STS
	if s3.Token != "" {
		queryString["X-Amz-Security-Token"] = s3.Token
	}

	// We need to have a sorted order,
	// for QueryStrings and SignedHeaders
	sortedQS := make([]string, 0, len(queryString))
	for name := range queryString {
		sortedQS = append(sortedQS, name)
	}
	sort.Strings(sortedQS) //sort by key

	sortedSH := make([]string, 0, len(signedHeaders))
	for name := range signedHeaders {
		sortedSH = append(sortedSH, name)
	}
	sort.Strings(sortedSH) //sort by key

	// Proceed to write canonical query params
	for _, k := range sortedQS {
		// HTTP Verb
		h.Write([]byte(url.QueryEscape(k)))
		h.Write([]byte{'='})
		h.Write([]byte(url.QueryEscape(string(queryString[k]))))
		h.Write([]byte{'&'})
	}

	h.Write([]byte("X-Amz-SignedHeaders="))
	// Add Signed Headers to Query String
	first := true
	for i := 0; i < len(sortedSH); i++ {
		if first {
			h.Write([]byte(url.QueryEscape(sortedSH[i])))
			first = false
		} else {
			h.Write([]byte{';'})
			h.Write([]byte(url.QueryEscape(sortedSH[i])))
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
	first = true
	for i := 0; i < len(sortedSH); i++ {
		if first {
			h.Write([]byte(url.QueryEscape(sortedSH[i])))
			first = false
		} else {
			h.Write([]byte{';'})
			h.Write([]byte(url.QueryEscape(sortedSH[i])))
		}
	}
	h.Write(newLine)
	// End Canonical Headers

	// Mention Unsigned Payload
	h.Write([]byte("UNSIGNED-PAYLOAD"))

	// canonicalReq := h.Bytes()
	// Start StringToSign
	b.WriteString(algorithm)
	b.WriteRune('\n')
	b.WriteString(amzdate)
	b.WriteRune('\n')
	b.Write(s3.buildCredentialWithoutKey(nowTime))
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
				[]byte(nowTime.UTC().Format(shortTimeFormat))),
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
	b.WriteString(protocol)
	b.WriteString(in.Bucket)
	b.WriteRune('.')
	b.WriteString(endpoint)
	b.WriteRune('/')
	b.WriteString(in.ObjectKey)
	b.WriteRune('?')

	// We don't need to have a sorted order here,
	// but just to preserve tests.
	for i := 0; i < len(sortedQS); i++ {
		b.WriteString(url.QueryEscape(sortedQS[i]))
		b.WriteRune('=')
		b.WriteString(url.QueryEscape(string(queryString[sortedQS[i]])))
		b.WriteRune('&')
	}
	b.WriteString("X-Amz-SignedHeaders")
	b.WriteRune('=')
	first = true
	for i := 0; i < len(sortedSH); i++ {
		if first {
			b.WriteString(url.QueryEscape(sortedSH[i]))
			first = false
		} else {
			b.WriteRune(';')
			b.WriteString(url.QueryEscape(sortedSH[i]))
		}
	}
	b.WriteString("&X-Amz-Signature=")
	b.WriteString(signature)

	return b.String()
}
