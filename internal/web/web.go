// Package web 提供 hostctl 内置的 Web 资源：
//   - user/：用户端站点（参考 htmlcode.fun UI 风格，自实现）
//   - admin/：管理后台单页 + 子资源（独立简约风格）
//
// 所有内容通过 go:embed 打进二进制，避免运行时磁盘依赖。
package web

import (
	"embed"
	"io/fs"
	"sync"
)

//go:embed admin admin/admin.html
var adminFS embed.FS

//go:embed user
var userFS embed.FS

var (
	adminHTMLOnce sync.Once
	adminHTMLBytes []byte
	adminHTMLErr   error
)

// AdminHTML 返回管理后台单页 HTML（admin/admin.html）。
func AdminHTML() []byte {
	adminHTMLOnce.Do(func() {
		adminHTMLBytes, adminHTMLErr = fs.ReadFile(adminFS, "admin/admin.html")
	})
	if adminHTMLErr != nil {
		panic(adminHTMLErr)
	}
	return adminHTMLBytes
}

// UserSubFS 返回用户端站点子目录的 fs.FS（已剥离 "user/" 前缀）。
func UserSubFS() fs.FS {
	sub, err := fs.Sub(userFS, "user")
	if err != nil {
		panic(err)
	}
	return sub
}

// AdminSubFS 返回管理后台资源子目录的 fs.FS（已剥离 "admin/" 前缀）。
// 用于 /admin/static/* 路径下的额外资源（CSS/JS/图标等）。
func AdminSubFS() fs.FS {
	sub, err := fs.Sub(adminFS, "admin")
	if err != nil {
		panic(err)
	}
	return sub
}
