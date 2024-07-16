# simples3 : Simple no frills AWS S3 Library using REST with V4 Signing

## Overview [![GoDoc](https://godoc.org/github.com/rhnvrm/simples3?status.svg)](https://godoc.org/github.com/rhnvrm/simples3) [![Go Report Card](https://goreportcard.com/badge/github.com/rhnvrm/simples3)](https://goreportcard.com/report/github.com/rhnvrm/simples3) [![GoCover](https://gocover.io/_badge/github.com/rhnvrm/simples3)](https://gocover.io/_badge/github.com/rhnvrm/simples3) [![Zerodha Tech](https://zerodha.tech/static/images/github-badge.svg)](https://zerodha.tech) 

SimpleS3 is a Go library for manipulating objects 
in S3 buckets using REST API calls or Presigned URLs signed 
using AWS Signature Version 4.

## Install

```sh
go get github.com/rhnvrm/simples3
```

## Example

```go
testTxt, _ := os.Open("testdata/test.txt")
defer testTxt.Close()

// Create an instance of the package
// You can either create by manually supplying credentials
// (preferably using Environment vars)
s3 := simples3.New(Region, AWSAccessKey, AWSSecretKey)
// or you can use this on an EC2 instance to 
// obtain credentials from IAM attached to the instance.
s3, _ := simples3.NewUsingIAM(Region)

// You can also set a custom endpoint to a compatible s3 instance. 
s3.SetEndpoint(CustomEndpoint)

// Note: Consider adding a testTxt.Seek(0, 0)
// in case you have read 
// the body, as the pointer is shared by the library.

// File Upload is as simple as providing the following
// details.
resp, err := s3.FileUpload(simples3.UploadInput{
    Bucket:      AWSBucket,
    ObjectKey:   "test.txt",
    ContentType: "text/plain",
    FileName:    "test.txt",
    Body:        testTxt,
})

// Similarly, Files can be deleted.
err := s3.FileDelete(simples3.DeleteInput{
    Bucket:    os.Getenv("AWS_S3_BUCKET"),
    ObjectKey: "test.txt",
})

// You can also download the file.
file, _ := s3.FileDownload(simples3.DownloadInput{
    Bucket:    AWSBucket,
    ObjectKey: "test.txt",
})

data, _ := ioutil.ReadAll(file)
file.Close()

// And also list files.
resp, _ := s3.ListObjectsV2(simples3.ListObjectsV2Details{
    Bucket:    AWSBucket,
    Delimiter: "/",
})
for _, c := range resp.Contents {
    fmt.Printf("%s    %s\n", c.Key, c.LastModified)
}

// You can also use this library to generate
// Presigned URLs that can for eg. be used to
// GET/PUT files on S3 through the browser.
var time, _ = time.Parse(time.RFC1123, "Fri, 24 May 2013 00:00:00 GMT")

url := s.GeneratePresignedURL(PresignedInput{
    Bucket:        AWSBucket,
    ObjectKey:     "test.txt",
    Method:        "GET",
    Timestamp:     time,
    ExpirySeconds: 86400,
})
```

## Contributing

You are more than welcome to contribute to this project. Fork and make 
a Pull Request, or create an Issue if you see any problem or want to
propose a feature.

## Author

Rohan Verma <hello@rohanverma.net>

## License

BSD-2-Clause-FreeBSD
