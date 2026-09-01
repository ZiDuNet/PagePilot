package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yourorg/hostctl/internal/api"
	"github.com/yourorg/hostctl/internal/bundle"
)

type preflightLimits struct {
	MaxSingleFileBytes int64 `json:"maxSingleFileBytes"`
	MaxSiteTotalBytes  int64 `json:"maxSiteTotalBytes"`
	MaxFiles           int   `json:"maxFiles"`
}

type preflightFile struct {
	Path     string `json:"path"`
	Bytes    int64  `json:"bytes"`
	IsBinary bool   `json:"isBinary"`
	SHA256   string `json:"sha256,omitempty"`
}

type preflightIssue struct {
	Code   string `json:"code"`
	Stage  string `json:"stage,omitempty"`
	Detail string `json:"detail"`
	Hint   string `json:"hint,omitempty"`
}

type preflightReport struct {
	Success    bool             `json:"success"`
	Source     string           `json:"source"`
	SourceType string           `json:"sourceType"`
	Kind       string           `json:"kind,omitempty"`
	MainEntry  string           `json:"mainEntry,omitempty"`
	Root       string           `json:"root,omitempty"`
	Files      []preflightFile  `json:"files"`
	Count      int              `json:"count"`
	Bytes      int64            `json:"bytes"`
	Limits     preflightLimits  `json:"limits"`
	Errors     []preflightIssue `json:"errors,omitempty"`
}

func cmdPreflight() *cobra.Command {
	var filename string
	c := &cobra.Command{
		Use:   "preflight <source>",
		Short: "Inspect a local HTML, Markdown, ZIP, or directory before upload",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			report := analyzePreflight(args[0], filename)
			if flagJSON {
				_ = json.NewEncoder(os.Stdout).Encode(report)
			} else {
				printPreflightReport(report)
			}
			if !report.Success {
				return errSilent
			}
			return nil
		},
	}
	c.Flags().StringVarP(&filename, "filename", "f", "", "optional explicit HTML or Markdown entry path")
	return c
}

func analyzePreflight(source, entryHint string) preflightReport {
	limits := bundle.DefaultLimits()
	report := preflightReport{
		Source: source,
		Files:  make([]preflightFile, 0),
		Limits: preflightLimits{
			MaxSingleFileBytes: limits.MaxSingleFileBytes,
			MaxSiteTotalBytes:  limits.MaxSiteTotalBytes,
			MaxFiles:           limits.MaxFiles,
		},
	}
	entryHint = strings.TrimSpace(entryHint)
	if entryHint != "" {
		entryHint = strings.ReplaceAll(entryHint, "\\", "/")
		if !safePreflightPath(entryHint) {
			report.addError("UNSAFE_ENTRY_PATH", "", fmt.Sprintf("--filename %q is not a safe relative path", entryHint), "Use a clean relative HTML or Markdown path without '..', absolute paths, or drive letters.")
			return report
		}
	}
	info, err := os.Lstat(source)
	if err != nil {
		report.addError("SOURCE_NOT_FOUND", "", err.Error(), "Choose an existing HTML, Markdown, ZIP, or directory.")
		return report
	}
	if info.Mode()&os.ModeSymlink != 0 {
		report.addError("UNSAFE_SYMLINK", "", "source path is a symbolic link", "Use the real source file or directory instead of a symbolic link.")
		return report
	}

	if info.IsDir() {
		report.SourceType = "directory"
		multipart, err := prepareMultipartSource(source)
		if err != nil {
			report.addError("SOURCE_PREPARE_FAILED", "", err.Error(), "Resolve the local directory issue and run preflight again.")
			return report
		}
		defer multipart.Cleanup()
		return analyzePreflightZIP(report, multipart.Path, multipart.Name, entryHint, limits)
	}
	if strings.EqualFold(filepath.Ext(source), ".zip") {
		report.SourceType = "zip"
		return analyzePreflightZIP(report, source, filepath.Base(source), entryHint, limits)
	}

	report.SourceType = "file"
	path := api.SanitizeMultipartDeployPath(filepath.Base(source), "upload")
	if !bundle.IsPageEntry(path) {
		report.addError("ENTRY_MISSING", "", "single-file uploads must be HTML or Markdown", "Use an .html, .htm, .md, or .markdown file, or package a static site as a ZIP.")
		return report
	}
	if info.Size() > limits.MaxSingleFileBytes {
		report.addError("ZIP_FILE_TOO_LARGE", bundle.StageZipBundle, fmt.Sprintf("file %s exceeds max single-file size (%d bytes)", path, limits.MaxSingleFileBytes), "Split large assets or raise the single-file upload limit in admin settings.")
		return report
	}
	if info.Size() > limits.MaxSiteTotalBytes {
		report.addError("ZIP_TOTAL_TOO_LARGE", bundle.StageZipBundle, fmt.Sprintf("file %s exceeds site limit (%d bytes)", path, limits.MaxSiteTotalBytes), "Reduce the source file size or raise the whole-site upload limit in admin settings.")
		return report
	}
	report.MainEntry = path
	if bundle.IsMarkdownPath(path) {
		report.Kind = "markdown"
	} else {
		report.Kind = "single_html"
	}
	data, err := os.ReadFile(source)
	if err != nil {
		report.addError("SOURCE_READ_FAILED", "", err.Error(), "Check that the source file is readable.")
		return report
	}
	report.Files = append(report.Files, preflightFile{Path: path, Bytes: info.Size(), IsBinary: looksBinary(data)})
	report.Count = 1
	report.Bytes = info.Size()
	if !validatePreflightEntrypoint(&report, path, []bundle.File{{
		Path:     path,
		Bytes:    data,
		IsBinary: looksBinary(data),
	}}) {
		return report
	}
	report.Success = true
	return report
}

