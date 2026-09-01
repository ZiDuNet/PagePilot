package deploy

import (
	"path/filepath"
	"testing"
)

func TestSafeJoinAllowsDotSequencesInFileNames(t *testing.T) {
	base := t.TempDir()
	got, err := safeJoin(base, "assets/foo..bar.js")
	if err != nil {
		t.Fatalf("safeJoin returned error for valid filename: %v", err)
	}
	want := filepath.Join(base, "assets", "foo..bar.js")
	if got != want {
		t.Fatalf("safeJoin = %q, want %q", got, want)
	}
}

func TestSafeJoinRejectsTraversalSegments(t *testing.T) {
	base := t.TempDir()
	for _, rel := range []string{"../outside.txt", "assets/../../outside.txt", "/absolute.txt", "assets\\..\\outside.txt"} {
		if got, err := safeJoin(base, rel); err == nil || got != "" {
			t.Fatalf("safeJoin(%q) = (%q, %v), want traversal rejection", rel, got, err)
		}
	}
}
