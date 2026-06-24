package web

import (
	"io/fs"
	"strings"
	"testing"
)

func TestAdminHTMLUsesReactAppShell(t *testing.T) {
	html := string(AdminHTML())

	for _, want := range []string{`id="root"`, `/admin/assets/index.js`, `/admin/assets/index.css`} {
		if !strings.Contains(html, want) {
			t.Fatalf("admin html missing %q", want)
		}
	}
}

func TestAdminAppAssetsAreEmbedded(t *testing.T) {
	sub := AdminAppFS()
	for _, path := range []string{"index.html", "assets/index.js", "assets/index.css"} {
		if _, err := fs.Stat(sub, path); err != nil {
			t.Fatalf("admin app asset %s missing: %v", path, err)
		}
	}
}
