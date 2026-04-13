package simples3

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestCreateUploadPolicies_UsePathStyleFalseUsesVirtualHostedStyle(t *testing.T) {
	s3 := New("us-west-2", "AccessKey", "SuperSecretKey").SetUsePathStyle(false)

	policies, err := s3.CreateUploadPolicies(UploadConfig{
		BucketName:  "examplebucket",
		ObjectKey:   "test.txt",
		ContentType: "text/plain",
		FileSize:    123,
	})
	if err != nil {
		t.Fatalf("CreateUploadPolicies() error = %v", err)
	}

	if policies.URL != "https://examplebucket.s3.us-west-2.amazonaws.com/" {
		t.Fatalf("URL = %q, want https://examplebucket.s3.us-west-2.amazonaws.com/", policies.URL)
	}
}

func TestCreateUploadPolicies_UsePathStyleFalseFallbacksToPathStyle(t *testing.T) {
	s3 := New("us-west-2", "AccessKey", "SuperSecretKey")
	s3.SetEndpoint("https://objects.example.com/base")
	s3.SetUsePathStyle(false)

	policies, err := s3.CreateUploadPolicies(UploadConfig{
		BucketName:  "examplebucket",
		ObjectKey:   "test.txt",
		ContentType: "text/plain",
		FileSize:    123,
	})
	if err != nil {
		t.Fatalf("CreateUploadPolicies() error = %v", err)
	}

	if policies.URL != "https://objects.example.com/base/examplebucket" {
		t.Fatalf("URL = %q, want https://objects.example.com/base/examplebucket", policies.URL)
	}
}

func TestCreateUploadPolicies_UsePathStyleFalseFallbacksInvalidDottedLabelToPathStyle(t *testing.T) {
	s3 := New("us-west-2", "AccessKey", "SuperSecretKey").SetUsePathStyle(false)

	policies, err := s3.CreateUploadPolicies(UploadConfig{
		BucketName:  "a.-b",
		ObjectKey:   "test.txt",
		ContentType: "text/plain",
		FileSize:    123,
	})
	if err != nil {
		t.Fatalf("CreateUploadPolicies() error = %v", err)
	}

	if policies.URL != "https://s3.us-west-2.amazonaws.com/a.-b" {
		t.Fatalf("URL = %q, want https://s3.us-west-2.amazonaws.com/a.-b", policies.URL)
	}
}

func TestCreateUploadPolicies_SetUsePathStyleTrueUsesPathStyle(t *testing.T) {
	s3 := New("us-west-2", "AccessKey", "SuperSecretKey").SetUsePathStyle(true)

	policies, err := s3.CreateUploadPolicies(UploadConfig{
		BucketName:  "examplebucket",
		ObjectKey:   "test.txt",
		ContentType: "text/plain",
		FileSize:    123,
	})
	if err != nil {
		t.Fatalf("CreateUploadPolicies() error = %v", err)
	}

	if policies.URL != "https://s3.us-west-2.amazonaws.com/examplebucket" {
		t.Fatalf("URL = %q, want https://s3.us-west-2.amazonaws.com/examplebucket", policies.URL)
	}
}

func TestCreateUploadPolicies_TokenAddsFormFieldAndPolicyCondition(t *testing.T) {
	s3 := New("us-west-2", "AccessKey", "SuperSecretKey")
	s3.SetToken("session-token")

	policies, err := s3.CreateUploadPolicies(UploadConfig{
		BucketName:  "examplebucket",
		ObjectKey:   "test.txt",
		ContentType: "text/plain",
		FileSize:    123,
	})
	if err != nil {
		t.Fatalf("CreateUploadPolicies() error = %v", err)
	}

	if got := policies.Form["X-Amz-Security-Token"]; got != "session-token" {
		t.Fatalf("form[X-Amz-Security-Token] = %q, want session-token", got)
	}

	decodedPolicy, err := base64.StdEncoding.DecodeString(policies.Form["Policy"])
	if err != nil {
		t.Fatalf("DecodeString() error = %v", err)
	}

	var policy PolicyJSON
	if err := json.Unmarshal(decodedPolicy, &policy); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if !policyHasCondition(policy, "x-amz-security-token", "session-token") {
		t.Fatalf("policy missing x-amz-security-token condition: %s", string(decodedPolicy))
	}
}

func policyHasCondition(policy PolicyJSON, key, value string) bool {
	for _, condition := range policy.Conditions {
		conditionMap, ok := condition.(map[string]interface{})
		if !ok {
			continue
		}
		if got, ok := conditionMap[key]; ok && got == value {
			return true
		}
	}
	return false
}
