# simples3 : Simple no frills AWS S3 Library using REST with V4 Signing

## Overview [![GoDoc](https://godoc.org/github.com/rhnvrm/simples3?status.svg)](https://godoc.org/github.com/rhnvrm/simples3) [![Go Report Card](https://goreportcard.com/badge/github.com/rhnvrm/simples3)](https://goreportcard.com/report/github.com/rhnvrm/simples3) [![GoCover](https://gocover.io/_badge/github.com/rhnvrm/simples3)](https://gocover.io/_badge/github.com/rhnvrm/simples3) [![Zerodha Tech](https://zerodha.tech/static/images/github-badge.svg)](https://zerodha.tech)

SimpleS3 is a Go library for manipulating objects
in S3 buckets using REST API calls or Presigned URLs signed
using AWS Signature Version 4.

**Features:**
- **Simple, intuitive API** following Go idioms
- **Complete S3 operations** - Upload, Download, Delete, List, Details
- **Multipart upload** - Large file support with parallel uploads, progress tracking, and resumability
- **Bucket management** - Create, Delete, and List buckets
- **AWS Signature Version 4** signing
- **Custom endpoint support** (MinIO, DigitalOcean Spaces, etc.)
- **Simple List API** with pagination, prefix filtering, and delimiter grouping
- **Iterator-based ListAll** for memory-efficient large bucket iteration (Go 1.23+)
- **Presigned URL generation** for secure browser uploads/downloads (including multipart)
- **Object Versioning** - Enable versioning, list versions, and access specific object versions
- **Server-Side Encryption** - Support for SSE-S3 (AES256) and SSE-KMS encryption
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

### Bucket Operations

#### List All Buckets
```go
// List all buckets for the AWS account
result, err := s3.ListBuckets(simples3.ListBucketsInput{})
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Found %d buckets:\n", len(result.Buckets))
for _, bucket := range result.Buckets {
    fmt.Printf("- %s (created: %s)\n", bucket.Name, bucket.CreationDate)
}

// Owner information is also available
fmt.Printf("Owner: %s (%s)\n", result.Owner.DisplayName, result.Owner.ID)
```

#### Create a Bucket
```go
// Create a new bucket
output, err := s3.CreateBucket(simples3.CreateBucketInput{
    Bucket: "my-new-bucket",
    Region: "us-east-1", // Optional, defaults to client region
})
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Bucket created at: %s\n", output.Location)
```

#### Delete a Bucket
```go
// Delete an empty bucket
err := s3.DeleteBucket(simples3.DeleteBucketInput{
    Bucket: "my-old-bucket",
})
if err != nil {
    log.Fatal(err)
}

// Note: The bucket must be empty before deletion
```

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

#### Copy Objects
```go
// Copy object within same bucket
output, err := s3.CopyObject(simples3.CopyObjectInput{
    SourceBucket: "my-bucket",
    SourceKey:    "original/file.txt",
    DestBucket:   "my-bucket",
    DestKey:      "copied/file.txt",
})

// Copy across buckets
output, err := s3.CopyObject(simples3.CopyObjectInput{
    SourceBucket: "source-bucket",
    SourceKey:    "file.txt",
    DestBucket:   "dest-bucket",
    DestKey:      "file.txt",
})

// Copy with metadata replacement
output, err := s3.CopyObject(simples3.CopyObjectInput{
    SourceBucket:      "my-bucket",
    SourceKey:         "file.txt",
    DestBucket:        "my-bucket",
    DestKey:           "file-copy.txt",
    MetadataDirective: "REPLACE",
    ContentType:       "application/json",
    CustomMetadata:    map[string]string{"version": "2"},
})
```

#### Batch Delete Files
```go
// Delete multiple objects in one request (up to 1000)
output, err := s3.DeleteObjects(simples3.DeleteObjectsInput{
    Bucket:  "my-bucket",
    Objects: []string{"file1.txt", "file2.txt", "file3.txt"},
    Quiet:   false,
})
if err != nil {
    log.Fatal(err)
}

// Check results
for _, deleted := range output.Deleted {
    fmt.Printf("Deleted: %s\n", deleted.Key)
}
for _, errItem := range output.Errors {
    fmt.Printf("Failed to delete %s: %s\n", errItem.Key, errItem.Message)
}
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

### Object Tagging

S3 object tags are key-value pairs that you can use to categorize, organize, and manage objects. Each object can have up to 10 tags.

#### Put Tags on an Object

Set or replace all tags on an existing object:

```go
err := s3.PutObjectTagging(simples3.PutObjectTaggingInput{
    Bucket:    "my-bucket",
    ObjectKey: "my-file.txt",
    Tags: map[string]string{
        "Environment": "production",
        "Project":     "website",
        "Version":     "v1.2.0",
    },
})
if err != nil {
    log.Fatal(err)
}
```

#### Get Tags from an Object

Retrieve all tags from an object:

```go
output, err := s3.GetObjectTagging(simples3.GetObjectTaggingInput{
    Bucket:    "my-bucket",
    ObjectKey: "my-file.txt",
})
if err != nil {
    log.Fatal(err)
}

