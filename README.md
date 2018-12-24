# simples3 : Simple no frills AWS S3 Library using REST with V4 Signing

## Overview [![GoDoc](https://godoc.org/github.com/rhnvrm/simples3?status.svg)](https://godoc.org/github.com/rhnvrm/simples3) [![Go Report Card](https://goreportcard.com/badge/github.com/rhnvrm/simples3)](https://goreportcard.com/report/github.com/rhnvrm/simples3)

SimpleS3 is a golang library for uploading and deleting objects on S3 buckets using the REST API calls signed using AWS Signature Version 4

## Install

```sh
go get github.com/rhnvrm/simples3
```

## Example

```go
testTxt, _ := os.Open("test.txt")
defer testTxt.Close()

s3 := simples3.New(Region, AWSAccessKey, AWSSecretKey)
err := s3.FileUpload(simples3.UploadInput{
    Bucket:      AWSBucket,
    ObjectKey:   "test.txt",
    ContentType: "text/plain",
    FileName:    "test.txt",
    Body:        testTxt,
}
```

## Contributing

ToDo.

## Contributors

ToDo.

## Author

ToDo.

## License

MIT.