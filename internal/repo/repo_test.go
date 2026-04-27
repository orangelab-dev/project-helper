package repo

import (
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
)

func TestParseGitHubURL(t *testing.T) {
	parsed, err := ParseGitHubURL("https://github.com/gin-gonic/gin.git")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Normalized != "https://github.com/gin-gonic/gin" || parsed.Owner != "gin-gonic" || parsed.Name != "gin" {
		t.Fatalf("unexpected parsed url: %+v", parsed)
	}
}

func TestSafeRelativePath(t *testing.T) {
	if _, err := SafeRelativePath("../secret"); err == nil {
		t.Fatal("expected traversal error")
	}
	path, err := SafeRelativePath("./internal/app.go")
	if err != nil {
		t.Fatal(err)
	}
	if path != "internal/app.go" {
		t.Fatalf("path = %q", path)
	}
}

func TestRevisionFromRefsSymbolicHEAD(t *testing.T) {
	refs := []*plumbing.Reference{
		plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("develop")),
		plumbing.NewHashReference(plumbing.NewBranchReferenceName("develop"), plumbing.NewHash("0123456789abcdef0123456789abcdef01234567")),
		plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), plumbing.NewHash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")),
	}
	branch, sha, err := revisionFromRefs(refs)
	if err != nil {
		t.Fatal(err)
	}
	if branch != "develop" {
		t.Fatalf("branch = %q", branch)
	}
	if sha != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatalf("sha = %q", sha)
	}
}

func TestRevisionFromRefsFallbackMain(t *testing.T) {
	refs := []*plumbing.Reference{
		plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), plumbing.NewHash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")),
	}
	branch, sha, err := revisionFromRefs(refs)
	if err != nil {
		t.Fatal(err)
	}
	if branch != "main" || sha != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("unexpected revision branch=%q sha=%q", branch, sha)
	}
}
