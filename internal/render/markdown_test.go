package render

import (
	"strings"
	"testing"
)

func TestMarkdownToHTMLAdvancedBlocksAreSafe(t *testing.T) {
	body := []byte(`# Guide

Inline math $a+b$.

![logo](images/logo.png)
[bad](javascript:alert(1))
https://example.com

| Name | Value |
| --- | --- |
| HTML | <script>alert(1)</script> |

- ~~old~~
- [x] publish
- [ ] review

` + "```go" + `
fmt.Println("<safe>")
` + "```" + `

` + "```mermaid" + `
flowchart LR
  A-->B
` + "```" + `

` + "```katex" + `
E = mc^2
` + "```" + `
`)

	rendered := MarkdownToHTML(body)
	mustContain := []string{
		`<img src="images/logo.png" alt="logo">`,
		`href="https://example.com"`,
		`<del>old</del>`,
		`<table>`,
		`type="checkbox"`,
		`checked=""`,
		`class="chroma"`,
		`fmt`,
		`href="/markdown-assets/katex/katex.min.css"`,
		`src="/markdown-assets/katex/katex.min.js"`,
		`src="/markdown-assets/katex/contrib/auto-render.min.js"`,
		`src="/markdown-assets/mermaid/mermaid.min.js"`,
		`class="mermaid"`,
		`data-pagepilot-math-block`,
		`data-pagepilot-math-inline`,
		`renderMathInElement`,
		`mermaid.initialize`,
	}
	for _, want := range mustContain {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered markdown missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, `href="javascript:alert(1)"`) || strings.Contains(rendered, "<script>alert(1)</script>") {
		t.Fatalf("rendered markdown contains unsafe active content:\n%s", rendered)
	}
}

func TestMarkdownToHTMLIncludesRenderedAndSourceViews(t *testing.T) {
	source := "# Title\n\n<script>alert(1)</script>\n\nInline math $a+b$."

	rendered := MarkdownToHTML([]byte(source))

	mustContain := []string{
		`<div class="markdown-body">`,
		`class="markdown-source"`,
		`class="markdown-floating-tools"`,
		`class="markdown-view-toggle"`,
		`class="markdown-theme-toggle"`,
		`data-markdown-view`,
		`ignoredClasses:['markdown-source']`,
		`# Title`,
		`&lt;script&gt;alert(1)&lt;/script&gt;`,
		`查看 Markdown 原文`,
	}
	for _, want := range mustContain {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered markdown missing source toggle marker %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, `<pre class="markdown-source" aria-label="Markdown 原文"><script>`) {
		t.Fatalf("raw markdown source must be HTML-escaped:\n%s", rendered)
	}
}

func TestMarkdownPageUsesJpageCompatibleTemplateShape(t *testing.T) {
	rendered := MarkdownToHTML([]byte("# Title\n\n```mermaid\nflowchart LR\n  A-->B\n```\n\n$$E=mc^2$$\n"))

	mustContain := []string{
		`<!DOCTYPE html>`,
		`<div class="markdown-body">`,
		`body {`,
		`background: var(--bg);`,
		`pre.mermaid`,
		`<pre class="mermaid"`,
		`<div class="katex-display" data-pagepilot-math-block>`,
		`mermaid.initialize`,
	}
	for _, want := range mustContain {
		if !strings.Contains(rendered, want) {
			t.Fatalf("markdown page should follow the jpage-compatible template shape, missing %q:\n%s", want, rendered)
		}
	}
	for _, bad := range []string{
		`<figure class="markdown-diagram">`,
		`<figcaption>Mermaid</figcaption>`,
		`radial-gradient`,
	} {
		if strings.Contains(rendered, bad) {
			t.Fatalf("markdown page should not use the old decorated wrapper %q:\n%s", bad, rendered)
		}
	}
}

func TestMarkdownPageUsesSolidBackgroundAndVisibleJSONCodeBlock(t *testing.T) {
	rendered := MarkdownToHTML([]byte("```json\n{\"code\":\"project-home\"}\n```\n"))

	if strings.Contains(rendered, "radial-gradient") {
		t.Fatalf("markdown page should use a solid background, not gradients:\n%s", rendered)
	}
	if !strings.Contains(rendered, `class="chroma"`) ||
		!strings.Contains(rendered, `pre.chroma`) ||
		!strings.Contains(rendered, `background: var(--code) !important`) ||
		!strings.Contains(rendered, `project-home`) {
		t.Fatalf("json fenced code block should render as a visible highlighted block:\n%s", rendered)
	}
}

func TestMarkdownToHTMLSupportsSingleLineBlockMath(t *testing.T) {
	rendered := MarkdownToHTML([]byte("Before\n\n$$E = mc^2$$\n\nAfter"))

	if !strings.Contains(rendered, `class="katex-display"`) ||
		!strings.Contains(rendered, `data-pagepilot-math-block`) {
		t.Fatalf("single-line block math was not rendered as a KaTeX block:\n%s", rendered)
	}
	if strings.Contains(rendered, `<p>$`) || strings.Contains(rendered, `</span>$`) {
		t.Fatalf("single-line block math leaked stray dollar markers:\n%s", rendered)
	}
}

func TestMarkdownInlineMathSkipsCodeSpansAndFences(t *testing.T) {
	rendered := MarkdownToHTML([]byte("Inline code `$HOME$` should stay code.\n\n```sh\necho \"$HOME$\"\n```\n"))

	if strings.Contains(rendered, `data-pagepilot-math-inline`) {
		t.Fatalf("inline math should not be extracted from code spans or code fences:\n%s", rendered)
	}
	if !strings.Contains(rendered, `$HOME$`) || !strings.Contains(rendered, `echo`) {
		t.Fatalf("code span or code fence content was not preserved:\n%s", rendered)
	}
}

