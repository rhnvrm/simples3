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

func TestS3_GeneratePresignedURL_Token(t *testing.T) {
	// Params based on
	// https://docs.aws.amazon.com/AmazonS3/latest/API/sigv4-query-string-auth.html
	var time, _ = time.Parse(time.RFC1123, "Fri, 24 May 2013 00:00:00 GMT")
	t.Run("Test", func(t *testing.T) {
		s := New(
			"us-east-1",
			"AKIAIOSFODNN7EXAMPLE",
			"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		)
		s.SetToken("IQoJb3JpT2luX2VjEPP%2F%2F%2F%2F%2F%2F%2F%2F%2F%2FwEaCmFwLXNvdXRoLTEiRzBFAiABaeeW0LZZaqVyQVx8EHfCY9KTLsR0hnw1nDae%2F%2BVDbwIhAKrGP4RYkoPv8x0qFScsp%2FQZZXAYWbspMOMpVEBa1%2FQ3Kr8DCPz%2F%2F%2F%2F%2F%2F%2F%2F%2F%2FwEQARoMOTMyNjk0MjUxNzI3IgxHyURIpz%2FBVH7V0ikqkwMTy9uf3umf7OWghmeDE8fpS7KxXYlTCQdVyC6tHcTQZdZ13qziy0ZgImvJEUz4lFNCszdQWR2jaDjgNGvWEUJ1ODAir7F1gTb%2BSx0PpH8o18yrrTJYCwZe7ZKtViCN2yDKHAk8DN9Ke77fYEl2W%2FLWV3VH9oqwEwUzCh4f6JrluiLW6HaxHcDqu7K6Qk8bhgTVlW5eHBzlyRJtrlmy232auL1m8XAoR01sjnpoCwE0ra1L3QuK7XmC9BIR5bRwMdZFcL0Ai0vzCyX9kd15hhDBRgzKrTNSrBFDaRJ9N%2FV3bZ61RAd%2FkwfQEDBiwUcTdm%2BVDLvxIUfVNmtQj628ZCWi%2BztUAe8Yz8IKpY50nEXr%2BHHX4wtVF2MZQPSOr%2B%2FON3OJYCl6TwVTGWoVGapn9y%2Bj9JOcdnnDuFUJMoJERRWnMNPCadZT68%2B3t30IgmXU4hcSX51olExLeGMSMtfK6LC7YCvMlGG8YxIJAeW5qznc2d9u%2BX7nXjqhvPCyc9hXMv4hXS4rowWnR6gaz6xZuY9fb8TMIK4v%2FQFOusBpv3m9H7b45zUr3o6xYh28GyB5%2F9zW%2FPkfm%2FpysDbwfz3r3G0WLchyE0t4%2BH8YZibj0KwY8rJyAV26u2DzIlp0bmJ%2F7Aaq4wUo%2BgUbhz7NMFUpWuR2ywszf28pdgsRQ4SHAlVQ4rOhx5XGqMREzjFPJo7jRW6uMCSJ8LvrQU38VTpZyrm7yQDCBK2lHwU00O8xTWSDhFXmrqFrCL9P76ZYXh2dCCJm6gPiSU3eGyqGBKDBWFt20lRHLWCyXwiyhGRULg3WLoLDVsjJDRO8xZta8nVxALUZLcteEv%2BE1QGCxVSg1W1WSAGLz8FQ%3D%3D")
		want := "https://examplebucket.s3.amazonaws.com/test.txt?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=AKIAIOSFODNN7EXAMPLE%2F20130524%2Fus-east-1%2Fs3%2Faws4_request&X-Amz-Date=20130524T000000Z&X-Amz-Expires=86400&X-Amz-Security-Token=IQoJb3JpT2luX2VjEPP%252F%252F%252F%252F%252F%252F%252F%252F%252F%252FwEaCmFwLXNvdXRoLTEiRzBFAiABaeeW0LZZaqVyQVx8EHfCY9KTLsR0hnw1nDae%252F%252BVDbwIhAKrGP4RYkoPv8x0qFScsp%252FQZZXAYWbspMOMpVEBa1%252FQ3Kr8DCPz%252F%252F%252F%252F%252F%252F%252F%252F%252F%252FwEQARoMOTMyNjk0MjUxNzI3IgxHyURIpz%252FBVH7V0ikqkwMTy9uf3umf7OWghmeDE8fpS7KxXYlTCQdVyC6tHcTQZdZ13qziy0ZgImvJEUz4lFNCszdQWR2jaDjgNGvWEUJ1ODAir7F1gTb%252BSx0PpH8o18yrrTJYCwZe7ZKtViCN2yDKHAk8DN9Ke77fYEl2W%252FLWV3VH9oqwEwUzCh4f6JrluiLW6HaxHcDqu7K6Qk8bhgTVlW5eHBzlyRJtrlmy232auL1m8XAoR01sjnpoCwE0ra1L3QuK7XmC9BIR5bRwMdZFcL0Ai0vzCyX9kd15hhDBRgzKrTNSrBFDaRJ9N%252FV3bZ61RAd%252FkwfQEDBiwUcTdm%252BVDLvxIUfVNmtQj628ZCWi%252BztUAe8Yz8IKpY50nEXr%252BHHX4wtVF2MZQPSOr%252B%252FON3OJYCl6TwVTGWoVGapn9y%252Bj9JOcdnnDuFUJMoJERRWnMNPCadZT68%252B3t30IgmXU4hcSX51olExLeGMSMtfK6LC7YCvMlGG8YxIJAeW5qznc2d9u%252BX7nXjqhvPCyc9hXMv4hXS4rowWnR6gaz6xZuY9fb8TMIK4v%252FQFOusBpv3m9H7b45zUr3o6xYh28GyB5%252F9zW%252FPkfm%252FpysDbwfz3r3G0WLchyE0t4%252BH8YZibj0KwY8rJyAV26u2DzIlp0bmJ%252F7Aaq4wUo%252BgUbhz7NMFUpWuR2ywszf28pdgsRQ4SHAlVQ4rOhx5XGqMREzjFPJo7jRW6uMCSJ8LvrQU38VTpZyrm7yQDCBK2lHwU00O8xTWSDhFXmrqFrCL9P76ZYXh2dCCJm6gPiSU3eGyqGBKDBWFt20lRHLWCyXwiyhGRULg3WLoLDVsjJDRO8xZta8nVxALUZLcteEv%252BE1QGCxVSg1W1WSAGLz8FQ%253D%253D&X-Amz-SignedHeaders=host&X-Amz-Signature=29d003f449ae4106d1c4cabaeebf84fc47960ee127e98f1b9132261852250cb4"
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
		s.Endpoint = os.Getenv("AWS_S3_ENDPOINT")
		dontwant := ""
		if got := s.GeneratePresignedURL(PresignedInput{
			Bucket:        os.Getenv("AWS_S3_BUCKET"),
			ObjectKey:     "test1.txt",
			Method:        "GET",
			Timestamp:     nowTime(),
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
		s.Endpoint = os.Getenv("AWS_S3_ENDPOINT")
		dontwant := ""
		if got := s.GeneratePresignedURL(PresignedInput{
			Bucket:        os.Getenv("AWS_S3_BUCKET"),
			ObjectKey:     "test2.txt",
			Method:        "GET",
			Timestamp:     nowTime(),
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
		s.Endpoint = os.Getenv("AWS_S3_ENDPOINT")
		dontwant := ""
		if got := s.GeneratePresignedURL(PresignedInput{
			Bucket:        os.Getenv("AWS_S3_BUCKET"),
			ObjectKey:     "test2.txt",
			Method:        "PUT",
			Timestamp:     nowTime(),
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
	s.Endpoint = os.Getenv("AWS_S3_ENDPOINT")

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		s.GeneratePresignedURL(PresignedInput{
			Bucket:        os.Getenv("AWS_S3_BUCKET"),
			ObjectKey:     "test.txt",
			Method:        "GET",
			Timestamp:     nowTime(),
			ExpirySeconds: 3600,
		})
	}
}
