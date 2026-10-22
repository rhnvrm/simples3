package simples3

import (
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const sseCustomerPrefix = "x-amz-server-side-encryption-customer-"

// SSECustomerHeaders returns the headers needed for SSE-C using a raw 32-byte
// AES-256 key. It is useful with PresignedInput.ExtraHeaders and
// PresignedMultipartInput.ExtraHeaders. Send these same headers with the signed
// request over HTTPS. The returned map contains the key: do not log it or put it
// in a URL. Key management remains the caller's responsibility.
func SSECustomerHeaders(key []byte) (map[string]string, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("SSE-C key must contain exactly 32 bytes")
	}
	digest := md5.Sum(key)
	return map[string]string{
		sseCustomerPrefix + "algorithm": "AES256",
		sseCustomerPrefix + "key":       base64.StdEncoding.EncodeToString(key),
		sseCustomerPrefix + "key-MD5":   base64.StdEncoding.EncodeToString(digest[:]),
	}, nil
}

func validateSSECustomerKey(key []byte, encryption, kmsKey string) error {
	if len(key) == 0 {
		return nil
	}
	if len(key) != 32 {
		return fmt.Errorf("SSE-C key must contain exactly 32 bytes")
	}
	if encryption != "" || kmsKey != "" {
		return fmt.Errorf("SSE-C cannot be combined with SSE-S3 or SSE-KMS")
	}
	return nil
}

func requireSSEHTTPS(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("SSE-C requires an HTTPS endpoint")
	}
	return nil
}

func setSSECustomerHeaders(req *http.Request, key []byte, source bool) error {
	if len(key) == 0 {
		return nil
	}
	if err := requireSSEHTTPS(req.URL.String()); err != nil {
		return err
	}
	headers, err := SSECustomerHeaders(key)
	if err != nil {
		return err
	}
	for k, v := range headers {
		if source {
			k = strings.Replace(k, "x-amz-", "x-amz-copy-source-", 1)
		}
		req.Header.Set(k, v)
	}
	return nil
}

func hasSSECustomerHeaders(headers map[string]string) bool {
	for k := range headers {
		k = strings.ToLower(k)
		if strings.HasPrefix(k, sseCustomerPrefix) || strings.HasPrefix(k, "x-amz-copy-source-server-side-encryption-customer-") {
			return true
		}
	}
	return false
}

// SSE-C headers are not among the sensitive headers net/http strips on
// cross-host redirects. Keep keys on the original HTTPS origin, including POST
// form bodies, and preserve any caller-supplied redirect policy.
func sseCustomerClient(client *http.Client) *http.Client {
	copyClient := *client
	copyClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := requireSSEHTTPS(req.URL.String()); err != nil {
			return err
		}
		if len(via) > 0 && req.URL.Host != via[0].URL.Host {
			return fmt.Errorf("SSE-C redirect must stay on the original HTTPS origin")
		}
		if client.CheckRedirect != nil {
			return client.CheckRedirect(req, via)
		}
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		return nil
	}
	return &copyClient
}

func (s3 *S3) doSSERequest(req *http.Request) (*http.Response, error) {
	client := s3.getClient()
	if req.Header.Get(sseCustomerPrefix+"key") != "" || req.Header.Get("x-amz-copy-source-server-side-encryption-customer-key") != "" {
		client = sseCustomerClient(client)
	}
	return client.Do(req)
}
