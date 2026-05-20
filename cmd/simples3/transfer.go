package main

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rhnvrm/simples3"
)

const (
	multipartThreshold int64 = 64 * 1024 * 1024
	multipartPartSize  int64 = 16 * 1024 * 1024
)

func normalizeSSE(sse, sseKMS string) string {
	if sse == "" && sseKMS != "" {
		return "aws:kms"
	}
	return sse
}

type sourceEntry struct {
	Kind     locationKind
	Local    string
	Bucket   string
	Key      string
	Version  string
	Relative string
	Size     int64
	ModTime  time.Time
}

type targetRef struct {
	Kind   locationKind
	Local  string
	Bucket string
	Key    string
}

type operationResult struct {
	Action      string `json:"action"`
	Source      string `json:"source,omitempty"`
	Destination string `json:"destination,omitempty"`
	Status      string `json:"status"`
	Size        int64  `json:"size,omitempty"`
	DryRun      bool   `json:"dryRun,omitempty"`
	Error       string `json:"error,omitempty"`
}

type targetMeta struct {
	Exists  bool
	Size    int64
	ModTime time.Time
}

type copyOptions struct {
	DryRun bool
	ACL    string
	SSE    string
	SSEKMS string
}

func collectSourceEntries(client *simples3.S3, source location, recursive bool, matcher matcher, versionID string) ([]sourceEntry, error) {
	if source.isLocal() {
		return collectLocalEntries(source, recursive, matcher)
	}
	return collectS3Entries(client, source, recursive, matcher, versionID)
}