for key, value := range output.Tags {
    fmt.Printf("%s: %s\n", key, value)
}
```

#### Delete All Tags from an Object

Remove all tags from an object:

```go
err := s3.DeleteObjectTagging(simples3.DeleteObjectTaggingInput{
    Bucket:    "my-bucket",
    ObjectKey: "my-file.txt",
})
if err != nil {
    log.Fatal(err)
}
```

#### Upload with Tags

You can set tags when uploading an object using `FilePut`:

```go
_, err := s3.FilePut(simples3.UploadInput{
    Bucket:      "my-bucket",
    ObjectKey:   "my-file.txt",
    ContentType: "text/plain",
    Body:        file,
    Tags: map[string]string{
        "Environment": "production",
        "Department":  "engineering",
    },
})
```

Note: Tag support during upload varies by operation:
- **FilePut** (PUT): ✅ Full support
- **FileUpload** (POST): ⚠️  AWS S3 supported, MinIO limited (does not support tags in POST policy)
- **CopyObject**: ✅ Full support (handles MinIO/R2 signature quirks automatically)

### Object Versioning

Enable and manage object versions to protect against accidental deletion or overwrite.

#### Enable Versioning

```go
err := s3.PutBucketVersioning(simples3.PutBucketVersioningInput{
    Bucket: "my-bucket",
    Status: "Enabled", // or "Suspended"
})
if err != nil {
    log.Fatal(err)
}

// Check status
config, err := s3.GetBucketVersioning("my-bucket")
fmt.Printf("Versioning status: %s\n", config.Status)
```

#### List Object Versions

List all versions of objects in a bucket:

```go
result, err := s3.ListVersions(simples3.ListVersionsInput{
    Bucket: "my-bucket",
    Prefix: "important-file.txt",
})
if err != nil {
    log.Fatal(err)
}

for _, version := range result.Versions {
    fmt.Printf("Key: %s, VersionId: %s, IsLatest: %v\n",
        version.Key, version.VersionId, version.IsLatest)
}
```

#### Download Specific Version

Retrieve a specific version of an object:

```go
file, err := s3.FileDownload(simples3.DownloadInput{
    Bucket:    "my-bucket",
    ObjectKey: "my-file.txt",
    VersionId: "v1-version-id",
})
if err != nil {
    log.Fatal(err)
}
// Use file (io.ReadCloser) normally
```

#### Delete Specific Version

Delete a specific version of an object:

```go
err := s3.FileDelete(simples3.DeleteInput{
    Bucket:    "my-bucket",
    ObjectKey: "my-file.txt",
    VersionId: "v1-version-id",
})
```

### Server-Side Encryption
 
 Secure your data at rest using Server-Side Encryption (SSE). SimpleS3 supports both SSE-S3 (AES256) and SSE-KMS.
 
 #### Upload with SSE-S3 (AES256)
 
 ```go
 _, err := s3.FilePut(simples3.UploadInput{
     Bucket:               "my-bucket",
     ObjectKey:            "secure-file.txt",
     Body:                 strings.NewReader("secret data"),
     ServerSideEncryption: "AES256",
 })
 ```
 
 #### Upload with SSE-KMS
 
 ```go
 _, err := s3.FilePut(simples3.UploadInput{
     Bucket:               "my-bucket",
     ObjectKey:            "kms-encrypted-file.txt",
     Body:                 strings.NewReader("secret data"),
     ServerSideEncryption: "aws:kms",
     SSEKMSKeyId:          "arn:aws:kms:us-east-1:123456789012:key/your-key-id",
 })
 ```
 
 #### Multipart Upload with Encryption
 
 ```go
 output, err := s3.FileUploadMultipart(simples3.MultipartUploadInput{
     Bucket:               "my-bucket",
     ObjectKey:            "large-secure-file.mp4",
     Body:                 file,
     ServerSideEncryption: "AES256",
 })
 ```
 
 #### Copy with Encryption
 
 ```go
 _, err := s3.CopyObject(simples3.CopyObjectInput{
     SourceBucket:         "my-bucket",
     SourceKey:            "original.txt",
     DestBucket:           "my-bucket",
     DestKey:              "encrypted-copy.txt",
     ServerSideEncryption: "AES256",
 })
 ```
 
 #### Check Encryption Status
 
 ```go
 details, err := s3.FileDetails(simples3.DetailsInput{
     Bucket:    "my-bucket",
     ObjectKey: "secure-file.txt",
 })
 
 fmt.Printf("Encryption: %s\n", details.ServerSideEncryption)
 if details.SSEKMSKeyId != "" {
     fmt.Printf("KMS Key ID: %s\n", details.SSEKMSKeyId)
 }
 ```
 
 ### Multipart Upload

For large files (>100MB), use multipart upload for better performance, resumability, and parallel uploads.

#### High-Level API (Recommended)

The easiest way to upload large files:

```go
file, err := os.Open("large-video.mp4")
if err != nil {
    log.Fatal(err)
}
defer file.Close()

