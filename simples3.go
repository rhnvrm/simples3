// LICENSE MIT
// Copyright (c) 2018, Rohan Verma <hello@rohanverma.net>

package simples3

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"time"
)

// S3 provides a wrapper around your S3 credentials.
type S3 struct {
	AccessKey string
	SecretKey string
	Region    string

	URIFormat string
}

// UploadInput is passed to FileUpload as a parameter.
type UploadInput struct {
	Bucket      string
	ObjectKey   string
	FileName    string
	ContentType string
	Body        multipart.File
}

// DeleteInput is passed to FileDelete as a parameter.
type DeleteInput struct {
	Bucket    string
	ObjectKey string
}

// New returns an instance of S3.
func New(region, accessKey, secretKey string) *S3 {
	return &S3{
		Region:    region,
		AccessKey: accessKey,
		SecretKey: secretKey,

		URIFormat: "https://s3.%s.amazonaws.com/%s",
	}
}

func detectFileSize(body multipart.File) int64 {
	switch r := body.(type) {
	case io.Seeker:
		pos, _ := r.Seek(0, 1)
		defer r.Seek(pos, 0)

		n, err := r.Seek(0, 2)
		if err != nil {
			return -1
		}
		return n
	}
	return -1
}

// FileUpload makes a POST call with the file written as multipart
// and on successful upload, checks for 200 OK.
func (s3 *S3) FileUpload(u UploadInput) error {
	policies, err := s3.CreateUploadPolicies(UploadConfig{
		UploadURL:   fmt.Sprintf(s3.URIFormat, s3.Region, u.Bucket),
		BucketName:  u.Bucket,
		ObjectKey:   u.ObjectKey,
		ContentType: u.ContentType,
		FileSize:    detectFileSize(u.Body),
		MetaData: map[string]string{
			"success_action_status": "200",
		},
	})

	if err != nil {
		return err
	}

	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	for k, v := range policies.Form {
		if err := w.WriteField(k, v); err != nil {
			return err
		}
	}

	fw, err := w.CreateFormFile("file", u.FileName)
	if err != nil {
		return err
	}
	if _, err = io.Copy(fw, u.Body); err != nil {
		return err
	}

	// Don't forget to close the multipart writer.
	// If you don't close it, your request will be missing the terminating boundary.
	if err := w.Close(); err != nil {
		return err
	}

	// Now that you have a form, you can submit it to your handler.
	req, err := http.NewRequest("POST", policies.URL, &b)
	if err != nil {
		return err
	}
	// Don't forget to set the content type, this will contain the boundary.
	req.Header.Set("Content-Type", w.FormDataContentType())

	// Submit the request
	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return err
	}

	// Check the response
	if res.Status != "200 OK" {
		err = fmt.Errorf("status code: %s", res.Status)
		data, _ := ioutil.ReadAll(res.Body)
		log.Printf("response:%s\n", string(data))
		return err
	}

	return nil
}

// FileDelete makes a POST call with the file written as multipart
// and on successful upload, checks for 204 No Content.
func (s3 *S3) FileDelete(u DeleteInput) error {
	url := fmt.Sprintf(
		s3.URIFormat+"/%s",
		s3.Region, u.Bucket, u.ObjectKey,
	)
	req, err := http.NewRequest(
		"DELETE", url, nil,
	)
	if err != nil {
		return err
	}

	date := req.Header.Get("Date")
	t := time.Now().UTC()
	if date != "" {
		var err error
		t, err = time.Parse(http.TimeFormat, date)
		if err != nil {
			return err
		}
	}
	req.Header.Set("Date", t.Format(amzDateISO8601TimeFormat))

	// The x-amz-content-sha256 header is required for all AWS
	// Signature Version 4 requests. It provides a hash of the
	// request payload. If there is no payload, you must provide
	// the hash of an empty string.
	emptyhash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	req.Header.Set("x-amz-content-sha256", emptyhash)

	k := s3.signKeys(t)
	h := hmac.New(sha256.New, k)

	s3.writeStringToSign(h, t, req)

	auth := bytes.NewBufferString("AWS4-HMAC-SHA256 ")
	auth.Write([]byte("Credential=" + s3.AccessKey + "/" + s3.creds(t)))
	auth.Write([]byte{',', ' '})
	auth.Write([]byte("SignedHeaders="))
	writeHeaderList(auth, req)
	auth.Write([]byte{',', ' '})
	auth.Write([]byte("Signature=" + fmt.Sprintf("%x", h.Sum(nil))))

	req.Header.Set("Authorization", auth.String())

	// Submit the request
	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return err
	}

	// // Check the response
	if res.Status != "204 No Content" {
		err = fmt.Errorf("status code: %s", res.Status)
		data, _ := ioutil.ReadAll(res.Body)
		log.Printf("response:%s\n", string(data))
		return err
	}

	return nil
}
