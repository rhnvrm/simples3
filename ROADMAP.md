# Project Roadmap

## Current: v0.x (Pre-Stable)

### Completed Features
- File Upload (PUT via FilePut, POST multipart via FileUpload)
- File Download (GetObject)
- File Delete (DeleteObject)
- File Details (HeadObject)
- List Objects (ListObjectsV2) with pagination
- ListAll iterator (Go 1.23+) for memory-efficient iteration
- Presigned URLs (GET/PUT with expiry)
- Upload policies for browser POST uploads
- IAM Role Support (EC2 instances with IMDSv2)
- IAM token auto-renewal
- Custom Metadata
- Custom Endpoints (MinIO, DigitalOcean Spaces, etc.)
- Content-Disposition and ACL support
- Zero dependencies (stdlib only)

## v0.10.2 - Code Organization (Refactor) ✅ COMPLETED
**Scope**: Internal restructuring, no API changes
**Size**: Medium (refactor ~1064 LOC)
**Released**: Nov 26, 2025 ([v0.10.2](https://github.com/rhnvrm/simples3/releases/tag/v0.10.2))

Split simples3.go into logical modules:
- [x] `simples3.go` - Core (S3 struct, New(), config methods: SetEndpoint/SetToken/SetClient)
- [x] `iam.go` - IAM (fetchIMDSToken, fetchIAMData, NewUsingIAM, renewIAMToken, SetIAMData)
- [x] `object.go` - Object ops (FileUpload, FilePut, FileDownload, FileDelete, FileDetails)
- [x] `list.go` - List ops (List, ListAll)
- [x] `helpers.go` - Utils (encodePath, detectFileSize, getFirstString, getURL)
- [x] Ensure all tests pass after refactor

**Why**: 1064-line file hard to navigate. Clean foundation before adding features.

---

## v0.11.0 - Bucket & Object Operations ✅ COMPLETED
**Scope**: Essential bucket management + object manipulation for CLI
**Size**: Medium (5 APIs, ~400 LOC)
**Released**: Nov 26, 2025

Bucket Operations:
- [x] ListBuckets
- [x] CreateBucket (with region support)
- [x] DeleteBucket

Object Operations:
- [x] CopyObject (within/across buckets)
- [x] DeleteObjects (batch delete up to 1000 objects)

**Why**: These operations unblock CLI development (mv, sync, batch operations). Combined release for efficiency.

**Note**: DeleteObjects has a known compatibility issue with MinIO that requires further investigation. The implementation follows AWS S3 specification correctly.

---

## v0.12.0 - Multipart Upload ✅ COMPLETED
**Scope**: Large file support with parallel uploads, progress tracking, and presigned URLs
**Size**: Large (10 APIs/features, ~800 LOC)
**Released**: Nov 27, 2025

Core APIs:
- [x] InitiateMultipartUpload
- [x] UploadPart
- [x] CompleteMultipartUpload
- [x] AbortMultipartUpload
- [x] ListParts

High-Level Features:
- [x] FileUploadMultipart (automatic chunking)
- [x] uploadPartWithRetry (exponential backoff retry logic)
- [x] uploadPartsParallel (concurrent part uploads with worker pool)
- [x] Progress callbacks (real-time upload progress)
- [x] GeneratePresignedUploadPartURL (browser-based multipart uploads)

Advanced Features Implemented:
- [x] Automatic file chunking with configurable part sizes (5MB-5GB)
- [x] Parallel upload worker pool (configurable concurrency)
- [x] Retry logic with exponential backoff (3 retries default, configurable)
- [x] Real-time progress tracking with upload speed calculation
- [x] Automatic cleanup on errors (AbortMultipartUpload)
- [x] Support for resumable uploads via ListParts
- [x] Comprehensive input validation (part size, part numbers, etc.)
- [x] Integration tests with MinIO (500+ LOC)
- [x] File integrity verification across all upload modes

**Why**: Critical for large files (>100MB). Enables resumable uploads, parallel transfers, and better performance.

---

## v0.13.0 - Server-Side Encryption
**Scope**: Security basics
**Size**: Small (~150 LOC)

- [ ] SSE-S3 support (AES256)
- [ ] SSE-KMS support (key ARN)
- [ ] Encryption headers in Put/Upload/Copy

**Why**: Security is table stakes. Simple to add to existing operations.

---

## v0.14.0 - Object Tagging ✅ COMPLETED
**Scope**: Metadata management
**Size**: Small (4 APIs, ~250 LOC)
**Released**: Nov 27, 2025

Core APIs:
- [x] PutObjectTagging
- [x] GetObjectTagging
- [x] DeleteObjectTagging

Tagging Support in Operations:
- [x] FilePut (PUT upload)
- [x] FileUpload (POST upload) - AWS S3 only, MinIO limited
- [x] CopyObject - AWS S3 supported, MinIO/R2 handled automatically

Features Implemented:
- [x] Put/get/delete tags on existing objects (up to 10 tags per object)
- [x] Set tags during object upload operations
- [x] Tags field in UploadInput and CopyObjectInput
- [x] URL-encoded tag header formatting
- [x] Input validation (max 10 tags, required fields)
- [x] Integration tests with MinIO (and workaround for signature issues)
- [x] Comprehensive documentation in README

**Why**: Common requirement for organization/billing. Simple XML operations.

**Note**: Some MinIO limitations exist for tagging via POST uploads. CopyObject tagging is handled via a 2-step process (copy then tag) to support both AWS and MinIO/R2.

---

## v0.15.0 - Object Versioning
**Scope**: Version control
**Size**: Medium (5 APIs, ~300 LOC)

- [ ] PutBucketVersioning
- [ ] GetBucketVersioning
- [ ] ListObjectVersions
- [ ] GetObjectVersion
- [ ] DeleteObjectVersion (with version ID)

**Why**: Important for data safety. More complex due to version handling.

---

## v0.16.0 - ACLs & Lifecycle
**Scope**: Advanced management
**Size**: Medium (9 APIs, ~500 LOC)

ACLs:
- [ ] PutObjectAcl
- [ ] GetObjectAcl
- [ ] PutBucketAcl
- [ ] GetBucketAcl

Lifecycle:
- [ ] PutBucketLifecycle
- [ ] GetBucketLifecycle
- [ ] DeleteBucketLifecycle

**Why**: Grouped advanced features. Completes library API surface.

---

## v0.17.0 - CLI Complete
**Scope**: Full-featured CLI with all library features
**Size**: Large (~2000 LOC)

CLI (`cmd/simples3/`):
- [ ] Project structure (cobra or custom parser)
- [ ] S3 URI parsing (`s3://bucket/key`)
- [ ] Config (env vars: AWS_*, flags: --region/--profile)
- [ ] AWS credentials file support (~/.aws/credentials)
- [ ] Basic commands:
  - [ ] ls (list buckets/objects)
  - [ ] cp (upload/download, with multipart for large files)
  - [ ] rm (delete single/batch)
  - [ ] mb (make bucket)
  - [ ] rb (remove bucket)
  - [ ] mv (move/rename using copy+delete)
  - [ ] presign (generate URL)
  - [ ] sync (local ↔ S3 with diff algorithm)
- [ ] Flags:
  - [ ] --recursive (cp/rm)
  - [ ] --sse/--sse-kms-key-id (encryption)
  - [ ] --tags (tagging)
  - [ ] --version-id (versioning)
  - [ ] --acl (canned ACLs)
  - [ ] --delete (sync)
  - [ ] --exclude/--include (patterns)
  - [ ] --json (output mode)
  - [ ] --dry-run
- [ ] Features:
  - [ ] Progress indicators
  - [ ] Parallel transfers
  - [ ] Better error messages
- [ ] Subcommands:
  - [ ] acl get/set
  - [ ] lifecycle get/set/delete
  - [ ] tags get/set/delete

**Why**: Library complete. Build full-featured CLI supporting all operations.

---

## v1.0.0 - Stable Release
**Focus**: Production-ready stability
**Size**: Documentation, tests, polish

Library:
- [ ] All tests passing with 80%+ coverage
- [ ] API documentation complete
- [ ] Performance benchmarks
- [ ] Security audit of signing/crypto code

CLI:
- [ ] CLI tests with real/mocked S3
- [ ] CLI documentation (man pages, --help)
- [ ] Installation guides (brew, apt, etc.)
- [ ] Shell completions (bash, zsh, fish)

Release:
- [ ] CHANGELOG.md complete
- [ ] Migration guide from v0.x
- [ ] Examples for all features
- [ ] API stability guarantee

**Why**: v1.0 = stability commitment. Library + CLI production-ready.