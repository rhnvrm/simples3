package simples3

import (
	"os"
	"testing"
	"time"
)

func TestS3_GeneratePresignedURL(t *testing.T) {
	// Params based on
	// https://docs.aws.amazon.com/AmazonS3/latest/API/sigv4-query-string-auth.html
	var time, _ = time.Parse(time.RFC1123, "Fri, 24 May 2013 00:00:00 GMT")
	t.Run("Test", func(t *testing.T) {
		s := New(
			"us-east-1",
			"AKIAIOSFODNN7EXAMPLE",
			"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		)
		want := "https://examplebucket.s3.amazonaws.com/test.txt?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=AKIAIOSFODNN7EXAMPLE%2F20130524%2Fus-east-1%2Fs3%2Faws4_request&X-Amz-Date=20130524T000000Z&X-Amz-Expires=86400&X-Amz-SignedHeaders=host&X-Amz-Signature=aeeed9bbccd4d02ee5c0109b86d86835f995330da4c265957d157751f604d404"
		if got := s.GeneratePresignedURL(PresignedInput{
			Bucket:        "examplebucket",
			ObjectKey:     "test.txt",
			Method:        "GET",
			Timestamp:     time,
			ExpirySeconds: 86400,
		}); got != want {
			t.Errorf("S3.GeneratePresignedURL() = %v, want %v", got, want)
		}
	})
}

func TestS3_GeneratePresignedURL_Personal(t *testing.T) {
	t.Run("Test", func(t *testing.T) {
		s := New(
			os.Getenv("AWS_S3_REGION"),
			os.Getenv("AWS_S3_ACCESS_KEY"),
			os.Getenv("AWS_S3_SECRET_KEY"),
		)
		dontwant := ""
		if got := s.GeneratePresignedURL(PresignedInput{
			Bucket:        os.Getenv("AWS_S3_BUCKET"),
			ObjectKey:     "test1.txt",
			Method:        "GET",
			Timestamp:     NowTime(),
			ExpirySeconds: 3600,
		}); got == dontwant {
			t.Errorf("S3.GeneratePresignedURL() = %v, dontwant %v", got, dontwant)
		}
	})
}

func TestS3_GeneratePresignedURL_ExtraHeader(t *testing.T) {
	t.Run("Test", func(t *testing.T) {
		s := New(
			os.Getenv("AWS_S3_REGION"),
			os.Getenv("AWS_S3_ACCESS_KEY"),
			os.Getenv("AWS_S3_SECRET_KEY"),
		)
		dontwant := ""
		if got := s.GeneratePresignedURL(PresignedInput{
			Bucket:        os.Getenv("AWS_S3_BUCKET"),
			ObjectKey:     "test2.txt",
			Method:        "GET",
			Timestamp:     NowTime(),
			ExpirySeconds: 3600,
			ExtraHeaders: map[string]string{
				"x-amz-meta-test": "test",
			},
		}); got == dontwant {
			t.Errorf("S3.GeneratePresignedURL() = %v, dontwant %v", got, dontwant)
		}
	})
}

func TestS3_GeneratePresignedURL_PUT(t *testing.T) {
	t.Run("Test", func(t *testing.T) {
		s := New(
			os.Getenv("AWS_S3_REGION"),
			os.Getenv("AWS_S3_ACCESS_KEY"),
			os.Getenv("AWS_S3_SECRET_KEY"),
		)
		dontwant := ""
		if got := s.GeneratePresignedURL(PresignedInput{
			Bucket:        os.Getenv("AWS_S3_BUCKET"),
			ObjectKey:     "test2.txt",
			Method:        "PUT",
			Timestamp:     NowTime(),
			ExpirySeconds: 3600,
		}); got == dontwant {
			t.Errorf("S3.GeneratePresignedURL() = %v, dontwant %v", got, dontwant)
		}
	})
}

func BenchmarkS3_GeneratePresigned(b *testing.B) {
	// run the Fib function b.N times
	s := New(
		os.Getenv("AWS_S3_REGION"),
		os.Getenv("AWS_S3_ACCESS_KEY"),
		os.Getenv("AWS_S3_SECRET_KEY"),
	)
	for n := 0; n < b.N; n++ {
		s.GeneratePresignedURL(PresignedInput{
			Bucket:        os.Getenv("AWS_S3_BUCKET"),
			ObjectKey:     "test.txt",
			Method:        "GET",
			Timestamp:     NowTime(),
			ExpirySeconds: 3600,
		})
	}
}
