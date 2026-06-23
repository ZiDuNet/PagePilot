package main

import (
	"testing"

	"github.com/yourorg/hostctl/internal/api"
)

func TestDeriveSiteTitleFromHtmlTitle(t *testing.T) {
	files := []api.DeployFile{
		{Path: "index.html", Content: "<!doctype html><html><head><title>My Landing</title></head><body></body></html>"},
	}
	if got := deriveSiteTitle(files, "index.html"); got != "My Landing" {
		t.Fatalf("deriveSiteTitle() = %q, want %q", got, "My Landing")
	}
}

func TestDeriveSiteTitleRejectsFilenameLikeTitle(t *testing.T) {
	files := []api.DeployFile{
		{Path: "index.html", Content: "<!doctype html><html><head><title>index.html</title></head><body></body></html>"},
	}
	if got := deriveSiteTitle(files, "index.html"); got != "" {
		t.Fatalf("deriveSiteTitle() = %q, want empty title for filename-like value", got)
	}
}
