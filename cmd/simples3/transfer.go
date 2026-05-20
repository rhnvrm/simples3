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
	"sync"
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

type executionOptions struct {
	DryRun          bool
	Concurrency     int
	Retries         int
	ContinueOnError bool
}

type copyOptions struct {
	executionOptions
	EmitProgress bool
	ACL          string
	SSE          string
	SSEKMS       string
}

type plannedOperation struct {
	result operationResult
	run    func() error
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
		if options.EmitProgress {
			printLine(rt.stderr, "uploading %s (%d bytes, multipart)", entry.Local, entry.Size)
		}
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

	if options.EmitProgress {
		printLine(rt.stderr, "uploading %s (%d bytes)", entry.Local, entry.Size)
	}
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

type deleteBatch struct {
	entries []sourceEntry
}

func buildDeleteResult(entry sourceEntry, dryRun bool) operationResult {
	result := operationResult{Action: "remove", Source: entrySourceString(entry), Status: "deleted", Size: entry.Size, DryRun: dryRun}
	if dryRun {
		result.Status = "would-delete"
	}
	return result
}

func executeDeleteBatches(client *simples3.S3, entries []sourceEntry, bucket string, options executionOptions) []operationResult {
	if len(entries) == 0 {
		return []operationResult{}
	}
	if options.DryRun {
		results := make([]operationResult, len(entries))
		for i, entry := range entries {
			results[i] = buildDeleteResult(entry, true)
		}
		return results
	}

	batches := makeDeleteBatches(entries)
	concurrency := options.Concurrency
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency == 1 {
		results := make([]operationResult, 0, len(entries))
		for _, batch := range batches {
			batchResults, failed := runDeleteBatch(client, batch, bucket, options.Retries)
			results = append(results, batchResults...)
			if failed && !options.ContinueOnError {
				break
			}
		}
		return results
	}

	type batchResult struct {
		index   int
		results []operationResult
		failed  bool
	}
	results := make([][]operationResult, len(batches))
	started := make([]bool, len(batches))
	resultCh := make(chan batchResult, concurrency)
	var wg sync.WaitGroup
	stopLaunching := false
	next := 0
	inFlight := 0
	launch := func(index int) {
		started[index] = true
		inFlight++
		wg.Add(1)
		go func() {
			defer wg.Done()
			batchResults, failed := runDeleteBatch(client, batches[index], bucket, options.Retries)
			resultCh <- batchResult{index: index, results: batchResults, failed: failed}
		}()
	}
	for next < len(batches) && inFlight < concurrency {
		launch(next)
		next++
	}
	for inFlight > 0 {
		completed := <-resultCh
		results[completed.index] = completed.results
		inFlight--
		if completed.failed && !options.ContinueOnError {
			stopLaunching = true
		}
		for !stopLaunching && next < len(batches) && inFlight < concurrency {
			launch(next)
			next++
		}
	}
	wg.Wait()

	flattened := make([]operationResult, 0, len(entries))
	for i := 0; i < next; i++ {
		if started[i] {
			flattened = append(flattened, results[i]...)
		}
	}
	return flattened
}

func makeDeleteBatches(entries []sourceEntry) []deleteBatch {
	chunks := chunkKeys(entries, 1000)
	batches := make([]deleteBatch, 0, len(chunks))
	for _, chunk := range chunks {
		batches = append(batches, deleteBatch{entries: chunk})
	}
	return batches
}

func runDeleteBatch(client *simples3.S3, batch deleteBatch, bucket string, retries int) ([]operationResult, bool) {
	results := make([]operationResult, len(batch.entries))
	keys := make([]string, 0, len(batch.entries))
	for i, entry := range batch.entries {
		results[i] = buildDeleteResult(entry, false)
		keys = append(keys, entry.Key)
	}

	var output simples3.DeleteObjectsOutput
	if err := retryOperation(retries, func() error {
		var err error
		output, err = client.DeleteObjects(simples3.DeleteObjectsInput{Bucket: bucket, Objects: keys, Quiet: true})
		return err
	}); err != nil {
		for i := range results {
			results[i].Status = "failed"
			results[i].Error = err.Error()
		}
		return results, true
	}

	errorsByKey := make(map[string]simples3.DeleteError, len(output.Errors))
	for _, deleteErr := range output.Errors {
		errorsByKey[deleteErr.Key] = deleteErr
	}
	failed := false
	for i, entry := range batch.entries {
		if deleteErr, ok := errorsByKey[entry.Key]; ok {
			results[i].Status = "failed"
			results[i].Error = formatDeleteError(deleteErr)
			failed = true
		}
	}
	return results, failed
}

func formatDeleteError(deleteErr simples3.DeleteError) string {
	switch {
	case deleteErr.Code != "" && deleteErr.Message != "":
		return deleteErr.Code + ": " + deleteErr.Message
	case deleteErr.Message != "":
		return deleteErr.Message
	case deleteErr.Code != "":
		return deleteErr.Code
	default:
		return "delete failed"
	}
}

func executePlannedOperations(ops []plannedOperation, options executionOptions) []operationResult {
	if len(ops) == 0 {
		return []operationResult{}
	}
	if options.DryRun {
		results := make([]operationResult, len(ops))
		for i, op := range ops {
			results[i] = op.result
		}
		return results
	}
	concurrency := options.Concurrency
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency == 1 {
		results := make([]operationResult, 0, len(ops))
		for _, op := range ops {
			result := runPlannedOperation(op, options)
			results = append(results, result)
			if result.Error != "" && !options.ContinueOnError {
				break
			}
		}
		return results
	}

	type workResult struct {
		index  int
		result operationResult
	}
	results := make([]operationResult, len(ops))
	started := make([]bool, len(ops))
	resultCh := make(chan workResult, concurrency)
	var wg sync.WaitGroup
	stopLaunching := false
	next := 0
	inFlight := 0
	launch := func(index int) {
		started[index] = true
		inFlight++
		wg.Add(1)
		go func() {
			defer wg.Done()
			resultCh <- workResult{index: index, result: runPlannedOperation(ops[index], options)}
		}()
	}
	for next < len(ops) && inFlight < concurrency {
		launch(next)
		next++
	}
	for inFlight > 0 {
		completed := <-resultCh
		results[completed.index] = completed.result
		inFlight--
		if completed.result.Error != "" && !options.ContinueOnError {
			stopLaunching = true
		}
		for !stopLaunching && next < len(ops) && inFlight < concurrency {
			launch(next)
			next++
		}
	}
	wg.Wait()

	attempted := make([]operationResult, 0, next)
	for i := 0; i < next; i++ {
		if started[i] {
			attempted = append(attempted, results[i])
		}
	}
	return attempted
}

func runPlannedOperation(op plannedOperation, options executionOptions) operationResult {
	result := op.result
	if op.run == nil {
		return result
	}
	if err := retryOperation(options.Retries, op.run); err != nil {
		result.Status = "failed"
		result.Error = err.Error()
	}
	return result
}

func retryOperation(retries int, fn func() error) error {
	attempts := retries + 1
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt == attempts || !isRetriableError(err) {
			break
		}
		time.Sleep(time.Duration(attempt*attempt) * 100 * time.Millisecond)
	}
	return lastErr
}

func isRetriableError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, token := range []string{"timeout", "temporary", "connection reset", "connection refused", "unexpected eof", "no such host", "503", "502", "500", "504", "slow down", "internalerror"} {
		if strings.Contains(message, token) {
			return true
		}
	}
	return false
}

func operationsPartialFailure(command string, results []operationResult) error {
	if !hasFailures(results) {
		return nil
	}
	failed := summarizeOperations(results).Failed
	return partialFailureError(fmt.Sprintf("%s completed with %d failed operation(s)", command, failed), buildOperationsData(results))
}