func TestMarkdownSpecialFenceAcceptsInfoStringOptions(t *testing.T) {
	rendered := MarkdownToHTML([]byte("```mermaid title=flow\nflowchart LR\n  A-->B\n```\n\n```katex display\nE = mc^2\n```\n"))

	if !strings.Contains(rendered, `class="mermaid"`) {
		t.Fatalf("mermaid fence with info string options was not recognized:\n%s", rendered)
	}
	if !strings.Contains(rendered, `data-pagepilot-math-block`) {
		t.Fatalf("katex fence with info string options was not recognized:\n%s", rendered)
	}
}

func TestMarkdownMermaidThemeToggleRestoresOriginalSource(t *testing.T) {
	rendered := MarkdownToHTML([]byte("```mermaid\nflowchart LR\n  A-->B\n```\n"))

	mustContain := []string{
		`data-pagepilot-mermaid-source=`,
		`function restoreMermaidSource`,
		`el.textContent=source`,
		`el.removeAttribute('data-processed')`,
	}
	for _, want := range mustContain {
		if !strings.Contains(rendered, want) {
			t.Fatalf("markdown Mermaid theme toggle missing %q:\n%s", want, rendered)
		}
	}
}

func TestMarkdownSanitizerRejectsEncodedActiveURLsAndEventHandlers(t *testing.T) {
	input := `<a href="javascript&#58;alert(1)" onclick=alert(1)>bad</a>` +
		`<img src="https://safe.example.com/a.png" onerror=alert(1) onload='alert(2)'>` +
		`<img src="data:image/svg+xml;base64,PHN2ZyBvbmxvYWQ9YWxlcnQoMSk+" alt="bad">`

	rendered := sanitizeMarkdownHTML(input)

	mustNotContain := []string{
		`href="javascript`,
		`javascript&#58;`,
		`onclick=`,
		`onerror=`,
		`onload=`,
		`data:image/svg+xml`,
	}
	for _, bad := range mustNotContain {
		if strings.Contains(strings.ToLower(rendered), strings.ToLower(bad)) {
			t.Fatalf("sanitized markdown still contains %q:\n%s", bad, rendered)
		}
	}
	if !strings.Contains(rendered, `src="https://safe.example.com/a.png"`) {
		t.Fatalf("sanitizer removed safe image URL:\n%s", rendered)
	}
}

func TestMarkdownSanitizerRejectsUnsafeSrcsetCandidates(t *testing.T) {
	input := `<img src="images/safe.png" srcset="images/small.png 1x, https://safe.example.com/large.png 2x">` +
		`<img src="images/bad.png" srcset="data:image/svg+xml;base64,PHN2ZyBvbmxvYWQ9YWxlcnQoMSk+ 1x, javascript:alert(1) 2x">`

	rendered := sanitizeMarkdownHTML(input)

	if !strings.Contains(rendered, `srcset="images/small.png 1x, https://safe.example.com/large.png 2x"`) {
		t.Fatalf("sanitizer removed safe srcset candidates:\n%s", rendered)
	}
	for _, bad := range []string{
		`data:image/svg+xml`,
		`javascript:alert`,
	} {
		if strings.Contains(strings.ToLower(rendered), strings.ToLower(bad)) {
			t.Fatalf("sanitized markdown still contains unsafe srcset candidate %q:\n%s", bad, rendered)
		}
	}
	if strings.Contains(rendered, `srcset="data:`) || strings.Contains(rendered, `srcset="javascript`) {
		t.Fatalf("unsafe srcset attribute should be removed entirely:\n%s", rendered)
	}
}

func TestMarkdownSanitizerRejectsNamespacedActiveURLAttributes(t *testing.T) {
	input := `<svg><a xlink:href="javascript:alert(1)">bad</a></svg>` +
		`<a href="https://safe.example.com/page">safe</a>`

	rendered := sanitizeMarkdownHTML(input)

	for _, bad := range []string{
		`xlink:href`,
		`javascript:alert`,
	} {
		if strings.Contains(strings.ToLower(rendered), strings.ToLower(bad)) {
			t.Fatalf("sanitized markdown still contains unsafe namespaced URL %q:\n%s", bad, rendered)
		}
	}
	if !strings.Contains(rendered, `href="https://safe.example.com/page"`) {
		t.Fatalf("sanitizer removed safe href:\n%s", rendered)
	}
}

func TestMarkdownToHTMLAppliesExplicitTheme(t *testing.T) {
	dark := MarkdownToHTMLWithTheme([]byte("# Theme"), "dark")
	auto := MarkdownToHTMLWithTheme([]byte("# Theme"), "auto")
	invalid := MarkdownToHTMLWithTheme([]byte("# Theme"), "unknown")
	defaultRendered := MarkdownToHTML([]byte("# Theme"))

	if !strings.Contains(dark, `<html lang="zh-CN" data-theme="dark">`) ||
		!strings.Contains(dark, `html[data-theme="dark"]`) {
		t.Fatalf("dark themed markdown missing theme marker or CSS:\n%s", dark)
	}
	if !strings.Contains(auto, `<html lang="zh-CN" data-theme="auto">`) {
		t.Fatalf("auto themed markdown missing theme marker:\n%s", auto)
	}
	if !strings.Contains(invalid, `<html lang="zh-CN" data-theme="dark">`) {
		t.Fatalf("invalid theme should normalize to default dark:\n%s", invalid)
	}
	if !strings.Contains(defaultRendered, `<html lang="zh-CN" data-theme="dark">`) {
		t.Fatalf("default theme should be dark:\n%s", defaultRendered)
	}
}
