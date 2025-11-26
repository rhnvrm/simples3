# simples3 : Simple no frills AWS S3 Library using REST with V4 Signing

## Overview [![GoDoc](https://godoc.org/github.com/rhnvrm/simples3?status.svg)](https://godoc.org/github.com/rhnvrm/simples3) [![Go Report Card](https://goreportcard.com/badge/github.com/rhnvrm/simples3)](https://goreportcard.com/report/github.com/rhnvrm/simples3) [![GoCover](https://gocover.io/_badge/github.com/rhnvrm/simples3)](https://gocover.io/_badge/github.com/rhnvrm/simples3) [![Zerodha Tech](https://zerodha.tech/static/images/github-badge.svg)](https://zerodha.tech)

SimpleS3 is a Go library for manipulating objects
in S3 buckets using REST API calls or Presigned URLs signed
using AWS Signature Version 4.

**Features:**
- **Simple, intuitive API** following Go idioms
- **Complete S3 operations** - Upload, Download, Delete, List, Details
- **AWS Signature Version 4** signing
- **Custom endpoint support** (MinIO, DigitalOcean Spaces, etc.)
- **Simple List API** with pagination, prefix filtering, and delimiter grouping
- **Iterator-based ListAll** for memory-efficient large bucket iteration (Go 1.23+)
- **Presigned URL generation** for secure browser uploads/downloads
- **IAM credential support** for EC2 instances
- **Comprehensive test coverage**
- **Zero dependencies** - uses only Go standard library

## Install

```sh
go get github.com/rhnvrm/simples3
```

## Quick Start

```go
package main

import (
    "log"
    "os"
    "github.com/rhnvrm/simples3"
)

func main() {
    // Initialize S3 client
    s3 := simples3.New("us-east-1", "your-access-key", "your-secret-key")

    // Upload a file
    file, err := os.Open("my-file.txt")
    if err != nil {
        log.Fatal(err)
    }
    defer file.Close()

    resp, err := s3.FileUpload(simples3.UploadInput{
        Bucket:      "my-bucket",
        ObjectKey:   "my-file.txt",
        ContentType: "text/plain",
        FileName:    "my-file.txt",
        Body:        file,
    })
    if err != nil {
        log.Fatal(err)
    }

    log.Printf("File uploaded successfully: %+v", resp)
}
```

## API Reference

### File Operations

#### Upload Files
```go
// POST upload (recommended for browsers)
resp, err := s3.FileUpload(simples3.UploadInput{
    Bucket:      "my-bucket",
    ObjectKey:   "path/to/file.txt",
    ContentType: "text/plain",
    FileName:    "file.txt",
    Body:        file,
})

// PUT upload (simpler for programmatic use)
resp, err := s3.FilePut(simples3.UploadInput{
    Bucket:      "my-bucket",
    ObjectKey:   "path/to/file.txt",
    ContentType: "text/plain",
    Body:        file,
})
```

#### Download Files
```go
// Download file
file, err := s3.FileDownload(simples3.DownloadInput{
    Bucket:    "my-bucket",
    ObjectKey: "path/to/file.txt",
})
if err != nil {
    log.Fatal(err)
}
defer file.Close()

// Read the content
data, err := io.ReadAll(file)
if err != nil {
    log.Fatal(err)
}
```

#### Delete Files
```go
err := s3.FileDelete(simples3.DeleteInput{
    Bucket:    "my-bucket",
    ObjectKey: "path/to/file.txt",
})
```

#### Get File Details
```go
details, err := s3.FileDetails(simples3.DetailsInput{
    Bucket:    "my-bucket",
    ObjectKey: "path/to/file.txt",
})
if err != nil {
    log.Fatal(err)
}

log.Printf("File size: %s bytes", details.ContentLength)
log.Printf("Last modified: %s", details.LastModified)
log.Printf("Content type: %s", details.ContentType)
```

### List Objects

SimpleS3 provides a clean, easy-to-use List API that follows the same pattern as other library methods:

#### Simple Listing
```go
// List all objects in a bucket
result, err := s3.List(simples3.ListInput{
    Bucket: "my-bucket",
})
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Found %d objects:\n", len(result.Objects))
for _, obj := range result.Objects {
    fmt.Printf("- %s (%d bytes)\n", obj.Key, obj.Size)
}
```

#### Advanced Listing with Options
```go
// List with prefix filtering and pagination
result, err := s3.List(simples3.ListInput{
    Bucket:    "my-bucket",
    Prefix:    "documents/",
    Delimiter: "/",
    MaxKeys:   100,
})
if err != nil {
    log.Fatal(err)
}

// Process objects
for _, obj := range result.Objects {
    fmt.Printf("%s (%d bytes)\n", obj.Key, obj.Size)
}

// Process "directories" (common prefixes)
for _, prefix := range result.CommonPrefixes {
    fmt.Printf("%s/\n", prefix)
}
```

