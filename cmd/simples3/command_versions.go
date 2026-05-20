package main

import (
	"fmt"

	"github.com/rhnvrm/simples3"
)

type versionItem struct {
	Key          string `json:"key"`
	VersionID    string `json:"versionId"`
	IsLatest     bool   `json:"isLatest"`
	LastModified string `json:"lastModified"`
	Size         int64  `json:"size"`
	StorageClass string `json:"storageClass,omitempty"`
	OwnerID      string `json:"ownerId,omitempty"`
	OwnerName    string `json:"ownerName,omitempty"`
}

type deleteMarkerItem struct {
	Key          string `json:"key"`
	VersionID    string `json:"versionId"`
	IsLatest     bool   `json:"isLatest"`
	LastModified string `json:"lastModified"`
	OwnerID      string `json:"ownerId,omitempty"`
	OwnerName    string `json:"ownerName,omitempty"`
}

type versionsResult struct {
	Command             string             `json:"command"`
	OK                  bool               `json:"ok"`
	Bucket              string             `json:"bucket"`
	Prefix              string             `json:"prefix,omitempty"`
	Delimiter           string             `json:"delimiter,omitempty"`
	MaxKeys             int64              `json:"maxKeys,omitempty"`
	IsTruncated         bool               `json:"isTruncated"`
	NextKeyMarker       string             `json:"nextKeyMarker,omitempty"`
	NextVersionIDMarker string             `json:"nextVersionIdMarker,omitempty"`
	CommonPrefixes      []string           `json:"commonPrefixes,omitempty"`
	Versions            []versionItem      `json:"versions,omitempty"`
	DeleteMarkers       []deleteMarkerItem `json:"deleteMarkers,omitempty"`
}

func (rt *runtime) runVersions(args []string) error {
	var flags awsFlags
	var delimiter string
	var maxKeys int64
	var keyMarker string
	var versionIDMarker string
	fs := newFlagSet("versions", func() {
		fmt.Fprint(rt.stderr, "Usage: simples3 versions [--profile PROFILE] [--region REGION] [--endpoint URL] [--json] [--delimiter /] [--max-keys N] [--key-marker KEY] [--version-id-marker VID] s3://bucket[/prefix]\n")
	})
	addAWSFlags(fs, &flags)
	fs.StringVar(&delimiter, "delimiter", "", "delimiter to group keys")
	fs.Int64Var(&maxKeys, "max-keys", 0, "maximum versions to return")
	fs.StringVar(&keyMarker, "key-marker", "", "key marker for pagination")
	fs.StringVar(&versionIDMarker, "version-id-marker", "", "version ID marker for pagination")
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErrorf("versions requires exactly one S3 bucket or prefix URI")
	}
	loc, err := parseLocation(fs.Arg(0))
	if err != nil {
		return err
	}
	if !loc.isS3() {
		return usageErrorf("versions target must be an S3 URI like s3://bucket or s3://bucket/prefix")
	}
	settings, err := rt.resolveAWSSettings(flags)
	if err != nil {
		return err
	}
	prefix := loc.key
	if loc.hasTrailing {
		prefix = loc.s3Prefix()
	}
	output, err := settings.newClient().ListVersions(simples3.ListVersionsInput{
		Bucket:          loc.bucket,
		Prefix:          prefix,
		Delimiter:       delimiter,
		MaxKeys:         maxKeys,
		KeyMarker:       keyMarker,
		VersionIdMarker: versionIDMarker,
	})
	if err != nil {
		return err
	}
	result := versionsResult{
		Command:             "versions",
		OK:                  true,
		Bucket:              loc.bucket,
		Prefix:              output.Prefix,
		Delimiter:           output.Delimiter,
		MaxKeys:             output.MaxKeys,
		IsTruncated:         output.IsTruncated,
		NextKeyMarker:       output.NextKeyMarker,
		NextVersionIDMarker: output.NextVersionIdMarker,
		CommonPrefixes:      output.CommonPrefixes,
		Versions:            make([]versionItem, 0, len(output.Versions)),
		DeleteMarkers:       make([]deleteMarkerItem, 0, len(output.DeleteMarkers)),
	}
	for _, version := range output.Versions {
		result.Versions = append(result.Versions, versionItem{
			Key:          version.Key,
			VersionID:    version.VersionId,
			IsLatest:     version.IsLatest,
			LastModified: version.LastModified,
			Size:         version.Size,
			StorageClass: version.StorageClass,
			OwnerID:      version.Owner.ID,
			OwnerName:    version.Owner.DisplayName,
		})
	}
	for _, marker := range output.DeleteMarkers {
		result.DeleteMarkers = append(result.DeleteMarkers, deleteMarkerItem{
			Key:          marker.Key,
			VersionID:    marker.VersionId,
			IsLatest:     marker.IsLatest,
			LastModified: marker.LastModified,
			OwnerID:      marker.Owner.ID,
			OwnerName:    marker.Owner.DisplayName,
		})
	}
	if flags.json {
		return writeJSON(rt.stdout, result)
	}
	for _, prefix := range result.CommonPrefixes {
		printLine(rt.stdout, "prefix %s", prefix)
	}
	for _, version := range result.Versions {
		printLine(rt.stdout, "version %s version=%s latest=%t size=%d", version.Key, version.VersionID, version.IsLatest, version.Size)
	}
	for _, marker := range result.DeleteMarkers {
		printLine(rt.stdout, "delete-marker %s version=%s latest=%t", marker.Key, marker.VersionID, marker.IsLatest)
	}
	return nil
}
