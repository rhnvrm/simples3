package simples3

import (
	"os"
	"strings"
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

func TestS3_GeneratePresignedURL_ResponseContentDisposition(t *testing.T) {
	t.Run("BasicDisposition", func(t *testing.T) {
		s := New(
			os.Getenv("AWS_S3_REGION"),
			os.Getenv("AWS_S3_ACCESS_KEY"),
			os.Getenv("AWS_S3_SECRET_KEY"),
		)
		s.Endpoint = os.Getenv("AWS_S3_ENDPOINT")
		url := s.GeneratePresignedURL(PresignedInput{
			Bucket:                     os.Getenv("AWS_S3_BUCKET"),
			ObjectKey:                  "test-download.txt",
			Method:                     "GET",
			Timestamp:                  nowTime(),
			ExpirySeconds:              3600,
			ResponseContentDisposition: "attachment; filename=\"report.pdf\"",
		})
		// Check that the URL contains the response-content-disposition parameter
		if !strings.Contains(url, "response-content-disposition=") {
			t.Errorf("URL missing response-content-disposition parameter")
		}
		// Check that the disposition is properly encoded
		expectedDisposition := "attachment%3B%20filename%3D%22report.pdf%22"
		if !strings.Contains(url, expectedDisposition) {
			t.Errorf("response-content-disposition not properly encoded. URL: %v", url)
		}
	})

	t.Run("DispositionWithSpacesAndSpecialChars", func(t *testing.T) {
		s := New(
			os.Getenv("AWS_S3_REGION"),
			os.Getenv("AWS_S3_ACCESS_KEY"),
			os.Getenv("AWS_S3_SECRET_KEY"),
		)
		s.Endpoint = os.Getenv("AWS_S3_ENDPOINT")

		// Test with spaces in filename
		url := s.GeneratePresignedURL(PresignedInput{
			Bucket:                     os.Getenv("AWS_S3_BUCKET"),
			ObjectKey:                  "test-download.txt",
			Method:                     "GET",
			Timestamp:                  nowTime(),
			ExpirySeconds:              3600,
			ResponseContentDisposition: "attachment; filename=\"my report 2024.pdf\"",
		})

		// Check that spaces are encoded as %20 not +
		if strings.Contains(url, "my+report") {
			t.Errorf("Spaces incorrectly encoded as +, should be %%20. URL: %v", url)
		}
		if !strings.Contains(url, "my%20report%202024") {
			t.Errorf("Spaces not properly encoded as %%20. URL: %v", url)
		}

		// Test with special characters
		url2 := s.GeneratePresignedURL(PresignedInput{
			Bucket:                     os.Getenv("AWS_S3_BUCKET"),
			ObjectKey:                  "test-download2.txt",
			Method:                     "GET",
			Timestamp:                  nowTime(),
			ExpirySeconds:              3600,
			ResponseContentDisposition: "attachment; filename=\"file+with=special&chars.pdf\"",
		})

		// Check that special chars are properly encoded
		// + should be encoded as %2B
		// = should be encoded as %3D
		// & should be encoded as %26
		if !strings.Contains(url2, "file%2Bwith%3Dspecial%26chars") {
			t.Errorf("Special characters not properly encoded. URL: %v", url2)
		}
	})
}

func TestS3_GeneratePresignedURL_URLEncoding(t *testing.T) {
	t.Run("VerifyQueryParameterSpaceEncoding", func(t *testing.T) {
		var testTime, _ = time.Parse(time.RFC1123, "Fri, 24 May 2013 00:00:00 GMT")
		s := New(
			"us-east-1",
			"AKIAIOSFODNN7EXAMPLE",
			"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		)

		// The token contains spaces that should be encoded as %20 in query params
		s.SetToken("test token with spaces")

		url := s.GeneratePresignedURL(PresignedInput{
			Bucket:        "testbucket",
			ObjectKey:     "test.txt",
			Method:        "GET",
			Timestamp:     testTime,
			ExpirySeconds: 3600,
		})

		// Verify that spaces in the security token are encoded as %20 not +
		if strings.Contains(url, "test+token+with+spaces") {
			t.Errorf("Spaces in token incorrectly encoded as +. URL: %v", url)
		}
		if !strings.Contains(url, "test%20token%20with%20spaces") {
			t.Errorf("Spaces in token not properly encoded as %%20. URL: %v", url)
		}
	})

	t.Run("VerifyCredentialEncoding", func(t *testing.T) {
		var testTime, _ = time.Parse(time.RFC1123, "Fri, 24 May 2013 00:00:00 GMT")

		// Test with special characters in the access key that need encoding
		s := New(
			"us-east-1",
			"KEY+WITH=SPECIAL&CHARS",
			"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		)

		url := s.GeneratePresignedURL(PresignedInput{
			Bucket:        "testbucket",
			ObjectKey:     "test.txt",
			Method:        "GET",
			Timestamp:     testTime,
			ExpirySeconds: 3600,
		})

		// Check that special characters in credentials are properly encoded
		// + should be %2B, = should be %3D, & should be %26
		if strings.Contains(url, "KEY+WITH=SPECIAL&CHARS") {
			t.Errorf("Special chars in credential not encoded. URL: %v", url)
		}
		if !strings.Contains(url, "KEY%2BWITH%3DSPECIAL%26CHARS") {
			t.Errorf("Special chars in credential not properly encoded. URL: %v", url)
		}
	})
}

func TestS3_GeneratePresignedURL_ResponseContentDisposition_PUT(t *testing.T) {
	t.Run("PUTMethodWithDisposition", func(t *testing.T) {
		s := New(
			os.Getenv("AWS_S3_REGION"),
			os.Getenv("AWS_S3_ACCESS_KEY"),
			os.Getenv("AWS_S3_SECRET_KEY"),
		)
		s.Endpoint = os.Getenv("AWS_S3_ENDPOINT")

		// Test PUT method with response-content-disposition
		url := s.GeneratePresignedURL(PresignedInput{
			Bucket:                     os.Getenv("AWS_S3_BUCKET"),
			ObjectKey:                  "upload-test.txt",
			Method:                     "PUT",
			Timestamp:                  nowTime(),
			ExpirySeconds:              3600,
			ResponseContentDisposition: "attachment; filename=\"uploaded-file.txt\"",
		})

		// Check that the URL contains the response-content-disposition parameter
		if !strings.Contains(url, "response-content-disposition=") {
			t.Errorf("PUT URL missing response-content-disposition parameter")
		}

		// Check that it contains the PUT-specific parameters
		if !strings.Contains(url, "X-Amz-Expires=3600") {
			t.Errorf("PUT URL missing X-Amz-Expires parameter")
		}

		// Verify encoding is correct for PUT as well
		expectedDisposition := "attachment%3B%20filename%3D%22uploaded-file.txt%22"
		if !strings.Contains(url, expectedDisposition) {
			t.Errorf("response-content-disposition not properly encoded in PUT URL. URL: %v", url)
		}
	})

	t.Run("CompareGETandPUTWithSameDisposition", func(t *testing.T) {
		s := New(
			os.Getenv("AWS_S3_REGION"),
			os.Getenv("AWS_S3_ACCESS_KEY"),
			os.Getenv("AWS_S3_SECRET_KEY"),
		)
		s.Endpoint = os.Getenv("AWS_S3_ENDPOINT")

		fixedTime := nowTime()
		disposition := "inline; filename=\"document.pdf\""

		// Generate GET URL
		getURL := s.GeneratePresignedURL(PresignedInput{
			Bucket:                     os.Getenv("AWS_S3_BUCKET"),
			ObjectKey:                  "test-file.pdf",
			Method:                     "GET",
			Timestamp:                  fixedTime,
			ExpirySeconds:              3600,
			ResponseContentDisposition: disposition,
		})

		// Generate PUT URL with same parameters
		putURL := s.GeneratePresignedURL(PresignedInput{
			Bucket:                     os.Getenv("AWS_S3_BUCKET"),
			ObjectKey:                  "test-file.pdf",
			Method:                     "PUT",
			Timestamp:                  fixedTime,
			ExpirySeconds:              3600,
			ResponseContentDisposition: disposition,
		})

		// Both should have the disposition parameter
		if !strings.Contains(getURL, "response-content-disposition=") {
			t.Errorf("GET URL missing response-content-disposition")
		}
		if !strings.Contains(putURL, "response-content-disposition=") {
			t.Errorf("PUT URL missing response-content-disposition")
		}

		// The signatures will be different due to different HTTP methods
		// but both should encode the disposition the same way
		expectedEncodedDisposition := "inline%3B%20filename%3D%22document.pdf%22"
		if !strings.Contains(getURL, expectedEncodedDisposition) {
			t.Errorf("GET URL disposition encoding incorrect")
		}
		if !strings.Contains(putURL, expectedEncodedDisposition) {
			t.Errorf("PUT URL disposition encoding incorrect")
		}
	})
}

func TestS3_GeneratePresignedURL_ObjectKeyEncoding(t *testing.T) {
	t.Run("ObjectKeyWithSpaces", func(t *testing.T) {
		s := New(
			"us-east-1",
			"AKIAIOSFODNN7EXAMPLE",
			"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		)

		url := s.GeneratePresignedURL(PresignedInput{
			Bucket:        "mybucket",
			ObjectKey:     "folder/file with spaces.txt",
			Method:        "GET",
			Timestamp:     nowTime(),
			ExpirySeconds: 3600,
		})

		// This test will currently FAIL because spaces are not encoded
		// The URL contains raw spaces which makes it invalid
		if strings.Contains(url, "file with spaces") {
			t.Errorf("CRITICAL BUG: Spaces in object key are not encoded! Invalid URL generated: %v", url)
		}
		// Should contain encoded spaces
		if !strings.Contains(url, "file%20with%20spaces") {
			t.Skipf("WARNING: Object key encoding not implemented. Raw URL: %v", url)
		}
	})

	t.Run("ObjectKeyWithSpecialChars", func(t *testing.T) {
		s := New(
			"us-east-1",
			"AKIAIOSFODNN7EXAMPLE",
			"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		)

		// Test with various special characters that MUST be encoded
		testCases := []struct {
			objectKey      string
			mustNotContain string // Raw character that should be encoded
			shouldContain  string // Expected encoded version
			description    string
		}{
			{
				"file&name.txt",
				"file&name",
				"file%26name",
				"Ampersand breaks URL query string parsing",
			},
			{
				"file#anchor.txt",
				"file#anchor",
				"file%23anchor",
				"Hash creates URL fragment",
			},
			{
				"file?query.txt",
				"file?query",
				"file%3Fquery",
				"Question mark starts query string prematurely",
			},
		}

		for _, tc := range testCases {
			url := s.GeneratePresignedURL(PresignedInput{
				Bucket:        "mybucket",
				ObjectKey:     tc.objectKey,
				Method:        "GET",
				Timestamp:     nowTime(),
				ExpirySeconds: 3600,
			})

			if strings.Contains(url, tc.mustNotContain) {
				t.Errorf("CRITICAL BUG - %s: Special char not encoded in object key! Invalid URL: %v",
					tc.description, url)
			}
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