#### Iterator-based Listing (Go 1.23+)
```go
// Use the new ListAll method for memory-efficient iteration
s3 := simples3.New("us-east-1", "your-access-key", "your-secret-key")

// Iterate over all objects with automatic pagination and error handling
seq, finish := s3.ListAll(simples3.ListInput{
    Bucket: "my-bucket",
    Prefix: "documents/", // Optional filtering
})

for obj := range seq {
    fmt.Printf("%s (%d bytes)\n", obj.Key, obj.Size)
}

// Check for any errors that occurred during iteration
if err := finish(); err != nil {
    log.Fatalf("Error during iteration: %v", err)
}
```

#### Handle Pagination (Legacy)
```go
// Handle large result sets with pagination
func listAllObjects(s3 *simples3.S3, bucket string) ([]simples3.Object, error) {
    var allObjects []simples3.Object
    var continuationToken string

    for {
        // List objects with continuation token
        result, err := s3.List(simples3.ListInput{
            Bucket:            bucket,
            ContinuationToken: continuationToken,
            MaxKeys:          1000, // Maximum page size
        })
        if err != nil {
            return nil, err
        }

        // Add current page objects to our collection
        allObjects = append(allObjects, result.Objects...)

        // Check if there are more pages
        if !result.IsTruncated {
            break
        }

        // Set token for next page
        continuationToken = result.NextContinuationToken
    }

    return allObjects, nil
}

// Usage example
func main() {
    s3 := simples3.New("us-east-1", "your-access-key", "your-secret-key")

    allObjects, err := listAllObjects(s3, "my-bucket")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Total objects: %d\n", len(allObjects))
    for _, obj := range allObjects {
        fmt.Printf("- %s (%d bytes)\n", obj.Key, obj.Size)
    }
}
```

### Presigned URLs

Generate secure URLs for browser-based uploads/downloads:

```go
// Generate presigned URL for download
url := s3.GeneratePresignedURL(simples3.PresignedInput{
    Bucket:        "my-bucket",
    ObjectKey:     "private-file.pdf",
    Method:        "GET",
    ExpirySeconds: 3600, // 1 hour
})

// Users can now download directly: <a href="{{url}}">Download</a>
```

### Custom Endpoints

Use with MinIO, DigitalOcean Spaces, or other S3-compatible services:

```go
// MinIO
s3 := simples3.New("us-east-1", "minioadmin", "minioadmin")
s3.SetEndpoint("http://localhost:9000")

// DigitalOcean Spaces
s3 := simples3.New("nyc3", "your-access-key", "your-secret-key")
s3.SetEndpoint("https://nyc3.digitaloceanspaces.com")
```

### IAM Credentials

On EC2 instances, use IAM roles automatically:

```go
s3, err := simples3.NewUsingIAM("us-east-1")
if err != nil {
    log.Fatal(err)
}
// Automatically uses instance IAM role
```

## Development

### Setup Development Environment

```sh
# Clone the repository
git clone https://github.com/rhnvrm/simples3.git
cd simples3

# Using Nix (recommended)
nix develop

# Or using direnv
direnv allow

# Start local MinIO for testing
just setup

# Run tests
just test-local
```

### Testing

The library includes comprehensive tests that run against a local MinIO instance:

```sh
# Run all tests (without MinIO)
just test

# Run tests with local MinIO
just test-local

# Run specific test
go test -v -run TestList
```

### Available Commands

```sh
just              # List all commands
just test         # Run tests without MinIO
just test-local   # Run tests with MinIO (includes setup)
just setup        # Setup development environment
just minio-up      # Start MinIO container
just minio-down    # Stop MinIO container
just clean         # Clean up everything
just status        # Check development environment status
```

## Environment Variables

For development and testing:

```sh
export AWS_S3_REGION="us-east-1"
export AWS_S3_ACCESS_KEY="minioadmin"
export AWS_S3_SECRET_KEY="minioadmin"
export AWS_S3_ENDPOINT="http://localhost:9000"
export AWS_S3_BUCKET="testbucket"
```

## Contributing

Contributions welcome! Check [ROADMAP.md](ROADMAP.md) for planned features. Please add tests and ensure `just test-local` passes before submitting PRs.

## Author

Rohan Verma <hello@rohanverma.net>

## License

BSD-2-Clause-FreeBSD
