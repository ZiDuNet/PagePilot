// Package web provides PagePilot built-in web assets.
// Only current React build outputs are embedded at runtime; older static HTML
// files in the repository are intentionally excluded from the binary.
package web

import (
	"embed"
	"io/fs"
	"sync"
)

//go:embed admin/app admin/app/index.html
var adminFS embed.FS

//go:embed user/app user/app/index.html
var userFS embed.FS

//go:embed skill/hostctl-deploy.zip
var skillPackageFS embed.FS

//go:embed markdown/vendor
var markdownAssetFS embed.FS

var (
	adminHTMLOnce  sync.Once
	adminHTMLBytes []byte
	adminHTMLErr   error
)

// AdminHTML returns the admin React entrypoint.
func AdminHTML() []byte {
	adminHTMLOnce.Do(func() {
		adminHTMLBytes, adminHTMLErr = fs.ReadFile(adminFS, "admin/app/index.html")
	})
	if adminHTMLErr != nil {
		panic(adminHTMLErr)
	}
	return adminHTMLBytes
}

// UserSubFS returns the current user React app build.
func UserSubFS() fs.FS {
	sub, err := fs.Sub(userFS, "user/app")
	if err != nil {
		panic(err)
	}
	return sub
}

// AdminSubFS returns admin assets for legacy /admin/static requests.
func AdminSubFS() fs.FS {
	sub, err := fs.Sub(adminFS, "admin/app")
	if err != nil {
		panic(err)
	}
	return sub
}

// AdminAppFS returns the admin React app build.
func AdminAppFS() fs.FS {
	sub, err := fs.Sub(adminFS, "admin/app")
	if err != nil {
		panic(err)
	}
	return sub
}

// SkillPackage returns the built-in pagep Skill ZIP.
func SkillPackage() ([]byte, error) {
	return skillPackageFS.ReadFile("skill/hostctl-deploy.zip")
}

// MarkdownAssetsFS returns platform-owned Markdown runtime assets.
func MarkdownAssetsFS() fs.FS {
	sub, err := fs.Sub(markdownAssetFS, "markdown/vendor")
	if err != nil {
		panic(err)
	}
	return sub
}
