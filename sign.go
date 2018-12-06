// LICENSE MIT
// Copyright (c) 2018, Rohan Verma <hello@rohanverma.net>
// Copyright (C) 2012 Blake Mizerany
// contains code from: github.com/bmizerany/aws4

package simples3

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (s3 *S3) signKeys(t time.Time) []byte {
	h := ghmac([]byte("AWS4"+s3.SecretKey), []byte(t.Format(shortTimeFormat)))
	h = ghmac(h, []byte(s3.Region))
	h = ghmac(h, []byte(serviceName))
	h = ghmac(h, []byte("aws4_request"))
	return h
}

func (s3 *S3) writeRequest(w io.Writer, r *http.Request) {
	r.Header.Set("host", r.Host)

	w.Write([]byte(r.Method))
	w.Write(lf)
	writeURI(w, r)
	w.Write(lf)
	writeQuery(w, r)
	w.Write(lf)
	writeHeader(w, r)
	w.Write(lf)
	w.Write(lf)
	writeHeaderList(w, r)
	w.Write(lf)
	writeBody(w, r)
}

func (s3 *S3) writeStringToSign(w io.Writer, t time.Time, r *http.Request) {
	w.Write([]byte("AWS4-HMAC-SHA256"))
	w.Write(lf)
	w.Write([]byte(t.Format(amzDateISO8601TimeFormat)))
	w.Write(lf)

	w.Write([]byte(s3.creds(t)))
	w.Write(lf)

	h := sha256.New()
	s3.writeRequest(h, r)
	fmt.Fprintf(w, "%x", h.Sum(nil))
}

func (s3 *S3) creds(t time.Time) string {
	return t.Format(shortTimeFormat) + "/" + s3.Region + "/" + serviceName + "/aws4_request"
}

func writeURI(w io.Writer, r *http.Request) {
	path := r.URL.RequestURI()
	if r.URL.RawQuery != "" {
		path = path[:len(path)-len(r.URL.RawQuery)-1]
	}
	slash := strings.HasSuffix(path, "/")
	path = filepath.Clean(path)
	if path != "/" && slash {
		path += "/"
	}
	w.Write([]byte(path))
}

func writeQuery(w io.Writer, r *http.Request) {
	var a []string
	for k, vs := range r.URL.Query() {
		k = url.QueryEscape(k)
		for _, v := range vs {
			if v == "" {
				a = append(a, k)
			} else {
				v = url.QueryEscape(v)
				a = append(a, k+"="+v)
			}
		}
	}
	sort.Strings(a)
	for i, s := range a {
		if i > 0 {
			w.Write([]byte{'&'})
		}
		w.Write([]byte(s))
	}
}

func writeHeader(w io.Writer, r *http.Request) {
	i, a := 0, make([]string, len(r.Header))
	for k, v := range r.Header {
		sort.Strings(v)
		a[i] = strings.ToLower(k) + ":" + strings.Join(v, ",")
		i++
	}
	sort.Strings(a)
	for i, s := range a {
		if i > 0 {
			w.Write(lf)
		}
		io.WriteString(w, s)
	}
}

func writeHeaderList(w io.Writer, r *http.Request) {
	i, a := 0, make([]string, len(r.Header))
	for k := range r.Header {
		a[i] = strings.ToLower(k)
		i++
	}
	sort.Strings(a)
	for i, s := range a {
		if i > 0 {
			w.Write([]byte{';'})
		}
		w.Write([]byte(s))
	}
}

func writeBody(w io.Writer, r *http.Request) {
	var b []byte
	// If the payload is empty, use the empty string as the input to the SHA256 function
	// http://docs.amazonwebservices.com/general/latest/gr/sigv4-create-canonical-request.html
	if r.Body == nil {
		b = []byte("")
	} else {
		var err error
		b, err = ioutil.ReadAll(r.Body)
		if err != nil {
			panic(err)
		}
		r.Body = ioutil.NopCloser(bytes.NewBuffer(b))
	}

	h := sha256.New()
	h.Write(b)
	fmt.Fprintf(w, "%x", h.Sum(nil))
}

func ghmac(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}
