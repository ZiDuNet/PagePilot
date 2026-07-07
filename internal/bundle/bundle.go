package bundle

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
)

const (
	KindHTML     = "html"
	KindMarkdown = "markdown"

	StageZipBundle = "zip_bundle"
)

type ErrorCode string

const (
	ErrCodeZipOpen        ErrorCode = "ZIP_OPEN_FAILED"
	ErrCodeUnsafePath     ErrorCode = "ZIP_UNSAFE_PATH"
	ErrCodeEntryRead      ErrorCode = "ZIP_ENTRY_READ_FAILED"
	ErrCodeFileTooLarge   ErrorCode = "ZIP_FILE_TOO_LARGE"
	ErrCodeTotalTooLarge  ErrorCode = "ZIP_TOTAL_TOO_LARGE"
	ErrCodeTooManyFiles   ErrorCode = "ZIP_TOO_MANY_FILES"
	ErrCodeEmpty          ErrorCode = "ZIP_EMPTY"
	ErrCodeDuplicatePath  ErrorCode = "ZIP_DUPLICATE_PATH"
	ErrCodeEntryMissing   ErrorCode = "ZIP_ENTRY_MISSING"
	ErrCodeAmbiguousEntry ErrorCode = "ZIP_AMBIGUOUS_ENTRY"
)

type Limits struct {
	MaxSingleFileBytes int64
	MaxSiteTotalBytes  int64
	MaxFiles           int
}

type Input struct {
	Name      string
	Data      []byte
	EntryHint string
	Limits    Limits
}

type File struct {
	Path     string
	Bytes    []byte
	IsBinary bool
	SHA256   string
}

type Result struct {
	Files     []File
	MainEntry string
	Root      string
	Kind      string
	TreeJSON  string
}

// Error 是 ZIP/Bundle 入口识别阶段的结构化错误。
type Error struct {
	Code   ErrorCode
	Stage  string
	Detail string
	Hint   string
	Err    error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return e.Detail + ": " + e.Err.Error()
	}
	return e.Detail
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type treeItem struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	IsBinary bool   `json:"isBinary"`
	SHA256   string `json:"sha256"`
}

// AnalyzeZip validates a website ZIP and returns deployable files rooted at the real entry folder.
func AnalyzeZip(input Input) (Result, error) {
	limits := normalizeLimits(input.Limits)
	zr, err := zip.NewReader(bytes.NewReader(input.Data), int64(len(input.Data)))
	if err != nil {
		return Result{}, newError(
			ErrCodeZipOpen,
			fmt.Sprintf("ZIP file %q cannot be opened", input.Name),
			"Upload a valid .zip archive.",
			err,
		)
	}

	raw := make([]File, 0, len(zr.File))
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		if unsafeZipEntryName(zf.Name) {
			return Result{}, unsafePathError(zf.Name, nil)
		}
		path := normalizeZipEntryPath(zf.Name)
		if shouldSkipArchivePath(path) {
			continue
		}
		if err := validatePath(path); err != nil {
			return Result{}, unsafePathError(zf.Name, err)
		}
		if zf.UncompressedSize64 > uint64(limits.MaxSingleFileBytes) {
			return Result{}, newError(
				ErrCodeFileTooLarge,
				fmt.Sprintf("file %s exceeds max single-file size (%d bytes)", path, limits.MaxSingleFileBytes),
				"Split large assets or raise the single-file upload limit in admin settings.",
				nil,
			)
		}
		rc, err := zf.Open()
		if err != nil {
			return Result{}, newError(
				ErrCodeEntryRead,
				fmt.Sprintf("ZIP entry %q cannot be read", path),
				"Rebuild the archive and upload it again.",
				err,
			)
		}
		data, err := readLimited(rc, limits.MaxSingleFileBytes)
		_ = rc.Close()
		if err != nil {
			return Result{}, newError(
				ErrCodeFileTooLarge,
				fmt.Sprintf("ZIP entry %s exceeds max single-file size (%d bytes)", path, limits.MaxSingleFileBytes),
				"Split large assets or raise the single-file upload limit in admin settings.",
				err,
			)
		}
		raw = append(raw, File{
			Path:     path,
			Bytes:    data,
			IsBinary: !looksTextPath(path),
			SHA256:   sha256sum(data),
		})
	}
	if len(raw) == 0 {
		return Result{}, newError(
			ErrCodeEmpty,
			"ZIP did not contain deployable files",
			"Upload a ZIP containing index.html, README.md, or a static site folder.",
			nil,
		)
	}

	entryHint := normalizeEntryHint(input.EntryHint)
	root, err := chooseRoot(raw, entryHint)
	if err != nil {
		return Result{}, err
	}
	files, err := trimRoot(raw, root, limits)
	if err != nil {
		return Result{}, err
	}
	mainEntry, err := ChooseMainEntry(files, entryHintAfterRoot(entryHint, root))
	if err != nil {
		return Result{}, err
	}
	kind := KindHTML
	if IsMarkdownPath(mainEntry) {
		kind = KindMarkdown
	}
	treeJSON, err := buildTreeJSON(files)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Files:     files,
		MainEntry: mainEntry,
		Root:      root,
		Kind:      kind,
		TreeJSON:  treeJSON,
	}, nil
}