func safePreflightPath(path string) bool {
	return bundle.IsSafePath(path)
}

func analyzePreflightZIP(report preflightReport, sourcePath, uploadName, entryHint string, limits bundle.Limits) preflightReport {
	info, err := os.Stat(sourcePath)
	if err != nil {
		report.addError("SOURCE_READ_FAILED", "", err.Error(), "Check that the source archive is readable.")
		return report
	}
	if info.Size() > limits.MaxSingleFileBytes {
		report.addError("ZIP_FILE_TOO_LARGE", bundle.StageZipBundle,
			fmt.Sprintf("source ZIP exceeds max single-file size (%d bytes)", limits.MaxSingleFileBytes),
			"Reduce the compressed archive size or raise the single-file upload limit in admin settings.")
		return report
	}
	if info.Size() > limits.MaxSiteTotalBytes {
		report.addError("SOURCE_TOO_LARGE", "", fmt.Sprintf("source ZIP exceeds site limit (%d bytes)", limits.MaxSiteTotalBytes), "Reduce the archive size before uploading.")
		return report
	}
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		report.addError("SOURCE_READ_FAILED", "", err.Error(), "Check that the source archive is readable.")
		return report
	}
	result, err := bundle.AnalyzeZip(bundle.Input{
		Name:      uploadName,
		Data:      data,
		EntryHint: entryHint,
		Limits:    limits,
	})
	if err != nil {
		var bundleErr *bundle.Error
		if errors.As(err, &bundleErr) {
			report.addError(string(bundleErr.Code), bundleErr.Stage, bundleErr.Detail, bundleErr.Hint)
		} else {
			report.addError("ZIP_ANALYZE_FAILED", bundle.StageZipBundle, err.Error(), "Rebuild the archive and run preflight again.")
		}
		return report
	}
	report.Root = result.Root
	report.MainEntry = result.MainEntry
	if result.Kind == bundle.KindMarkdown {
		report.Kind = "markdown"
	} else {
		report.Kind = "zip_site"
	}
	for _, file := range result.Files {
		report.Files = append(report.Files, preflightFile{
			Path:     file.Path,
			Bytes:    int64(len(file.Bytes)),
			IsBinary: file.IsBinary,
			SHA256:   file.SHA256,
		})
		report.Bytes += int64(len(file.Bytes))
	}
	sort.Slice(report.Files, func(i, j int) bool { return report.Files[i].Path < report.Files[j].Path })
	report.Count = len(report.Files)
	if !validatePreflightEntrypoint(&report, result.MainEntry, result.Files) {
		return report
	}
	report.Success = true
	return report
}

// validatePreflightEntrypoint mirrors the final entry validation performed by
// the deployment service after bundle expansion. Keeping this local check in
// step with the server prevents a misleading READY result for invalid pages.
func validatePreflightEntrypoint(report *preflightReport, mainEntry string, files []bundle.File) bool {
	for _, file := range files {
		if file.Path != mainEntry || file.IsBinary {
			continue
		}
		content := strings.TrimSpace(string(file.Bytes))
		if bundle.IsMarkdownPath(mainEntry) {
			if len(content) < 3 {
				report.addError("INVALID_INPUT", "validate", "main Markdown entry is too short", "Upload a Markdown document with at least one heading or paragraph.")
				return false
			}
			return true
		}

		html := strings.ToLower(content)
		if len(html) < 32 {
			report.addError("INVALID_INPUT", "validate", "main HTML entry is too short to be a page", "Upload a real HTML file with tags such as <html>, <body>, <main>, <script>, or <style>.")
			return false
		}
		if !preflightLooksLikeHTMLContent(html) {
			report.addError("INVALID_INPUT", "validate", "main entry does not look like an HTML page", "Plain text is not deployable here. Provide a valid HTML document or generated static site.")
			return false
		}
		return true
	}
	report.addError("INVALID_INPUT", "validate", fmt.Sprintf("main entry %q was not found in uploaded files", mainEntry), "Ensure the selected HTML or Markdown entry is uploaded as text.")
	return false
}

func preflightLooksLikeHTMLContent(content string) bool {
	if !strings.Contains(content, "<") || !strings.Contains(content, ">") {
		return false
	}
	for _, tag := range []string{
		"<main", "<section", "<article", "<nav", "<header", "<footer",
		"<div", "<p", "<h1", "<h2", "<h3", "<ul", "<ol", "<table",
		"<form", "<button", "<canvas", "<svg", "<script", "<style",
	} {
		if strings.Contains(content, tag) {
			return true
		}
	}
	return false
}

func (r *preflightReport) addError(code, stage, detail, hint string) {
	r.Errors = append(r.Errors, preflightIssue{Code: code, Stage: stage, Detail: detail, Hint: hint})
}

func printPreflightReport(report preflightReport) {
	status := "READY"
	if !report.Success {
		status = "BLOCKED"
	}
	fmt.Printf("PagePilot preflight: %s\n", status)
	fmt.Printf("  Source: %s (%s)\n", report.Source, report.SourceType)
	if report.Success {
		fmt.Printf("  Bundle: %s, entry %s\n", report.Kind, report.MainEntry)
		if report.Root != "" {
			fmt.Printf("  Root:   %s\n", report.Root)
		}
		fmt.Printf("  Files:  %d, %d bytes\n", report.Count, report.Bytes)
	}
	for _, issue := range report.Errors {
		fmt.Printf("  [FAIL] %s: %s\n", issue.Code, issue.Detail)
		if issue.Hint != "" {
			fmt.Printf("         %s\n", issue.Hint)
		}
	}
}