// FileUploadMultipart automatically handles chunking and parallel uploads
output, err := s3.FileUploadMultipart(simples3.MultipartUploadInput{
    Bucket:      "my-bucket",
    ObjectKey:   "videos/large-video.mp4",
    Body:        file,
    ContentType: "video/mp4",
    PartSize:    10 * 1024 * 1024, // 10MB parts (optional, default 5MB)
    Concurrency: 5,                 // Upload 5 parts in parallel (optional, default 1)
    OnProgress: func(info simples3.ProgressInfo) {
        fmt.Printf("\rProgress: %.1f%% (%d/%d parts)",
            float64(info.UploadedBytes)/float64(info.TotalBytes)*100,
            info.CurrentPart, info.TotalParts)
    },
})
if err != nil {
    log.Fatal(err)
}

fmt.Printf("\nUploaded: %s (ETag: %s)\n", output.Key, output.ETag)
```

#### Low-Level API (Advanced)

For more control over the upload process:

```go
// 1. Initiate multipart upload
initOutput, err := s3.InitiateMultipartUpload(simples3.InitiateMultipartUploadInput{
    Bucket:      "my-bucket",
    ObjectKey:   "large-file.bin",
    ContentType: "application/octet-stream",
})
if err != nil {
    log.Fatal(err)
}

// 2. Upload parts (can be done in parallel or resumed later)
var completedParts []simples3.CompletedPart
partSize := int64(10 * 1024 * 1024) // 10MB

for partNum := 1; partNum <= totalParts; partNum++ {
    // Read part data
    partData := getPartData(partNum, partSize) // Your function to get part data

    output, err := s3.UploadPart(simples3.UploadPartInput{
        Bucket:     "my-bucket",
        ObjectKey:  "large-file.bin",
        UploadID:   initOutput.UploadID,
        PartNumber: partNum,
        Body:       bytes.NewReader(partData),
        Size:       int64(len(partData)),
    })
    if err != nil {
        // Abort on error
        s3.AbortMultipartUpload(simples3.AbortMultipartUploadInput{
            Bucket:    "my-bucket",
            ObjectKey: "large-file.bin",
            UploadID:  initOutput.UploadID,
        })
        log.Fatal(err)
    }

    completedParts = append(completedParts, simples3.CompletedPart{
        PartNumber: output.PartNumber,
        ETag:       output.ETag,
    })
}

// 3. Complete the upload
result, err := s3.CompleteMultipartUpload(simples3.CompleteMultipartUploadInput{
    Bucket:    "my-bucket",
    ObjectKey: "large-file.bin",
    UploadID:  initOutput.UploadID,
    Parts:     completedParts,
})
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Upload completed: %s\n", result.ETag)
```

#### List Uploaded Parts

Query which parts have been uploaded (useful for resuming uploads):

```go
output, err := s3.ListParts(simples3.ListPartsInput{
    Bucket:    "my-bucket",
    ObjectKey: "large-file.bin",
    UploadID:  uploadID,
})
if err != nil {
    log.Fatal(err)
}

for _, part := range output.Parts {
    fmt.Printf("Part %d: %d bytes (ETag: %s)\n",
        part.PartNumber, part.Size, part.ETag)
}
```

#### Abort Multipart Upload

Cancel an in-progress upload and clean up parts:

```go
err := s3.AbortMultipartUpload(simples3.AbortMultipartUploadInput{
    Bucket:    "my-bucket",
    ObjectKey: "large-file.bin",
    UploadID:  uploadID,
})
```

#### Browser-Based Multipart Upload

Generate presigned URLs for each part to enable direct browser uploads:

```go
// Backend: Initiate and generate presigned URLs
initOutput, _ := s3.InitiateMultipartUpload(simples3.InitiateMultipartUploadInput{
    Bucket:    "my-bucket",
    ObjectKey: "client-upload.bin",
})

// Generate presigned URL for part 1
presignedURL := s3.GeneratePresignedUploadPartURL(simples3.PresignedMultipartInput{
    Bucket:        "my-bucket",
    ObjectKey:     "client-upload.bin",
    UploadID:      initOutput.UploadID,
    PartNumber:    1,
    ExpirySeconds: 3600,
})

// Frontend: Upload directly to S3 using the presigned URL (via PUT request)
// Then send ETags back to backend for completion
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
