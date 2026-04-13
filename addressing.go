package simples3

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

type addressingMode uint8

const (
	addressingModeLegacy addressingMode = iota
	addressingModePath
	addressingModeVirtual
)

type addressingSurface uint8

const (
	addressingSurfaceRuntime addressingSurface = iota
	addressingSurfacePresign
	addressingSurfacePolicy
)

type addressStyle uint8

const (
	addressStylePath addressStyle = iota
	addressStyleVirtual
)

type resolvedAddress struct {
	scheme         string
	host           string
	path           string
	style          addressStyle
	fallbackReason string
}

func (a resolvedAddress) urlString() string {
	path := a.path
	if path == "" {
		path = "/"
	}
	return a.scheme + "://" + a.host + path
}

func (s3 *S3) resolveAddress(surface addressingSurface, bucket string, args ...string) resolvedAddress {
	switch s3.addressingMode {
	case addressingModePath:
		return s3.resolvePathAddress(bucket, args...)
	case addressingModeVirtual:
		return s3.resolveExplicitVirtualAddress(bucket, args...)
	default:
		return s3.resolveLegacyAddress(surface, bucket, args...)
	}
}

func (s3 *S3) resolveLegacyAddress(surface addressingSurface, bucket string, args ...string) resolvedAddress {
	switch surface {
	case addressingSurfacePresign:
		if endpoint, _ := url.Parse(s3.Endpoint); endpoint.Host != "" {
			return s3.resolvePathAddress(bucket, args...)
		}
		return resolvedAddress{
			scheme: "https",
			host:   bucket + "." + defaultPresignedHost,
			path:   buildVirtualObjectPath("", args...),
			style:  addressStyleVirtual,
		}
	case addressingSurfacePolicy:
		return parseResolvedAddress(fmt.Sprintf(defaultUploadURLFormat, bucket), addressStyleVirtual, "")
	default:
		return s3.resolvePathAddress(bucket, args...)
	}
}

func (s3 *S3) resolvePathAddress(bucket string, args ...string) resolvedAddress {
	return parseResolvedAddress(s3.legacyPathURL(bucket, args...), addressStylePath, "")
}

func (s3 *S3) resolveExplicitVirtualAddress(bucket string, args ...string) resolvedAddress {
	base, err := s3.serviceBaseURL()
	if err != nil {
		return parseResolvedAddress(s3.legacyPathURL(bucket, args...), addressStylePath, "invalid service endpoint")
	}

	if reason := virtualAddressingFallbackReason(base, bucket); reason != "" {
		return parseResolvedAddress(s3.legacyPathURL(bucket, args...), addressStylePath, reason)
	}

	return resolvedAddress{
		scheme: base.Scheme,
		host:   bucketHost(base, bucket),
		path:   buildVirtualObjectPath(base.EscapedPath(), args...),
		style:  addressStyleVirtual,
	}
}

func (s3 *S3) legacyPathURL(bucket string, args ...string) string {
	path := bucket
	if len(args) > 0 {
		path += "/" + strings.Join(args, "/")
	}
	encodedPath := encodePath(path)

	if len(s3.Endpoint) > 0 {
		return s3.Endpoint + "/" + encodedPath
	}
	return fmt.Sprintf(s3.URIFormat, s3.Region, encodedPath)
}

func (s3 *S3) serviceBaseURL() (*url.URL, error) {
	rawURL := s3.Endpoint
	if rawURL == "" {
		rawURL = fmt.Sprintf(s3.URIFormat, s3.Region, "")
	}
	return url.Parse(rawURL)
}

func parseResolvedAddress(rawURL string, style addressStyle, fallbackReason string) resolvedAddress {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return resolvedAddress{style: style, fallbackReason: fallbackReason}
	}

	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	}

	return resolvedAddress{
		scheme:         parsed.Scheme,
		host:           parsed.Host,
		path:           path,
		style:          style,
		fallbackReason: fallbackReason,
	}
}

func buildVirtualObjectPath(basePath string, args ...string) string {
	trimmedBase := strings.TrimRight(basePath, "/")
	objectPath := strings.Join(args, "/")
	if objectPath == "" {
		if trimmedBase == "" {
			return "/"
		}
		return trimmedBase + "/"
	}

	encodedObjectPath := encodePath(objectPath)
	if trimmedBase == "" {
		return "/" + encodedObjectPath
	}
	return trimmedBase + "/" + encodedObjectPath
}

func bucketHost(base *url.URL, bucket string) string {
	hostname := base.Hostname()
	if port := base.Port(); port != "" {
		return bucket + "." + hostname + ":" + port
	}
	return bucket + "." + hostname
}

func virtualAddressingFallbackReason(base *url.URL, bucket string) string {
	if !dnsCompatibleBucketName(bucket) {
		return "bucket is not DNS compatible"
	}
	if base.Scheme == "https" && strings.Contains(bucket, ".") {
		return "dotted bucket over https"
	}
	if basePath := strings.Trim(base.EscapedPath(), "/"); basePath != "" {
		return "endpoint has path prefix"
	}
	if hostname := base.Hostname(); hostname == "localhost" {
		return "localhost endpoint"
	} else if hostname != "" {
		if ip := net.ParseIP(hostname); ip != nil {
			return "ip endpoint"
		}
	}
	return ""
}

func dnsCompatibleBucketName(bucket string) bool {
	if len(bucket) < 3 || len(bucket) > 63 {
		return false
	}
	if strings.Contains(bucket, "..") {
		return false
	}
	if !isLowercaseLetterOrDigit(bucket[0]) || !isLowercaseLetterOrDigit(bucket[len(bucket)-1]) {
		return false
	}
	for i := 1; i < len(bucket)-1; i++ {
		c := bucket[i]
		if !isLowercaseLetterOrDigit(c) && c != '.' && c != '-' {
			return false
		}
	}

	parts := strings.Split(bucket, ".")
	for _, part := range parts {
		if part == "" {
			return false
		}
		if !isLowercaseLetterOrDigit(part[0]) || !isLowercaseLetterOrDigit(part[len(part)-1]) {
			return false
		}
	}

	if len(parts) == 4 {
		isIPAddress := true
		for _, part := range parts {
			for i := 0; i < len(part); i++ {
				if part[i] < '0' || part[i] > '9' {
					isIPAddress = false
					break
				}
			}
			if !isIPAddress {
				break
			}
		}
		if isIPAddress {
			return false
		}
	}

	return true
}

func isLowercaseLetterOrDigit(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
}