func collectLocalEntries(source location, recursive bool, matcher matcher) ([]sourceEntry, error) {
	info, err := os.Stat(source.path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		base := filepath.Base(source.path)
		if !matcher.Match(base) {
			return nil, nil
		}
		return []sourceEntry{{
			Kind:     locationKindLocal,
			Local:    source.path,
			Relative: filepath.ToSlash(base),
			Size:     info.Size(),
			ModTime:  info.ModTime(),
		}}, nil
	}
	if !recursive {
		return nil, usageErrorf("%s is a directory; use --recursive", source.path)
	}

	entries := []sourceEntry{}
	err = filepath.WalkDir(source.path, func(current string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(source.path, current)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !matcher.Match(rel) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		entries = append(entries, sourceEntry{
			Kind:     locationKindLocal,
			Local:    current,
			Relative: rel,
			Size:     info.Size(),
			ModTime:  info.ModTime(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

func collectS3Entries(client *simples3.S3, source location, recursive bool, matcher matcher, versionID string) ([]sourceEntry, error) {
	prefixMode := recursive || source.key == ""
	if prefixMode {
		if versionID != "" {
			return nil, usageErrorf("--version-id is only supported for a single source object")
		}
		prefix := s3TraversalPrefix(source, recursive)
		entries := []sourceEntry{}
		seq, finish := client.ListAll(simples3.ListInput{Bucket: source.bucket, Prefix: prefix})
		for object := range seq {
			rel := strings.TrimPrefix(object.Key, prefix)
			if rel == "" {
				rel = path.Base(object.Key)
			}
			if !matcher.Match(rel) {
				continue
			}
			entries = append(entries, sourceEntry{
				Kind:     locationKindS3,
				Bucket:   source.bucket,
				Key:      object.Key,
				Relative: rel,
				Size:     object.Size,
				ModTime:  parseS3ListTime(object.LastModified),
			})
		}
		if err := finish(); err != nil {
			return nil, err
		}
		return entries, nil
	}

	details, err := client.FileDetails(simples3.DetailsInput{Bucket: source.bucket, ObjectKey: source.key, VersionId: versionID})
	if err != nil {
		return nil, err
	}
	base := path.Base(source.key)
	if !matcher.Match(base) {
		return nil, nil
	}
	size, _ := strconv.ParseInt(details.ContentLength, 10, 64)
	return []sourceEntry{{
		Kind:     locationKindS3,
		Bucket:   source.bucket,
		Key:      source.key,
		Version:  versionID,
		Relative: base,
		Size:     size,
		ModTime:  parseHTTPTime(details.LastModified),
	}}, nil
}

func s3TraversalPrefix(source location, recursive bool) string {
	if source.key == "" {
		return ""
	}
	if recursive {
		return strings.TrimSuffix(source.key, "/") + "/"
	}
	return source.s3Prefix()
}

func validateRecursiveSource(source location, recursive bool) error {
	if source.isS3() && !recursive && (source.key == "" || source.hasTrailing) {
		return usageErrorf("%s looks like an S3 prefix; use --recursive", source.String())
	}
	return nil
}

func ensureSingleSourceEntry(source location, recursive bool, entries []sourceEntry) error {
	if !recursive && len(entries) > 1 {
		return usageErrorf("%s expands to multiple objects; use --recursive", source.String())
	}
	return nil
}

func parseS3ListTime(value string) time.Time {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return t
}

func parseHTTPTime(value string) time.Time {
	t, err := time.Parse(http.TimeFormat, value)
	if err != nil {
		return time.Time{}
	}
	return t
}

func resolveTarget(entry sourceEntry, dest location, recursive bool) (targetRef, error) {
	if dest.isS3() {
		targetKey := dest.key
		if recursive || dest.key == "" || dest.hasTrailing {
			targetKey = joinS3Key(dest.key, entry.Relative)
		}
		return targetRef{Kind: locationKindS3, Bucket: dest.bucket, Key: targetKey}, nil
	}

	targetPath := dest.path
	if recursive {
		targetPath = filepath.Join(dest.path, filepath.FromSlash(entry.Relative))
		return targetRef{Kind: locationKindLocal, Local: targetPath}, nil
	}
	if dest.hasTrailing || localPathIsDir(dest.path) {
		targetPath = filepath.Join(dest.path, filepath.Base(filepath.FromSlash(entry.Relative)))
	}
	return targetRef{Kind: locationKindLocal, Local: targetPath}, nil
}

func localPathIsDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func entrySourceString(entry sourceEntry) string {
	if entry.Kind == locationKindLocal {
		return entry.Local
	}
	if entry.Key == "" {
		return "s3://" + entry.Bucket
	}
	return "s3://" + entry.Bucket + "/" + entry.Key
}

func targetString(target targetRef) string {
	if target.Kind == locationKindLocal {
		return target.Local
	}
	if target.Key == "" {
		return "s3://" + target.Bucket
	}
	return "s3://" + target.Bucket + "/" + target.Key
}

func detectContentTypeFromPath(path string) string {
	contentType := mime.TypeByExtension(filepath.Ext(path))
	if contentType == "" {
		return "application/octet-stream"
	}
	return contentType
}

func copyEntry(client *simples3.S3, entry sourceEntry, target targetRef, options copyOptions, rt *runtime) error {
	if options.DryRun {
		return nil
	}
	switch {
	case entry.Kind == locationKindLocal && target.Kind == locationKindS3:
		return uploadLocalToS3(client, entry, target, options, rt)
	case entry.Kind == locationKindS3 && target.Kind == locationKindLocal:
		return downloadS3ToLocal(client, entry, target)
	case entry.Kind == locationKindS3 && target.Kind == locationKindS3:
		return copyS3ToS3(client, entry, target, options)
	default:
		return fmt.Errorf("unsupported transfer: %s -> %s", entrySourceString(entry), targetString(target))
	}
}

func uploadLocalToS3(client *simples3.S3, entry sourceEntry, target targetRef, options copyOptions, rt *runtime) error {
	file, err := os.Open(entry.Local)
	if err != nil {
		return err
	}
	defer file.Close()

	if entry.Size >= multipartThreshold {
		printLine(rt.stderr, "uploading %s (%d bytes, multipart)", entry.Local, entry.Size)
		_, err = client.FileUploadMultipart(simples3.MultipartUploadInput{
			Bucket:               target.Bucket,
			ObjectKey:            target.Key,
			Body:                 file,
			ContentType:          detectContentTypeFromPath(entry.Local),
			ACL:                  options.ACL,
			PartSize:             multipartPartSize,
			Concurrency:          4,
			ServerSideEncryption: options.SSE,
			SSEKMSKeyId:          options.SSEKMS,
		})
		return err
	}

	printLine(rt.stderr, "uploading %s (%d bytes)", entry.Local, entry.Size)
	_, err = client.FilePut(simples3.UploadInput{
		Bucket:               target.Bucket,
		ObjectKey:            target.Key,
		ContentType:          detectContentTypeFromPath(entry.Local),
		Body:                 file,
		ACL:                  options.ACL,
		ServerSideEncryption: options.SSE,
		SSEKMSKeyId:          options.SSEKMS,
	})
	return err
}

func downloadS3ToLocal(client *simples3.S3, entry sourceEntry, target targetRef) error {
	body, err := client.FileDownload(simples3.DownloadInput{Bucket: entry.Bucket, ObjectKey: entry.Key, VersionId: entry.Version})
	if err != nil {
		return err
	}
	defer body.Close()

	if err := os.MkdirAll(filepath.Dir(target.Local), 0o755); err != nil {
		return err
	}
	tempFile, err := os.CreateTemp(filepath.Dir(target.Local), ".simples3-download-*")
	if err != nil {
		return err
	}
	tempName := tempFile.Name()
	defer os.Remove(tempName)

	if _, err := io.Copy(tempFile, body); err != nil {
		tempFile.Close()
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, target.Local); err != nil {
		return err
	}
	if !entry.ModTime.IsZero() {
		_ = os.Chtimes(target.Local, time.Now(), entry.ModTime)
	}
	return nil
}

func copyS3ToS3(client *simples3.S3, entry sourceEntry, target targetRef, options copyOptions) error {
	_, err := client.CopyObject(simples3.CopyObjectInput{
		SourceBucket:         entry.Bucket,
		SourceKey:            entry.Key,
		DestBucket:           target.Bucket,
		DestKey:              target.Key,
		ServerSideEncryption: options.SSE,
		SSEKMSKeyId:          options.SSEKMS,
		MetadataDirective:    "COPY",
		ContentType:          "",
		CustomMetadata:       nil,
	})
	return err
}

func deleteSourceEntry(client *simples3.S3, entry sourceEntry, root location) error {
	if entry.Kind == locationKindLocal {
		if err := os.Remove(entry.Local); err != nil {
			return err
		}
		if root.isLocal() && root.path != entry.Local {
			removeEmptyParents(filepath.Dir(entry.Local), root.path)
		}
		return nil
	}
	return client.FileDelete(simples3.DeleteInput{Bucket: entry.Bucket, ObjectKey: entry.Key, VersionId: entry.Version})
}

func deleteTargetRef(client *simples3.S3, target targetRef) error {
	if target.Kind == locationKindLocal {
		return os.Remove(target.Local)
	}
	return client.FileDelete(simples3.DeleteInput{Bucket: target.Bucket, ObjectKey: target.Key})
}

func removeEmptyParents(startDir, stopDir string) {
	current := startDir
	stopDir = filepath.Clean(stopDir)
	for {
		if current == "." || current == string(filepath.Separator) {
			return
		}
		if err := os.Remove(current); err != nil {
			return
		}
		if filepath.Clean(current) == stopDir {
			return
		}
		next := filepath.Dir(current)
		if next == current {
			return
		}
		current = next
	}
}

func fetchTargetMeta(client *simples3.S3, target targetRef) (targetMeta, error) {
	if target.Kind == locationKindLocal {
		info, err := os.Stat(target.Local)
		if err != nil {
			if os.IsNotExist(err) {
				return targetMeta{}, nil
			}
			return targetMeta{}, err
		}
		if info.IsDir() {
			return targetMeta{}, fmt.Errorf("destination %s is a directory", target.Local)
		}
		return targetMeta{Exists: true, Size: info.Size(), ModTime: info.ModTime()}, nil
	}

	details, err := client.FileDetails(simples3.DetailsInput{Bucket: target.Bucket, ObjectKey: target.Key})
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(strings.ToLower(err.Error()), "not found") {
			return targetMeta{}, nil
		}
		return targetMeta{}, err
	}
	size, _ := strconv.ParseInt(details.ContentLength, 10, 64)
	return targetMeta{Exists: true, Size: size, ModTime: parseHTTPTime(details.LastModified)}, nil
}

func needsSync(entry sourceEntry, target targetRef, meta targetMeta) bool {
	if !meta.Exists {
		return true
	}
	if entry.Size != meta.Size {
		return true
	}
	if !entry.ModTime.IsZero() && !meta.ModTime.IsZero() {
		return !sameModTime(entry.ModTime, meta.ModTime)
	}
	return false
}

func sameModTime(a, b time.Time) bool {
	return a.UTC().Truncate(time.Second).Equal(b.UTC().Truncate(time.Second))
}

func collectTargetEntries(client *simples3.S3, target location, recursive bool, matcher matcher) ([]sourceEntry, error) {
	if target.isLocal() {
		if _, err := os.Stat(target.path); err != nil {
			if os.IsNotExist(err) {
				return nil, nil
			}
			return nil, err
		}
	}
	return collectSourceEntries(client, target, recursive, matcher, "")
}

func chunkKeys(entries []sourceEntry, size int) [][]sourceEntry {
	if len(entries) == 0 {
		return nil
	}
	chunks := make([][]sourceEntry, 0, (len(entries)+size-1)/size)
	for start := 0; start < len(entries); start += size {
		end := start + size
		if end > len(entries) {
			end = len(entries)
		}
		chunks = append(chunks, entries[start:end])
	}
	return chunks
}
