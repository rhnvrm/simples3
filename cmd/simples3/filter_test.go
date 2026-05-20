package main

import "testing"

func TestMatcher(t *testing.T) {
	matcher, err := newMatcher([]string{"*.txt", "docs/*"}, []string{"secret*"})
	if err != nil {
		t.Fatal(err)
	}
	if !matcher.Match("notes.txt") {
		t.Fatalf("expected notes.txt to match")
	}
	if matcher.Match("image.png") {
		t.Fatalf("expected image.png not to match")
	}
	if matcher.Match("secret-plan.txt") {
		t.Fatalf("expected secret-plan.txt to be excluded")
	}
}
