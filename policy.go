// LICENSE BSD-2-Clause-FreeBSD
// Copyright (c) 2018, Rohan Verma <hello@rohanverma.net>
// Copyright (c) 2017, L Campbell
// forked from: https://github.com/lye/s3/
// For previous license information visit
// https://github.com/lye/s3/blob/master/LICENSE

package simples3

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// UploadConfig generate policies from config
// for POST requests to S3 using Signing V4.
type UploadConfig struct {
	// Required
	BucketName         string
	ObjectKey          string
	ContentType        string
	ContentDisposition string
	ACL                string
	FileSize           int64
	// Optional
	UploadURL  string
	Expiration time.Duration
	MetaData   map[string]string
}

// UploadPolicies Amazon s3 upload policies
type UploadPolicies struct {
	URL  string
	Form map[string]string
}

// PolicyJSON is policy rule
type PolicyJSON struct {
	Expiration string        `json:"expiration"`
	Conditions []interface{} `json:"conditions"`
}

const (
	expirationTimeFormat     = "2006-01-02T15:04:05Z07:00"
	amzDateISO8601TimeFormat = "20060102T150405Z"
	shortTimeFormat          = "20060102"
	algorithm                = "AWS4-HMAC-SHA256"
	serviceName              = "s3"

	defaultUploadURLFormat = "http://%s.s3.amazonaws.com/" // <bucketName>
	defaultExpirationHour  = 1 * time.Hour
)

// nowTime mockable time.Now()
var nowTime = func() time.Time {
	return time.Now().UTC()
}

var newLine = []byte{'\n'}

// CreateUploadPolicies creates amazon s3 sigv4 compatible
// policy and signing keys with the signature returns the upload policy.
// https://docs.aws.amazon.com/ja_jp/AmazonS3/latest/API/sigv4-authentication-HTTPPOST.html
func (s3 *S3) CreateUploadPolicies(uploadConfig UploadConfig) (UploadPolicies, error) {
	nowTime := nowTime()
	credential := string(s3.buildCredential(nowTime))
	data, err := buildUploadSign(nowTime, credential, uploadConfig)
	if err != nil {
		return UploadPolicies{}, err
	}
	// 1. StringToSign
	policy := base64.StdEncoding.EncodeToString(data)
	// 2. Signing Key
	hash := hmac.New(sha256.New, buildSignature(nowTime, s3.SecretKey, s3.Region, serviceName))
	hash.Write([]byte(policy))
	// 3. Signature
	signature := hex.EncodeToString(hash.Sum(nil))

	uploadURL := uploadConfig.UploadURL
	if uploadURL == "" {
		uploadURL = fmt.Sprintf(defaultUploadURLFormat, uploadConfig.BucketName)
	}

	// essential fields
	form := map[string]string{
		"key":              uploadConfig.ObjectKey,
		"Content-Type":     uploadConfig.ContentType,
		"X-Amz-Credential": credential,
		"X-Amz-Algorithm":  algorithm,
		"X-Amz-Date":       nowTime.Format(amzDateISO8601TimeFormat),
		"Policy":           policy,
		"X-Amz-Signature":  signature,
	}

	// optional fields
	if uploadConfig.ContentDisposition != "" {
		form["Content-Disposition"] = uploadConfig.ContentDisposition
	}

	if uploadConfig.ACL != "" {
		form["acl"] = uploadConfig.ACL
	}

	for k, v := range uploadConfig.MetaData {
		form[k] = v
	}
	return UploadPolicies{
		URL:  uploadURL,
		Form: form,
	}, nil
}

func buildUploadSign(nowTime time.Time, credential string, uploadConfig UploadConfig) ([]byte, error) {
	// essential conditions
	conditions := []interface{}{
		map[string]string{"bucket": uploadConfig.BucketName},
		map[string]string{"key": uploadConfig.ObjectKey},
		map[string]string{"Content-Type": uploadConfig.ContentType},
		[]interface{}{"content-length-range", uploadConfig.FileSize, uploadConfig.FileSize},
		map[string]string{"x-amz-credential": credential},
		map[string]string{"x-amz-algorithm": algorithm},
		map[string]string{"x-amz-date": nowTime.Format(amzDateISO8601TimeFormat)},
	}

	// optional conditions
	if uploadConfig.ContentDisposition != "" {
		conditions = append(conditions, map[string]string{"Content-Disposition": uploadConfig.ContentDisposition})
	}

	if uploadConfig.ACL != "" {
		conditions = append(conditions, map[string]string{"acl": uploadConfig.ACL})
	}

	for k, v := range uploadConfig.MetaData {
		conditions = append(conditions, map[string]string{k: v})
	}

	expiration := defaultExpirationHour
	if uploadConfig.Expiration > 0 {
		expiration = uploadConfig.Expiration
	}

	return json.Marshal(&PolicyJSON{
		Expiration: nowTime.Add(expiration).Format(expirationTimeFormat),
		Conditions: conditions,
	})
}

func (s3 S3) buildCredential(nowTime time.Time) []byte {
	var b bytes.Buffer
	b.WriteString(s3.AccessKey)
	b.WriteRune('/')
	b.WriteString(nowTime.Format(shortTimeFormat))
	b.WriteRune('/')
	b.WriteString(s3.Region)
	b.WriteRune('/')
	b.WriteString(serviceName)
	b.WriteRune('/')
	b.WriteString("aws4_request")
	return b.Bytes()
}

func (s3 S3) buildCredentialWithoutKey(nowTime time.Time) []byte {
	var b bytes.Buffer
	b.WriteString(nowTime.Format(shortTimeFormat))
	b.WriteRune('/')
	b.WriteString(s3.Region)
	b.WriteRune('/')
	b.WriteString(serviceName)
	b.WriteRune('/')
	b.WriteString("aws4_request")
	return b.Bytes()
}

func buildSignature(nowTime time.Time, secretAccessKey string, regionName string, serviceName string) []byte {
	shortTime := nowTime.Format(shortTimeFormat)

	date := makeHMac([]byte("AWS4"+secretAccessKey), []byte(shortTime))
	region := makeHMac(date, []byte(regionName))
	service := makeHMac(region, []byte(serviceName))
	credentials := makeHMac(service, []byte("aws4_request"))
	return credentials
}

func makeHMac(key []byte, data []byte) []byte {
	hash := hmac.New(sha256.New, key)
	hash.Write(data)
	return hash.Sum(nil)
}
