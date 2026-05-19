package main

import (
	"fmt"
	gopath "path"
	"path/filepath"
	"strings"
)

type matcher struct {
	includes []string
	excludes []string
}

func newMatcher(includes, excludes []string) (matcher, error) {
	for _, pattern := range append(append([]string{}, includes...), excludes...) {
		if _, err := gopath.Match(pattern, "x"); err != nil {
			return matcher{}, fmt.Errorf("invalid pattern %q: %w", pattern, err)
		}
	}
	return matcher{includes: includes, excludes: excludes}, nil
}

func (m matcher) Match(name string) bool {
	normalized := normalizeMatchPath(name)
	if len(m.includes) > 0 && !matchesAny(m.includes, normalized) {
		return false
	}
	if matchesAny(m.excludes, normalized) {
		return false
	}
	return true
}

func matchesAny(patterns []string, name string) bool {
	for _, pattern := range patterns {
		matched, err := gopath.Match(pattern, name)
		if err == nil && matched {
			return true
		}
	}
	return false
}

func normalizeMatchPath(name string) string {
	name = filepath.ToSlash(name)
	name = strings.TrimPrefix(name, "./")
	return strings.TrimPrefix(name, "/")
}
