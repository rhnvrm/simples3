// LICENSE BSD-2-Clause-FreeBSD
// Copyright (c) 2018, Rohan Verma <hello@rohanverma.net>

package simples3

import (
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"
)

// getURL constructs a URL for a given path, with multiple optional
// arguments as individual subfolders, based on the endpoint
// specified in s3 struct.
func (s3 *S3) getURL(path string, args ...string) (uri string) {
	if len(args) > 0 {
		path += "/" + strings.Join(args, "/")
	}
	// need to encode special characters in the path part of the URL
	encodedPath := encodePath(path)

	if len(s3.Endpoint) > 0 {
		uri = s3.Endpoint + "/" + encodedPath
	} else {
		uri = fmt.Sprintf(s3.URIFormat, s3.Region, encodedPath)
	}

	return uri
}

func detectFileSize(body io.Seeker) (int64, error) {
	pos, err := body.Seek(0, 1)
	if err != nil {
		return -1, err
	}
	defer body.Seek(pos, 0)

	n, err := body.Seek(0, 2) //nolint:gomnd
	if err != nil {
		return -1, err
	}
	return n, nil
}

func getFirstString(s []string) string {
	if len(s) > 0 {
		return s[0]
	}

	return ""
}

// if object matches reserved string, no need to encode them.
var reservedObjectNames = regexp.MustCompile("^[a-zA-Z0-9-_.~/]+$")

// encodePath encode the strings from UTF-8 byte representations to HTML hex escape sequences
//
// This is necessary since regular url.Parse() and url.Encode() functions do not support UTF-8
// non english characters cannot be parsed due to the nature in which url.Encode() is written
//
// This function on the other hand is a direct replacement for url.Encode() technique to support
// pretty much every UTF-8 character.
// adapted from
// https://github.com/minio/minio-go/blob/fe1f3855b146c1b6ce4199740d317e44cf9e85c2/pkg/s3utils/utils.go#L285
func encodePath(pathName string) string {
	if reservedObjectNames.MatchString(pathName) {
		return pathName
	}
	var encodedPathname strings.Builder
	for _, s := range pathName {
		if 'A' <= s && s <= 'Z' || 'a' <= s && s <= 'z' || '0' <= s && s <= '9' { // §2.3 Unreserved characters (mark)
			encodedPathname.WriteRune(s)
			continue
		}
		switch s {
		case '-', '_', '.', '~', '/': // §2.3 Unreserved characters (mark)
			encodedPathname.WriteRune(s)
			continue
		default:
			lenR := utf8.RuneLen(s)
			if lenR < 0 {
				// if utf8 cannot convert, return the same string as is
				return pathName
			}
			u := make([]byte, lenR)
			utf8.EncodeRune(u, s)
			for _, r := range u {
				hex := hex.EncodeToString([]byte{r})
				encodedPathname.WriteString("%" + strings.ToUpper(hex))
			}
		}
	}
	return encodedPathname.String()
}
