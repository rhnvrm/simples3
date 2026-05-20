package simples3

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	normalizeTestEnv()
	if err := ensureTestBucket(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to bootstrap test bucket: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func normalizeTestEnv() {
	aliasEnv("AWS_S3_ACCESS_KEY", "AWS_ACCESS_KEY_ID")
	aliasEnv("AWS_S3_SECRET_KEY", "AWS_SECRET_ACCESS_KEY")
	aliasEnv("AWS_S3_REGION", "AWS_REGION", "AWS_DEFAULT_REGION")
	aliasEnv("AWS_REGION", "AWS_S3_REGION", "AWS_DEFAULT_REGION")
	aliasEnv("AWS_DEFAULT_REGION", "AWS_REGION", "AWS_S3_REGION")

	if os.Getenv("AWS_S3_REGION") == "" && os.Getenv("AWS_S3_ENDPOINT") != "" {
		_ = os.Setenv("AWS_S3_REGION", "us-east-1")
	}
	if os.Getenv("AWS_REGION") == "" && os.Getenv("AWS_S3_REGION") != "" {
		_ = os.Setenv("AWS_REGION", os.Getenv("AWS_S3_REGION"))
	}
	if os.Getenv("AWS_DEFAULT_REGION") == "" && os.Getenv("AWS_S3_REGION") != "" {
		_ = os.Setenv("AWS_DEFAULT_REGION", os.Getenv("AWS_S3_REGION"))
	}
}

func aliasEnv(target string, sources ...string) {
	if os.Getenv(target) != "" {
		return
	}
	for _, source := range sources {
		if value := os.Getenv(source); value != "" {
			_ = os.Setenv(target, value)
			return
		}
	}
}

func ensureTestBucket() error {
	bucket := os.Getenv("AWS_S3_BUCKET")
	accessKey := os.Getenv("AWS_S3_ACCESS_KEY")
	secretKey := os.Getenv("AWS_S3_SECRET_KEY")
	region := os.Getenv("AWS_S3_REGION")
	if bucket == "" || accessKey == "" || secretKey == "" {
		return nil
	}
	if region == "" {
		region = "us-east-1"
	}

	s3 := New(region, accessKey, secretKey)
	if endpoint := os.Getenv("AWS_S3_ENDPOINT"); endpoint != "" {
		s3.SetEndpoint(endpoint)
	}

	_, err := s3.CreateBucket(CreateBucketInput{Bucket: bucket, Region: region})
	if err == nil || bucketAlreadyExistsError(err) {
		return nil
	}
	return err
}

func bucketAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "bucketalreadyownedbyyou") ||
		strings.Contains(message, "bucket already owned by you") ||
		strings.Contains(message, "bucket already exists")
}