func normalizeLimits(limits Limits) Limits {
	if limits.MaxSingleFileBytes <= 0 {
		limits.MaxSingleFileBytes = 1 << 20
	}
	if limits.MaxSiteTotalBytes <= 0 {
		limits.MaxSiteTotalBytes = 10 << 20
	}
	if limits.MaxFiles <= 0 {
		limits.MaxFiles = 100
	}
	return limits
}

func trimRoot(files []File, root string, limits Limits) ([]File, error) {
	trimmed := make([]File, 0, len(files))
	seen := make(map[string]bool, len(files))
	var totalSize int64
	for _, f := range files {
		rel := f.Path
		if root != "" {
			if rel != root && !strings.HasPrefix(rel, root+"/") {
				continue
			}
			rel = strings.TrimPrefix(rel, root+"/")
		}
		if rel == "" || shouldSkipArchivePath(rel) {
			continue
		}
		if err := validatePath(rel); err != nil {
			return nil, unsafePathError(rel, err)
		}
		if seen[rel] {
			return nil, newError(
				ErrCodeDuplicatePath,
				fmt.Sprintf("duplicate file path after ZIP root detection: %s", rel),
				"Rename duplicate files that collapse to the same relative path after root detection.",
				nil,
			)
		}
		seen[rel] = true
		totalSize += int64(len(f.Bytes))
		if totalSize > limits.MaxSiteTotalBytes {
			return nil, newError(
				ErrCodeTotalTooLarge,
				fmt.Sprintf("total size exceeds site limit (%d bytes)", limits.MaxSiteTotalBytes),
				"Remove unused assets or raise the whole-site upload limit in admin settings.",
				nil,
			)
		}
		f.Path = rel
		f.SHA256 = sha256sum(f.Bytes)
		trimmed = append(trimmed, f)
	}
	if len(trimmed) > limits.MaxFiles {
		return nil, newError(
			ErrCodeTooManyFiles,
			fmt.Sprintf("too many files in ZIP (%d); max %d per site", len(trimmed), limits.MaxFiles),
			"Reduce generated artifacts or raise the file-count limit in admin settings.",
			nil,
		)
	}
	return trimmed, nil
}

func chooseRoot(files []File, entryHint string) (string, error) {
	if entryHint != "" {
		for _, f := range files {
			if strings.EqualFold(f.Path, entryHint) && IsPageEntry(f.Path) {
				return normalizeRoot(slashDir(f.Path)), nil
			}
		}
	}

	var candidates []string
	for _, preferred := range []string{"index.html", "index.htm", "README.md", "readme.md", "README.markdown", "readme.markdown"} {
		candidates = candidates[:0]
		for _, f := range files {
			if strings.EqualFold(filepath.Base(f.Path), preferred) {
				candidates = append(candidates, slashDir(f.Path))
			}
		}
		if len(candidates) == 1 {
			return normalizeRoot(candidates[0]), nil
		}
		if len(candidates) > 1 {
			if root, ok := commonSingleRoot(candidates); ok {
				return root, nil
			}
			return "", ambiguousEntryError(fmt.Sprintf("ZIP contains multiple possible %s entries; package one website root or pass an explicit entry", preferred))
		}
	}

	for _, f := range files {
		if IsHTMLPath(f.Path) || IsMarkdownPath(f.Path) {
			candidates = append(candidates, slashDir(f.Path))
		}
	}
	if len(candidates) == 0 {
		return "", missingEntryError()
	}
	if len(candidates) == 1 {
		return normalizeRoot(candidates[0]), nil
	}
	if root, ok := commonSingleRoot(candidates); ok {
		return root, nil
	}
	return "", ambiguousEntryError("ZIP contains multiple possible page entries; package one website root or pass an explicit entry")
}

func normalizeEntryHint(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = normalizeZipEntryPath(path)
	if validatePath(path) != nil || !IsPageEntry(path) {
		return ""
	}
	return path
}

func entryHintAfterRoot(entryHint, root string) string {
	if entryHint == "" {
		return ""
	}
	root = normalizeRoot(root)
	if root == "" {
		return entryHint
	}
	prefix := root + "/"
	if strings.EqualFold(entryHint, root) {
		return ""
	}
	if len(entryHint) > len(prefix) && strings.EqualFold(entryHint[:len(prefix)], prefix) {
		return entryHint[len(prefix):]
	}
	return entryHint
}

