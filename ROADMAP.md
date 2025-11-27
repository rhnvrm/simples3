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

## v0.11.0 - Core Library Features ✅ COMPLETED
**Scope**: Consolidated release of all core library features prior to CLI development
**Released**: Nov 27, 2025

This release consolidates all previously planned feature sets (v0.11.0 - v0.16.0) into a single stable library release.

### Feature Set 1: Bucket & Object Operations
- ListBuckets, CreateBucket (with region), DeleteBucket
- CopyObject (within/across buckets)
- DeleteObjects (batch delete)

### Feature Set 2: Multipart Upload
- Complete multipart upload API (Initiate, UploadPart, Complete, Abort, ListParts)
- High-level `FileUploadMultipart` with automatic chunking and parallel uploads
- Retry logic with exponential backoff and progress tracking
- Presigned URLs for multipart uploads

### Feature Set 3: Server-Side Encryption
- SSE-S3 (AES256) and SSE-KMS support
- Encryption headers in Put/Upload/Copy

### Feature Set 4: Object Tagging
- Put/Get/Delete Object Tagging
- Tagging support in upload/copy operations

### Feature Set 5: Object Versioning
- Put/Get Bucket Versioning
- ListObjectVersions
- Version-aware operations (Download, Delete, ACLs)

### Feature Set 6: ACLs & Lifecycle
- Put/Get Bucket & Object ACLs (Canned & Custom Policies)
- Put/Get/Delete Bucket Lifecycle configuration

**Why**: Consolidating these features provides a complete, feature-rich library foundation for the upcoming CLI tool.

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