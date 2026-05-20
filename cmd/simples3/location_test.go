package main

import "testing"

func TestParseLocationS3(t *testing.T) {
	loc, err := parseLocation("s3://example-bucket/path/to/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !loc.isS3() {
		t.Fatalf("expected S3 location")
	}
	if loc.bucket != "example-bucket" || loc.key != "path/to/file.txt" {
		t.Fatalf("unexpected parsed location: %+v", loc)
	}
}

func TestParseLocationLocal(t *testing.T) {
	loc, err := parseLocation("./tmp/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !loc.isLocal() {
		t.Fatalf("expected local location")
	}
}

func TestJoinS3Key(t *testing.T) {
	if got := joinS3Key("prefix/", "/child", "file.txt"); got != "prefix/child/file.txt" {
		t.Fatalf("unexpected key: %s", got)
	}
}