func commonSingleRoot(roots []string) (string, bool) {
	if len(roots) == 0 {
		return "", false
	}
	first := normalizeRoot(roots[0])
	for _, root := range roots[1:] {
		if normalizeRoot(root) != first {
			return "", false
		}
	}
	return first, true
}

func slashDir(path string) string {
	dir := filepath.ToSlash(filepath.Dir(path))
	if dir == "." {
		return ""
	}
	return dir
}

func normalizeRoot(root string) string {
	root = filepath.ToSlash(root)
	if root == "." {
		return ""
	}
	return strings.Trim(root, "/")
}

func normalizeZipEntryPath(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	path = strings.TrimPrefix(path, "/")
	path = filepath.Clean(path)
	path = strings.ReplaceAll(path, "\\", "/")
	if path == "." {
		return ""
	}
	return path
}

func unsafeZipEntryName(path string) bool {
	path = strings.ReplaceAll(path, "\\", "/")
	if path == "" || strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return true
	}
	if len(path) >= 2 && path[1] == ':' {
		return true
	}
	for _, seg := range strings.Split(path, "/") {
		if seg == ".." || seg == "." || seg == "" {
			return true
		}
	}
	return false
}

func shouldSkipArchivePath(path string) bool {
	if path == "" || strings.HasSuffix(path, "/") {
		return true
	}
	base := filepath.Base(path)
	return strings.HasPrefix(path, "__MACOSX/") || base == ".DS_Store" || base == "Thumbs.db"
}

func validatePath(path string) error {
	if path == "" || strings.HasPrefix(path, "/") || strings.HasPrefix(path, `\\`) {
		return fmt.Errorf("path must be a non-empty relative path")
	}
	path = filepath.ToSlash(path)
	if len(path) >= 2 && path[1] == ':' {
		return fmt.Errorf("path must not contain a drive letter")
	}
	for _, seg := range strings.Split(path, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return fmt.Errorf("path contains unsafe segment %q", seg)
		}
	}
	return nil
}

func readLimited(r io.Reader, max int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("too large")
	}
	return data, nil
}

func buildTreeJSON(files []File) (string, error) {
	items := make([]treeItem, 0, len(files))
	for _, f := range files {
		items = append(items, treeItem{
			Path:     f.Path,
			Size:     int64(len(f.Bytes)),
			IsBinary: f.IsBinary,
			SHA256:   f.SHA256,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Path < items[j].Path
	})
	data, err := json.Marshal(items)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func IsHTMLPath(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".html") || strings.HasSuffix(lower, ".htm")
}

func IsMarkdownPath(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown")
}

func IsPageEntry(path string) bool {
	return IsHTMLPath(path) || IsMarkdownPath(path)
}

func ChooseMainEntry(files []File, filenameHint string) (string, error) {
	if filenameHint != "" && IsPageEntry(filenameHint) {
		for _, f := range files {
			if f.Path == filenameHint {
				return filenameHint, nil
			}
		}
	}
	preferred := []string{"index.html", "index.htm", "README.md", "readme.md", "README.markdown", "readme.markdown"}
	for _, want := range preferred {
		for _, f := range files {
			if strings.EqualFold(f.Path, want) {
				return f.Path, nil
			}
		}
	}
	for _, f := range files {
		if IsHTMLPath(f.Path) {
			return f.Path, nil
		}
	}
	for _, f := range files {
		if IsMarkdownPath(f.Path) {
			return f.Path, nil
		}
	}
	return "", missingEntryError()
}

func looksTextPath(path string) bool {
	lower := strings.ToLower(path)
	for _, ext := range []string{".html", ".htm", ".css", ".js", ".mjs", ".json", ".txt", ".md", ".markdown", ".svg", ".xml", ".csv", ".webmanifest", ".map"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func sha256sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func newError(code ErrorCode, detail, hint string, err error) *Error {
	return &Error{
		Code:   code,
		Stage:  StageZipBundle,
		Detail: detail,
		Hint:   hint,
		Err:    err,
	}
}

func unsafePathError(path string, err error) *Error {
	return newError(
		ErrCodeUnsafePath,
		fmt.Sprintf("ZIP entry %q is not a safe relative path", path),
		"ZIP entries must not contain '..', absolute paths, drive letters, UNC paths, or empty path segments.",
		err,
	)
}

func ambiguousEntryError(detail string) *Error {
	return newError(
		ErrCodeAmbiguousEntry,
		detail,
		"Package one deployable website root per ZIP, or publish each website separately.",
		nil,
	)
}

func missingEntryError() *Error {
	return newError(
		ErrCodeEntryMissing,
		"ZIP did not contain an HTML or Markdown entry",
		"Put index.html or README.md in the site folder, or pass an explicit entry file.",
		nil,
	)
}
