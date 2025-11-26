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

## v0.10.2 - Code Organization (Refactor)
**Scope**: Internal restructuring, no API changes
**Size**: Medium (refactor ~1064 LOC)

Split simples3.go into logical modules:
- [ ] `simples3.go` - Core (S3 struct, New(), config methods: SetEndpoint/SetToken/SetClient)
- [ ] `iam.go` - IAM (fetchIMDSToken, fetchIAMData, NewUsingIAM, renewIAMToken, SetIAMData)
- [ ] `object.go` - Object ops (FileUpload, FilePut, FileDownload, FileDelete, FileDetails)
- [ ] `list.go` - List ops (List, ListAll)
- [ ] `helpers.go` - Utils (encodePath, detectFileSize, getFirstString, getURL)
- [ ] Ensure all tests pass after refactor

**Why**: 1064-line file hard to navigate. Clean foundation before adding features.

---

## v0.11.0 - Bucket Operations
**Scope**: Essential bucket management for CLI
**Size**: Small (3 APIs, ~200 LOC)

- [ ] ListBuckets
- [ ] CreateBucket (with region support)
- [ ] DeleteBucket

**Why**: These 3 operations unblock CLI development. Minimal but complete.

---

## v0.12.0 - Object Operations (Copy & Batch Delete)
**Scope**: Object manipulation for CLI mv/sync
**Size**: Small (2 APIs, ~150 LOC)

- [ ] CopyObject (within/across buckets)
- [ ] DeleteObjects (batch delete up to 1000 objects)

**Why**: CopyObject enables `mv` command. Batch delete improves performance.

---

## v0.13.0 - Multipart Upload
**Scope**: Large file support
**Size**: Large (5 APIs, ~400 LOC)

- [ ] InitiateMultipartUpload
- [ ] UploadPart (with retry logic)
- [ ] CompleteMultipartUpload
- [ ] AbortMultipartUpload
- [ ] ListParts

**Why**: Critical for large files. Enables resumable uploads.

---

## v0.14.0 - Server-Side Encryption
**Scope**: Security basics
**Size**: Small (~150 LOC)

- [ ] SSE-S3 support (AES256)
- [ ] SSE-KMS support (key ARN)
- [ ] Encryption headers in Put/Upload/Copy

**Why**: Security is table stakes. Simple to add to existing operations.

---

## v0.15.0 - Object Tagging
**Scope**: Metadata management
**Size**: Small (4 APIs, ~200 LOC)

- [ ] PutObjectTagging
- [ ] GetObjectTagging
- [ ] DeleteObjectTagging
- [ ] Tagging support in Put/Upload/Copy

**Why**: Common requirement for organization/billing. Simple XML operations.

---

## v0.16.0 - Object Versioning
**Scope**: Version control
**Size**: Medium (5 APIs, ~300 LOC)

- [ ] PutBucketVersioning
- [ ] GetBucketVersioning
- [ ] ListObjectVersions
- [ ] GetObjectVersion
- [ ] DeleteObjectVersion (with version ID)

**Why**: Important for data safety. More complex due to version handling.

---

## v0.17.0 - ACLs & Lifecycle
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

## v0.18.0 - CLI Complete
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