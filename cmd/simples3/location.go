package main

import (
	"net/url"
	"path"
	"path/filepath"
	"strings"
)

type locationKind string

const (
	locationKindLocal locationKind = "local"
	locationKindS3    locationKind = "s3"
)

type location struct {
	kind        locationKind
	raw         string
	path        string
	bucket      string
	key         string
	hasTrailing bool
}

func parseLocation(raw string) (location, error) {
	if strings.TrimSpace(raw) == "" {
		return location{}, usageErrorf("empty path")
	}
	if strings.HasPrefix(raw, "s3://") {
		parsed, err := url.Parse(raw)
		if err != nil {
			return location{}, usageErrorf("invalid s3 location %q: %v", raw, err)
		}
		bucket := parsed.Host
		if bucket == "" {
			return location{}, usageErrorf("missing bucket in %q", raw)
		}
		key := strings.TrimPrefix(parsed.EscapedPath(), "/")
		decodedKey, err := url.PathUnescape(key)
		if err != nil {
			return location{}, usageErrorf("invalid s3 key in %q: %v", raw, err)
		}
		return location{
			kind:        locationKindS3,
			raw:         raw,
			bucket:      bucket,
			key:         decodedKey,
			hasTrailing: strings.HasSuffix(raw, "/") || decodedKey == "",
		}, nil
	}
	cleaned := filepath.Clean(raw)
	return location{
		kind:        locationKindLocal,
		raw:         raw,
		path:        cleaned,
		hasTrailing: strings.HasSuffix(raw, string(filepath.Separator)),
	}, nil
}

func (loc location) String() string {
	if loc.kind == locationKindS3 {
		if loc.key == "" {
			return "s3://" + loc.bucket
		}
		return "s3://" + loc.bucket + "/" + loc.key
	}
	return loc.path
}

func (loc location) isS3() bool {
	return loc.kind == locationKindS3
}

func (loc location) isLocal() bool {
	return loc.kind == locationKindLocal
}

func (loc location) s3Prefix() string {
	if loc.key == "" {
		return ""
	}
	if loc.hasTrailing {
		return strings.TrimSuffix(loc.key, "/") + "/"
	}
	return loc.key
}

func joinS3Key(parts ...string) string {
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		clean = append(clean, strings.Trim(part, "/"))
	}
	if len(clean) == 0 {
		return ""
	}
	return path.Join(clean...)
}
