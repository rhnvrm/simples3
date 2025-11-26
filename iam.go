// LICENSE BSD-2-Clause-FreeBSD
// Copyright (c) 2018, Rohan Verma <hello@rohanverma.net>

package simples3

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	imdsTokenHeader        = "X-aws-ec2-metadata-token"
	imdsTokenTtlHeader     = "X-aws-ec2-metadata-token-ttl-seconds"
	metadataBaseURL        = "http://169.254.169.254/latest"
	securityCredentialsURI = "/meta-data/iam/security-credentials/"
	imdsTokenURI           = "/api/token"
	defaultIMDSTokenTTL    = "60"
)

// IAMResponse is used by NewUsingIAM to auto
// detect the credentials.
type IAMResponse struct {
	Code            string    `json:"Code"`
	LastUpdated     string    `json:"LastUpdated"`
	Type            string    `json:"Type"`
	AccessKeyID     string    `json:"AccessKeyId"`
	SecretAccessKey string    `json:"SecretAccessKey"`
	Token           string    `json:"Token"`
	Expiration      time.Time `json:"Expiration"`
}

// NewUsingIAM automatically generates an Instance of S3
// using instance metatdata.
func NewUsingIAM(region string) (*S3, error) {
	return newUsingIAM(
		&http.Client{
			// Set a timeout of 3 seconds for AWS IAM Calls.
			Timeout: time.Second * 3, //nolint:gomnd
		}, metadataBaseURL, region)
}

// fetchIMDSToken retrieves an IMDSv2 token from the
// EC2 instance metadata service. It returns a token and boolean,
// only if IMDSv2 is enabled in the EC2 instance metadata
// configuration, otherwise returns an error.
func fetchIMDSToken(cl *http.Client, baseURL string) (string, bool, error) {
	req, err := http.NewRequest(http.MethodPut, baseURL+imdsTokenURI, nil)
	if err != nil {
		return "", false, err
	}

	// Set the token TTL to 60 seconds.
	req.Header.Set(imdsTokenTtlHeader, defaultIMDSTokenTTL)

	resp, err := cl.Do(req)
	if err != nil {
		return "", false, err
	}

	defer func() {
		resp.Body.Close()
		io.Copy(io.Discard, resp.Body)
	}()

	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("failed to request IMDSv2 token: %s", resp.Status)
	}

	token, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false, err
	}

	return string(token), true, nil
}

// fetchIAMData fetches the IAM data from the given URL.
// In case of a normal AWS setup, baseURL would be metadataBaseURL.
// You can use this method, to manually fetch IAM data from a custom
// endpoint and pass it to SetIAMData.
func fetchIAMData(cl *http.Client, baseURL string) (IAMResponse, error) {
	token, useIMDSv2, err := fetchIMDSToken(cl, baseURL)
	if err != nil {
		return IAMResponse{}, fmt.Errorf("error fetching IMDSv2 token: %w", err)
	}

	url := baseURL + securityCredentialsURI

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return IAMResponse{}, fmt.Errorf("error creating imdsv2 token request: %w", err)
	}

	if useIMDSv2 {
		req.Header.Set(imdsTokenHeader, token)
	}

	resp, err := cl.Do(req)
	if err != nil {
		return IAMResponse{}, err
	}

	if resp.StatusCode != http.StatusOK {
		return IAMResponse{}, fmt.Errorf("error fetching IAM data: %s", resp.Status)
	}

	role, err := io.ReadAll(resp.Body)
	if err != nil {
		return IAMResponse{}, err
	}

	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	req, err = http.NewRequest(http.MethodGet, url+string(role), nil)
	if err != nil {
		return IAMResponse{}, fmt.Errorf("error creating role request: %w", err)
	}
	if useIMDSv2 {
		req.Header.Set(imdsTokenHeader, token)
	}

	resp, err = cl.Do(req)
	if err != nil {
		return IAMResponse{}, fmt.Errorf("error fetching role data: %w", err)
	}

	defer func() {
		// Drain and close the body to let the Transport reuse the connection
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return IAMResponse{}, fmt.Errorf("error fetching role data, got non 200 code: %s", resp.Status)
	}

	var jResp IAMResponse
	jsonString, err := io.ReadAll(resp.Body)
	if err != nil {
		return IAMResponse{}, fmt.Errorf("error reading role data: %w", err)
	}

	if err := json.Unmarshal(jsonString, &jResp); err != nil {
		return IAMResponse{}, fmt.Errorf("error unmarshalling role data: %w (%s)", err, jsonString)
	}

	return jResp, nil
}

func newUsingIAM(cl *http.Client, baseURL, region string) (*S3, error) {
	// Get the IAM role
	iamResp, err := fetchIAMData(cl, baseURL)
	if err != nil {
		return nil, fmt.Errorf("error fetching IAM data: %w", err)
	}

	return &S3{
		Region:    region,
		AccessKey: iamResp.AccessKeyID,
		SecretKey: iamResp.SecretAccessKey,
		Token:     iamResp.Token,

		URIFormat: "https://s3.%s.amazonaws.com/%s",
		initMode:  "iam",
		expiry:    iamResp.Expiration,
	}, nil
}

// setIAMData sets the IAM data on the S3 instance.
func (s3 *S3) SetIAMData(iamResp IAMResponse) {
	s3.AccessKey = iamResp.AccessKeyID
	s3.SecretKey = iamResp.SecretAccessKey
	s3.Token = iamResp.Token
}

func (s3 *S3) renewIAMToken() error {
	if s3.initMode != "iam" {
		return nil
	}

	if time.Since(s3.expiry) < 0 {
		return nil
	}

	s3.mu.Lock()
	defer s3.mu.Unlock()
	iamResp, err := fetchIAMData(s3.getClient(), metadataBaseURL)
	if err != nil {
		return fmt.Errorf("error fetching IAM data: %w", err)
	}

	s3.expiry = iamResp.Expiration
	s3.Token = iamResp.Token
	s3.AccessKey = iamResp.AccessKeyID
	s3.SecretKey = iamResp.SecretAccessKey

	return nil
}
