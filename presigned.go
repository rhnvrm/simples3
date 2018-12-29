package simples3

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultPresignedURLFormat = "%s.s3.amazonaws.com" // <bucket>
	defaultProtocol           = "https://"            // <bucket>
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
}

// GeneratePresignedURL creates a Presigned URL that can be used
// for Authentication using Query Parameters.
// (https://docs.aws.amazon.com/AmazonS3/latest/API/sigv4-query-string-auth.html)
func (s3 *S3) GeneratePresignedURL(in PresignedInput) string {
	var (
		nowTime  = in.Timestamp.UTC()
		cred     = fmt.Sprintf("%s/"+s3.buildCredentialWithoutKey(nowTime), s3.AccessKey)
		amzdate  = nowTime.Format(amzDateISO8601TimeFormat)
		expiry   = strconv.Itoa(in.ExpirySeconds)
		protocol = in.Protocol
	)

	// Set the protocol as default if not provided.
	if protocol == "" {
		protocol = defaultProtocol
	}

	// Add host to Headers
	signedHeaders := in.ExtraHeaders
	if signedHeaders == nil {
		signedHeaders = map[string]string{}
	}
	signedHeaders["host"] = fmt.Sprintf(defaultPresignedURLFormat, in.Bucket)

	// Start Canonical Request Formation
	h := sha256.New()                     // We write the canonical request directly to the SHA256 hash.
	fmt.Fprintf(h, "%s\n", in.Method)     // HTTP Verb
	fmt.Fprintf(h, "/%s\n", in.ObjectKey) // CanonicalURL

	// Start QueryString Params (before SignedHeaders)
	queryString := map[string]string{
		"X-Amz-Algorithm":  algorithm,
		"X-Amz-Credential": cred,
		"X-Amz-Date":       amzdate,
		"X-Amz-Expires":    expiry,
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
		fmt.Fprintf(h, "%s=%s&", url.QueryEscape(k), url.QueryEscape(queryString[k])) // HTTP Verb
	}

	fmt.Fprintf(h, "X-Amz-SignedHeaders=")
	// Add Signed Headers to Query String
	first := true
	for k := range sortedSH {
		if first {
			fmt.Fprintf(h, "%s", url.QueryEscape(sortedSH[k]))
			first = false
		} else {
			fmt.Fprintf(h, "%s", url.QueryEscape(";"+sortedSH[k]))
		}
	}
	fmt.Fprintf(h, "\n")
	// End QueryString Params

	// Start Canonical Headers
	for _, k := range sortedSH {
		fmt.Fprintf(h, "%s:%s\n", strings.ToLower(k), strings.TrimSpace(signedHeaders[k]))
	}
	fmt.Fprintf(h, "\n")
	// End Canonical Headers

	// Start Signed Headers
	first = true
	for k := range sortedSH {
		if first {
			fmt.Fprintf(h, "%s", url.QueryEscape(sortedSH[k]))
			first = false
		} else {
			fmt.Fprintf(h, "%s", url.QueryEscape(";"+sortedSH[k]))
		}
	}
	fmt.Fprintf(h, "\n")
	// End Canonical Headers

	// Mention Unsigned Payload
	fmt.Fprintf(h, "UNSIGNED-PAYLOAD")

	// canonicalReq := h.Bytes()
	// Reset Buffer to create StringToSign
	var b bytes.Buffer

	// Start StringToSign
	fmt.Fprintf(&b, "%s\n", algorithm)                             // Algo
	fmt.Fprintf(&b, "%s\n", amzdate)                               // Date
	fmt.Fprintf(&b, "%s\n", s3.buildCredentialWithoutKey(nowTime)) //Cred

	hashed := hex.EncodeToString(h.Sum(nil))
	fmt.Fprintf(&b, "%s", hashed)

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

	// Reset Buffer to create URl
	b.Reset()

	// Start Generating URL
	fmt.Fprintf(&b, protocol+defaultPresignedURLFormat+"/", in.Bucket)
	fmt.Fprintf(&b, "%s?", in.ObjectKey)

	// We don't need to have a sorted order here,
	// but just to preserve tests.
	for _, k := range sortedQS {
		fmt.Fprintf(&b, "%s=%s&", url.QueryEscape(k), url.QueryEscape(queryString[k])) // HTTP Verb
	}
	fmt.Fprintf(&b, "%s=", "X-Amz-SignedHeaders")
	first = true
	for k := range sortedSH {
		if first {
			fmt.Fprintf(&b, "%s", url.QueryEscape(sortedSH[k]))
			first = false
		} else {
			fmt.Fprintf(&b, "%s", url.QueryEscape(";"+sortedSH[k]))
		}
	}
	fmt.Fprintf(&b, "&X-Amz-Signature=%s", signature)

	return b.String()
}
