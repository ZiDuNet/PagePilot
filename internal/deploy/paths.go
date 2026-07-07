package deploy

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yourorg/hostctl/internal/api"
)

// maxFileDepth 限制路径深度，防止过深嵌套。
const maxFileDepth = 16

// maxPathSegments 单个路径最多段数。
const maxPathSegments = 16

// validateFilePath 校验单个 file.path 是否合法。
// 规则：
//   - 必须匹配白名单正则
//   - 不允许绝对路径（Windows 盘符或 UNC）
//   - 不允许 .. 段
//   - 不允许前导 /
//   - 不允许连续分隔符
//   - 不允许 Windows 保留名（CON, PRN, AUX, NUL, COM1-9, LPT1-9）
func validateFilePath(p string) *api.APIError {
	if p == "" {
		return api.NewError(api.CodeInvalidFilePath, "validate", "file path is empty")
	}
	if len(p) > 255 {
		return api.NewError(api.CodeInvalidFilePath, "validate",
			fmt.Sprintf("file path too long (max 255 chars): %s", truncate(p, 50)))
	}
	if !filePathCharsSafe(p) {
		return api.NewError(api.CodeInvalidFilePath, "validate",
			fmt.Sprintf("file path contains invalid characters: %s", truncate(p, 50))).
			WithHint("Only letters, digits, Unicode letters, hyphen, underscore, dot, and slash are allowed.")
	}
	if strings.HasPrefix(p, "/") {
		return api.NewError(api.CodeInvalidFilePath, "validate",
			"file path must not be absolute").WithHint("Use relative paths like 'index.html' or 'images/logo.png'.")
	}
	// Windows 盘符 / UNC
	if len(p) >= 2 && p[1] == ':' {
		return api.NewError(api.CodeInvalidFilePath, "validate",
			"file path must not contain drive letter").WithHint("Use relative paths only.")
	}
	if strings.HasPrefix(p, `\\`) {
		return api.NewError(api.CodeInvalidFilePath, "validate",
			"UNC paths are not allowed")
	}
	if strings.Contains(p, "//") || strings.Contains(p, `\\`) {
		return api.NewError(api.CodeInvalidFilePath, "validate",
			"file path must not contain consecutive separators")
	}

	// 切段，逐个检查
	segs := strings.Split(p, "/")
	if len(segs) > maxPathSegments {
		return api.NewError(api.CodeInvalidFilePath, "validate",
			fmt.Sprintf("file path too deep (max %d segments)", maxPathSegments))
	}
	for _, seg := range segs {
		if seg == "" {
			return api.NewError(api.CodeInvalidFilePath, "validate",
				"file path contains empty segment")
		}
		if seg == "." || seg == ".." {
			return api.NewError(api.CodeInvalidFilePath, "validate",
				fmt.Sprintf("file path contains forbidden segment %q", seg))
		}
		if isWindowsReservedName(seg) {
			return api.NewError(api.CodeInvalidFilePath, "validate",
				fmt.Sprintf("file path uses reserved name %q", seg))
		}
	}
	return nil
}

func filePathCharsSafe(p string) bool {
	if !utf8.ValidString(p) {
		return false
	}
	for _, r := range p {
		if r == '/' || r == '-' || r == '_' || r == '.' {
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		return false
	}
	return true
}

// resolveFileContent 把 DeployFile 解码成原始字节 + 是否二进制。
// 二选一：content（文本）或 contentBase64（二进制）。
func resolveFileContent(f api.DeployFile) ([]byte, bool, *api.APIError) {
	hasText := f.Content != ""
	hasB64 := f.ContentBase64 != ""
	if !hasText && !hasB64 {
		return nil, false, api.NewError(api.CodeInvalidInput, "validate",
			fmt.Sprintf("file %q has neither content nor contentBase64", f.Path))
	}
	if hasText && hasB64 {
		return nil, false, api.NewError(api.CodeInvalidInput, "validate",
			fmt.Sprintf("file %q provides both content and contentBase64; use one", f.Path))
	}
	if hasText {
		return []byte(f.Content), false, nil
	}
	dec, err := base64.StdEncoding.DecodeString(f.ContentBase64)
	if err != nil {
		return nil, false, api.NewError(api.CodeInvalidInput, "validate",
			fmt.Sprintf("file %q contentBase64 decode failed: %v", f.Path, err))
	}
	return dec, true, nil
}

// safeJoin 把相对路径安全地拼到 base 目录。
// 同时做 realpath 校验，确保最终路径在 base 之下。
func safeJoin(base, rel string) (string, error) {
	cleaned := filepath.Clean("/" + rel) // 强制为绝对路径再清理，剥离 ..
	if strings.Contains(cleaned, "..") {
		return "", fmt.Errorf("path traversal detected: %s", rel)
	}
	// 去掉前导 /
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "" {
		return "", fmt.Errorf("empty path after clean")
	}
	full := filepath.Join(base, filepath.FromSlash(cleaned))

	// realpath 校验（如果父目录已存在）
	parentDir := filepath.Dir(full)
	if info, err := os.Stat(parentDir); err == nil && info.IsDir() {
		realParent, err := filepath.EvalSymlinks(parentDir)
		if err == nil {
			realBase, err := filepath.EvalSymlinks(base)
			if err == nil {
				if !strings.HasPrefix(realParent+string(filepath.Separator), realBase+string(filepath.Separator)) && realParent != realBase {
					return "", fmt.Errorf("realpath escape: %s not under %s", realParent, realBase)
				}
			}
		}
	}
	return full, nil
}

// isWindowsReservedName 判断是否是 Windows 保留文件名。
// 在 Linux 上也拒绝，保证跨平台一致。
func isWindowsReservedName(name string) bool {
	// 去掉扩展名
	if i := strings.LastIndex(name, "."); i > 0 {
		name = name[:i]
	}
	name = strings.ToUpper(name)
	switch name {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return true
	}
	return false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ErrPathEscape 是路径越界的 sentinel error。
var ErrPathEscape = errors.New("path escape")
