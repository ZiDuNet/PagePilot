package render

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html"
	"regexp"
	"strings"

	chromaHTML "github.com/alecthomas/chroma/v2/formatters/html"
	highlighting "github.com/yuin/goldmark-highlighting/v2"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	goldmarkHTML "github.com/yuin/goldmark/renderer/html"
)

const markdownPageStyle = `
:root {
	color-scheme: light dark;
	--bg: #ffffff;
	--fg: #1f2328;
	--heading: #1f2328;
	--muted: #57606a;
	--brand: #0969da;
	--line: #d0d7de;
	--soft: #f6f8fa;
	--code: #f6f8fa;
	--codefg: #1f2328;
	--inline-code: rgba(175, 184, 193, .2);
	--danger: #cf222e;
}
body {
	margin: 0;
	background: var(--bg);
	color: var(--fg);
	font: 16px/1.6 -apple-system, BlinkMacSystemFont, "Segoe UI", "Noto Sans SC", "Helvetica Neue", Arial, "PingFang SC", "Microsoft YaHei", sans-serif;
}
.markdown-body {
	box-sizing: border-box;
	min-width: 200px;
	max-width: 980px;
	margin: 0 auto;
	padding: 45px 20px 80px;
}
h1, h2, h3, h4, h5, h6 {
	color: var(--heading);
	line-height: 1.25;
	margin: 24px 0 16px;
	font-weight: 600;
}
h1 {
	font-size: 2em;
	font-weight: 700;
	border-bottom: 1px solid var(--line);
	padding-bottom: .3em;
}
h2 {
	font-size: 1.5em;
	border-bottom: 1px solid var(--line);
	padding-bottom: .3em;
}
h3 { font-size: 1.25em; }
h4 { font-size: 1em; }
p {
	margin: 0 0 16px;
	color: var(--fg);
}
a {
	color: var(--brand);
	text-decoration: none;
}
a:hover { text-decoration: underline; }
img {
	box-sizing: border-box;
	max-width: 100%;
	height: auto;
}
pre {
	overflow-x: auto;
	margin: 16px 0;
	padding: 16px;
	border-radius: 6px;
	background: var(--code);
	color: var(--codefg);
	font-size: 85%;
	line-height: 1.45;
}
pre code {
	color: inherit;
	background: none;
	padding: 0;
	font-size: 100%;
}
code {
	font-family: ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace;
	font-size: 85%;
}
p code, li code, td code, :not(pre) > code {
	padding: .2em .4em;
	border-radius: 6px;
	background: var(--inline-code);
	color: inherit;
}
blockquote {
	margin: 0 0 16px;
	padding: 0 1em;
	border-left: .25em solid var(--line);
	color: var(--muted);
}
ul, ol {
	margin-top: 0;
	margin-bottom: 16px;
	padding-left: 2em;
}
.task-list, .contains-task-list {
	list-style: none;
	padding-left: 0;
}
.task-list input, .contains-task-list input { margin-right: .55em; }
del { color: var(--muted); }
hr {
	border: 0;
	border-top: 1px solid var(--line);
	margin: 24px 0;
}
table {
	width: 100%;
	border-collapse: collapse;
	margin: 16px 0;
	overflow: hidden;
}
th, td {
	padding: 6px 13px;
	border: 1px solid var(--line);
	text-align: left;
	vertical-align: top;
}
th {
	background: var(--soft);
	font-weight: 600;
}
tr:nth-child(even) { background: var(--soft); }
pre.chroma, .highlight pre {
	background: var(--code) !important;
	color: var(--codefg);
	border-radius: 6px;
	padding: 16px;
	overflow-x: auto;
}
.chroma { color: var(--codefg); }
.chroma .k, .chroma .kd, .chroma .kn { color: #cf222e; }
.chroma .s, .chroma .s1, .chroma .s2, .chroma .nt { color: #0a3069; }
.chroma .c, .chroma .c1, .chroma .cm { color: #6e7781; }
.chroma .nf, .chroma .nx, .chroma .na { color: #8250df; }
.chroma .mi, .chroma .mf { color: #0550ae; }
.chroma .p { color: var(--codefg); }
pre.mermaid {
	background: var(--bg);
	color: var(--fg);
	text-align: center;
}
.mermaid svg {
	display: block;
	max-width: 100%;
	height: auto;
	margin: 0 auto;
	background: var(--bg);
}
.markdown-math-inline {
	display: inline-block;
	padding: .1em .3em;
	border-radius: 6px;
	background: var(--inline-code);
	color: inherit;
	font-family: Georgia, serif;
}
.katex-display {
	margin: 16px 0;
	overflow-x: auto;
	overflow-y: hidden;
}
.katex-error {
	color: var(--danger);
	background: #ffebe9;
	padding: .2em .4em;
	border-radius: 6px;
}
@media (prefers-color-scheme: dark) {
	:root {
		--bg: #0d1117;
		--fg: #e6edf3;
		--heading: #f0f6fc;
		--muted: #9198a1;
		--brand: #2f81f7;
		--line: #3d444d;
		--soft: #161b22;
		--code: #161b22;
		--codefg: #e6edf3;
		--inline-code: rgba(110, 118, 129, .4);
		--danger: #ff7b72;
	}
	h1, h2 { border-bottom-color: var(--line); }
	.chroma .k, .chroma .kd, .chroma .kn { color: #ff7b72; }
	.chroma .s, .chroma .s1, .chroma .s2, .chroma .nt { color: #a5d6ff; }
	.chroma .c, .chroma .c1, .chroma .cm { color: #8b949e; }
	.chroma .nf, .chroma .nx, .chroma .na { color: #d2a8ff; }
	.chroma .mi, .chroma .mf { color: #79c0ff; }
	.katex-error { background: rgba(248, 81, 73, .15); }
}
`

const markdownThemeStyle = `
html[data-theme="light"] {
	--bg: #ffffff;
	--fg: #1f2328;
	--heading: #1f2328;
	--muted: #57606a;
	--brand: #0969da;
	--line: #d0d7de;
	--soft: #f6f8fa;
	--code: #f6f8fa;
	--codefg: #1f2328;
	--inline-code: rgba(175, 184, 193, .2);
	--danger: #cf222e;
}
html[data-theme="dark"] {
	--bg: #0d1117;
	--fg: #e6edf3;
	--heading: #f0f6fc;
	--muted: #9198a1;
	--brand: #2f81f7;
	--line: #3d444d;
	--soft: #161b22;
	--code: #161b22;
	--codefg: #e6edf3;
	--inline-code: rgba(110, 118, 129, .4);
	--danger: #ff7b72;
}
html[data-theme="dark"] .chroma .k,
html[data-theme="dark"] .chroma .kd,
html[data-theme="dark"] .chroma .kn { color: #ff7b72; }
html[data-theme="dark"] .chroma .s,
html[data-theme="dark"] .chroma .s1,
html[data-theme="dark"] .chroma .s2,
html[data-theme="dark"] .chroma .nt { color: #a5d6ff; }
html[data-theme="dark"] .chroma .c,
html[data-theme="dark"] .chroma .c1,
html[data-theme="dark"] .chroma .cm { color: #8b949e; }
html[data-theme="dark"] .chroma .nf,
html[data-theme="dark"] .chroma .nx,
html[data-theme="dark"] .chroma .na { color: #d2a8ff; }
html[data-theme="dark"] .chroma .mi,
html[data-theme="dark"] .chroma .mf { color: #79c0ff; }
html[data-theme="dark"] .katex-error { background: rgba(248, 81, 73, .15); }
`

const markdownSourceStyle = `
.markdown-source {
	display: none;
	max-width: 960px;
	margin: 0 auto;
	padding: 48px 22px 96px;
	white-space: pre-wrap;
	word-break: break-word;
	overflow: auto;
	background: transparent;
	color: var(--fg);
	box-shadow: none;
	border-radius: 0;
	font-size: 14px;
	line-height: 1.72;
}
.markdown-floating-tools {
	position: fixed;
	right: 18px;
	bottom: 18px;
	z-index: 30;
	display: inline-flex;
	gap: 8px;
	align-items: center;
}
.markdown-view-toggle,
.markdown-theme-toggle {
	width: 42px;
	height: 42px;
	display: grid;
	place-items: center;
	border: 1px solid rgba(15, 118, 158, .20);
	border-radius: 999px;
	background: rgba(255, 255, 255, .66);
	color: #075985;
	box-shadow: 0 10px 28px rgba(15, 23, 42, .12);
	backdrop-filter: blur(12px);
	opacity: .54;
	cursor: pointer;
	transition: opacity .16s ease, transform .16s ease, background .16s ease;
}
.markdown-view-toggle:hover,
.markdown-view-toggle:focus-visible,
.markdown-theme-toggle:hover,
.markdown-theme-toggle:focus-visible {
	opacity: 1;
	transform: translateY(-1px);
	background: rgba(255, 255, 255, .92);
	outline: none;
}
.markdown-view-toggle span,
.markdown-theme-toggle span {
	font: 800 11px/1 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
	letter-spacing: 0;
}
html[data-markdown-view="source"] .markdown-body { display: none; }
html[data-markdown-view="source"] .markdown-source { display: block; }
html[data-markdown-view="source"] .markdown-view-toggle {
	color: #0f172a;
	border-color: rgba(15, 23, 42, .18);
}
html[data-theme="dark"] .markdown-view-toggle {
	background: rgba(15, 23, 42, .62);
	color: #7dd3fc;
	border-color: rgba(125, 211, 252, .20);
	box-shadow: 0 10px 28px rgba(0, 0, 0, .32);
}
html[data-theme="dark"] .markdown-theme-toggle {
	background: rgba(15, 23, 42, .62);
	color: #facc15;
	border-color: rgba(250, 204, 21, .20);
	box-shadow: 0 10px 28px rgba(0, 0, 0, .32);
}
html[data-theme="dark"] .markdown-view-toggle:hover,
html[data-theme="dark"] .markdown-view-toggle:focus-visible,
html[data-theme="dark"] .markdown-theme-toggle:hover,
html[data-theme="dark"] .markdown-theme-toggle:focus-visible {
	background: rgba(15, 23, 42, .88);
}
@media (max-width: 640px) {
	.markdown-floating-tools {
		right: 12px;
		bottom: 12px;
		gap: 6px;
	}
	.markdown-view-toggle,
	.markdown-theme-toggle {
		width: 38px;
		height: 38px;
	}
	.markdown-source {
		padding: 34px 16px 82px;
		font-size: 13px;
	}
}
`

const markdownPageTemplate = `<!DOCTYPE html>
<html lang="zh-CN" data-theme="{{theme}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{title}}</title>
<style nonce="{{nonce}}">{{style}}</style>
<link rel="stylesheet" href="/markdown-assets/katex/katex.min.css">
<script defer src="/markdown-assets/katex/katex.min.js" nonce="{{nonce}}"></script>
<script defer src="/markdown-assets/katex/contrib/auto-render.min.js" nonce="{{nonce}}"></script>
<script defer src="/markdown-assets/mermaid/mermaid.min.js" nonce="{{nonce}}"></script>
<script defer nonce="{{nonce}}">{{runtime}}</script>
</head>
<body>
<div class="markdown-body">{{content}}</div>
<pre class="markdown-source" aria-label="Markdown 原文">{{source}}</pre>
<div class="markdown-floating-tools" aria-label="Markdown 显示控制">
<button class="markdown-view-toggle" type="button" aria-label="查看 Markdown 原文" title="查看原文"><span>MD</span></button>
<button class="markdown-theme-toggle" type="button" aria-label="切换浅色主题" title="切换浅色主题"><span>亮</span></button>
</div>
</body>
</html>`

const (
	DefaultMarkdownTheme     = "dark"
	MarkdownRendererVersion  = "markdown-runtime-v7"
	MarkdownNoncePlaceholder = "PAGEPILOT_MARKDOWN_NONCE"
)

// MarkdownToHTML renders a hosted Markdown document into a standalone HTML page.
func MarkdownToHTML(body []byte) string {
	return MarkdownToHTMLWithTheme(body, DefaultMarkdownTheme)
}

// MarkdownToHTMLWithTheme renders Markdown with an explicit light/dark/auto theme marker.
func MarkdownToHTMLWithTheme(body []byte, theme string) string {
	theme = NormalizeMarkdownTheme(theme)
	original := string(body)
	source, replacements := extractMarkdownSpecialBlocks(original)
	source, inlineReplacements := extractInlineMath(source, len(replacements))
	replacements = append(replacements, inlineReplacements...)

	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			highlighting.NewHighlighting(
				highlighting.WithStyle("github"),
				highlighting.WithGuessLanguage(true),
				highlighting.WithFormatOptions(chromaHTML.WithClasses(true)),
			),
		),
		goldmark.WithRendererOptions(
			goldmarkHTML.WithHardWraps(),
		),
	)

	var rendered bytes.Buffer
	if err := md.Convert([]byte(source), &rendered); err != nil {
		return wrapMarkdownPage("<p>Markdown 渲染失败。</p>", theme, original)
	}
	htmlBody := sanitizeMarkdownHTML(rendered.String())
	for _, item := range replacements {
		htmlBody = strings.ReplaceAll(htmlBody, "<p>"+item.Token+"</p>", item.HTML)
		htmlBody = strings.ReplaceAll(htmlBody, item.Token, item.HTML)
	}
	return wrapMarkdownPage(htmlBody, theme, original)
}

func NormalizeMarkdownTheme(theme string) string {
	switch strings.ToLower(strings.TrimSpace(theme)) {
	case "light", "dark", "auto":
		return strings.ToLower(strings.TrimSpace(theme))
	default:
		return DefaultMarkdownTheme
	}
}

func wrapMarkdownPage(body string, theme string, source string) string {
	theme = NormalizeMarkdownTheme(theme)
	return applyMarkdownTemplate("Markdown", body, theme, source)
}

func applyMarkdownTemplate(title string, content string, theme string, source string) string {
	style := markdownPageStyle + markdownThemeStyle + markdownSourceStyle
	replacer := strings.NewReplacer(
		"{{theme}}", html.EscapeString(theme),
		"{{title}}", html.EscapeString(title),
		"{{nonce}}", MarkdownNoncePlaceholder,
		"{{style}}", style,
		"{{content}}", content,
		"{{source}}", html.EscapeString(source),
		"{{runtime}}", markdownRuntimeScript(),
	)
	return replacer.Replace(markdownPageTemplate)
}

func ApplyMarkdownNonce(pageHTML, nonce string) string {
	return strings.ReplaceAll(pageHTML, MarkdownNoncePlaceholder, html.EscapeString(nonce))
}

func markdownRuntimeScript() string {
	return `(function(){
function ready(fn){if(document.readyState==='loading'){document.addEventListener('DOMContentLoaded',fn,{once:true});}else{fn();}}
ready(function(){
 var root=document.documentElement;
 var toggle=document.querySelector('.markdown-view-toggle');
 var themeToggle=document.querySelector('.markdown-theme-toggle');
 function currentTheme(){
  var theme=root.getAttribute('data-theme')||'dark';
  if(theme==='auto'){
   return window.matchMedia&&window.matchMedia('(prefers-color-scheme: light)').matches?'light':'dark';
  }
  return theme==='light'?'light':'dark';
 }
 function mermaidVariables(theme){
  return theme==='dark'?
   {background:'#0d1117',primaryColor:'#161b22',primaryTextColor:'#e6edf3',primaryBorderColor:'#38bdf8',lineColor:'#58a6ff',secondaryColor:'#0f766e',tertiaryColor:'#172554',clusterBkg:'#0d1117',clusterBorder:'#3d444d'}:
   {background:'#ffffff',primaryColor:'#e0f2fe',primaryTextColor:'#0f172a',primaryBorderColor:'#0284c7',lineColor:'#0284c7',secondaryColor:'#dcfce7',tertiaryColor:'#eef2ff',clusterBkg:'#ffffff',clusterBorder:'#93c5fd'};
 }
 function renderMermaid(){
  if(!window.mermaid){return;}
  var theme=currentTheme();
  window.mermaid.initialize({startOnLoad:false,securityLevel:'strict',theme:'base',themeVariables:mermaidVariables(theme)});
  var nodes=Array.prototype.slice.call(document.querySelectorAll('.mermaid')).filter(restoreMermaidSource);
  if(!nodes.length){return;}
  window.mermaid.run({nodes:nodes}).catch(function(err){console.error('PagePilot Mermaid render failed',err);});
 }
 function decodeMermaidSource(value){
  if(!value){return '';}
  try{return decodeURIComponent(escape(window.atob(value)));}catch(err){
   try{return window.atob(value);}catch(innerErr){return '';}
  }
 }
 function encodeMermaidSource(value){
  try{return window.btoa(unescape(encodeURIComponent(value)));}catch(err){return '';}
 }
 function restoreMermaidSource(el){
  var source=decodeMermaidSource(el.getAttribute('data-pagepilot-mermaid-source'));
  if(!source){
   source=el.textContent||'';
   if(!source.trim()||source.trim().charAt(0)==='<'){return false;}
   var encoded=encodeMermaidSource(source);
   if(encoded){el.setAttribute('data-pagepilot-mermaid-source',encoded);}
  }
  el.removeAttribute('data-processed');
  el.textContent=source;
  return true;
 }
 function setMarkdownView(view){
  var source=view==='source';
  root.setAttribute('data-markdown-view',source?'source':'rendered');
  if(toggle){
   toggle.setAttribute('aria-label',source?'查看渲染结果':'查看 Markdown 原文');
   toggle.setAttribute('title',source?'查看渲染结果':'查看原文');
   var label=toggle.querySelector('span');
   if(label){label.textContent=source?'HTML':'MD';}
  }
 }
 function setMarkdownTheme(theme){
  var next=theme==='light'?'light':'dark';
  root.setAttribute('data-theme',next);
  if(themeToggle){
   var toLight=next==='dark';
   themeToggle.setAttribute('aria-label',toLight?'切换浅色主题':'切换暗色主题');
   themeToggle.setAttribute('title',toLight?'切换浅色主题':'切换暗色主题');
   var label=themeToggle.querySelector('span');
   if(label){label.textContent=toLight?'亮':'暗';}
  }
  renderMermaid();
 }
 if(toggle){
  toggle.addEventListener('click',function(){
   setMarkdownView(root.getAttribute('data-markdown-view')==='source'?'rendered':'source');
  });
  setMarkdownView('rendered');
 }
 if(themeToggle){
  themeToggle.addEventListener('click',function(){
   setMarkdownTheme(currentTheme()==='dark'?'light':'dark');
  });
  setMarkdownTheme(currentTheme());
 } else {
  renderMermaid();
 }
 if(window.renderMathInElement){
  window.renderMathInElement(document.body,{throwOnError:false,strict:false,ignoredClasses:['markdown-source'],delimiters:[{left:'$$',right:'$$',display:true},{left:'$',right:'$',display:false}]});
 }
});
})();`
}

type markdownReplacement struct {
	Token string
	HTML  string
}

func extractMarkdownSpecialBlocks(input string) (string, []markdownReplacement) {
	lines := strings.Split(strings.ReplaceAll(input, "\r\n", "\n"), "\n")
	var out strings.Builder
	var replacements []markdownReplacement
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "```") {
			lang := cleanMarkdownFenceLang(strings.TrimSpace(strings.TrimPrefix(trimmed, "```")))
			if isSpecialFenceLang(lang) {
				var block []string
				for i++; i < len(lines); i++ {
					if strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
						break
					}
					block = append(block, lines[i])
				}
				token := markdownToken(len(replacements))
				replacements = append(replacements, markdownReplacement{Token: token, HTML: specialFenceHTML(lang, strings.Join(block, "\n"))})
				out.WriteString("\n" + token + "\n")
				continue
			}
		}
		if strings.HasPrefix(trimmed, "$$") && strings.HasSuffix(trimmed, "$$") && len(trimmed) > 4 {
			inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "$$"), "$$"))
			if inner != "" {
				token := markdownToken(len(replacements))
				replacements = append(replacements, markdownReplacement{Token: token, HTML: mathBlockHTML(inner)})
				out.WriteString("\n" + token + "\n")
				continue
			}
		}
		if trimmed == "$$" {
			var block []string
			for i++; i < len(lines); i++ {
				if strings.TrimSpace(lines[i]) == "$$" {
					break
				}
				block = append(block, lines[i])
			}
			token := markdownToken(len(replacements))
			replacements = append(replacements, markdownReplacement{Token: token, HTML: mathBlockHTML(strings.Join(block, "\n"))})
			out.WriteString("\n" + token + "\n")
			continue
		}
		out.WriteString(lines[i])
		if i < len(lines)-1 {
			out.WriteByte('\n')
		}
	}
	return out.String(), replacements
}

func extractInlineMath(input string, offset int) (string, []markdownReplacement) {
	lines := strings.Split(input, "\n")
	var replacements []markdownReplacement
	var out strings.Builder
	inFence := false
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			out.WriteString(line)
			inFence = !inFence
		} else if inFence {
			out.WriteString(line)
		} else {
			rendered, lineReplacements := extractInlineMathText(line, offset+len(replacements))
			out.WriteString(rendered)
			replacements = append(replacements, lineReplacements...)
		}
		if index < len(lines)-1 {
			out.WriteByte('\n')
		}
	}
	return out.String(), replacements
}

func extractInlineMathText(input string, offset int) (string, []markdownReplacement) {
	var replacements []markdownReplacement
	var out strings.Builder
	for i := 0; i < len(input); i++ {
		if input[i] == '`' {
			run := markdownBacktickRunLength(input, i)
			marker := strings.Repeat("`", run)
			searchStart := i + run
			if endRel := strings.Index(input[searchStart:], marker); endRel >= 0 {
				end := searchStart + endRel + run
				out.WriteString(input[i:end])
				i = end - 1
				continue
			}
			out.WriteString(input[i:searchStart])
			i = searchStart - 1
			continue
		}
		if input[i] != '$' || markdownDollarEscaped(input, i) || markdownDollarIsDouble(input, i) {
			out.WriteByte(input[i])
			continue
		}
		end := findInlineMathEnd(input, i+1)
		if end == -1 {
			out.WriteByte(input[i])
			continue
		}
		inner := input[i+1 : end]
		if strings.TrimSpace(inner) == "" || strings.Contains(inner, "\n") {
			out.WriteString(input[i : end+1])
			i = end
			continue
		}
		match := input[i : end+1]
		token := markdownToken(offset + len(replacements))
		replacements = append(replacements, markdownReplacement{
			Token: token,
			HTML:  `<span class="markdown-math-inline" data-pagepilot-math-inline>` + html.EscapeString(match) + `</span>`,
		})
		out.WriteString(token)
		i = end
	}
	return out.String(), replacements
}

func markdownBacktickRunLength(input string, index int) int {
	run := 0
	for index+run < len(input) && input[index+run] == '`' {
		run++
	}
	return run
}

func findInlineMathEnd(input string, start int) int {
	for i := start; i < len(input); i++ {
		if input[i] == '\n' {
			return -1
		}
		if input[i] == '$' && !markdownDollarEscaped(input, i) && !markdownDollarIsDouble(input, i) {
			return i
		}
	}
	return -1
}

func markdownDollarEscaped(input string, index int) bool {
	slashes := 0
	for i := index - 1; i >= 0 && input[i] == '\\'; i-- {
		slashes++
	}
	return slashes%2 == 1
}

func markdownDollarIsDouble(input string, index int) bool {
	return (index > 0 && input[index-1] == '$') || (index+1 < len(input) && input[index+1] == '$')
}

func markdownToken(index int) string {
	return fmt.Sprintf("PAGEPILOTMDTOKEN%d", index)
}

func isSpecialFenceLang(lang string) bool {
	switch strings.ToLower(lang) {
	case "mermaid", "math", "katex", "latex":
		return true
	default:
		return false
	}
}

func specialFenceHTML(lang, body string) string {
	switch strings.ToLower(lang) {
	case "mermaid":
		source := strings.TrimSpace(body)
		encoded := base64.StdEncoding.EncodeToString([]byte(source))
		return `<pre class="mermaid" data-pagepilot-mermaid-source="` + html.EscapeString(encoded) + `">` +
			html.EscapeString(source) +
			`</pre>`
	default:
		return mathBlockHTML(body)
	}
}

func mathBlockHTML(body string) string {
	return `<div class="katex-display" data-pagepilot-math-block>` +
		html.EscapeString("$$\n"+strings.TrimSpace(body)+"\n$$") +
		`</div>`
}

func cleanMarkdownFenceLang(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if lang == "" {
		return ""
	}
	if fields := strings.Fields(lang); len(fields) > 0 {
		lang = fields[0]
	}
	if idx := strings.IndexAny(lang, "{[("); idx > 0 {
		lang = lang[:idx]
	}
	var out strings.Builder
	for _, r := range lang {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func sanitizeMarkdownHTML(in string) string {
	in = markdownSrcsetAttrRe.ReplaceAllStringFunc(in, func(attr string) string {
		parts := markdownSrcsetAttrRe.FindStringSubmatch(attr)
		if len(parts) != 2 {
			return ""
		}
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, `"'`)
		if !safeMarkdownSrcset(value) {
			return ""
		}
		return ` srcset="` + value + `"`
	})
	in = markdownURLAttrRe.ReplaceAllStringFunc(in, func(attr string) string {
		parts := markdownURLAttrRe.FindStringSubmatch(attr)
		if len(parts) != 3 {
			return ""
		}
		name := strings.ToLower(parts[1])
		baseName := name
		if idx := strings.LastIndex(baseName, ":"); idx >= 0 {
			baseName = baseName[idx+1:]
		}
		value := strings.TrimSpace(parts[2])
		value = strings.Trim(value, `"'`)
		if !safeMarkdownURL(value, baseName == "src") {
			return ""
		}
		return ` ` + name + `="` + value + `"`
	})
	return eventHandlerAttrRe.ReplaceAllString(in, "")
}

var (
	markdownURLAttrRe    = regexp.MustCompile(`(?i)\s((?:[a-z0-9_-]+:)?(?:href|src))\s*=\s*("[^"]*"|'[^']*'|[^\s"'=<>]+)`)
	markdownSrcsetAttrRe = regexp.MustCompile(`(?i)\ssrcset\s*=\s*("[^"]*"|'[^']*'|[^\s"'=<>]+)`)
	eventHandlerAttrRe   = regexp.MustCompile(`(?i)\son[a-z][a-z0-9_:-]*\s*=\s*("[^"]*"|'[^']*'|[^\s"'=<>]+)`)
)

func safeMarkdownURL(raw string, image bool) bool {
	decoded := strings.TrimSpace(html.UnescapeString(raw))
	if decoded == "" || strings.ContainsAny(decoded, "\"'<>") {
		return false
	}
	normalized := stripMarkdownURLSchemeIgnoredChars(decoded)
	lower := strings.ToLower(normalized)
	if strings.HasPrefix(lower, "#") || strings.HasPrefix(lower, "./") || strings.HasPrefix(lower, "../") || strings.HasPrefix(lower, "/") {
		return true
	}
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return true
	}
	if !image && (strings.HasPrefix(lower, "mailto:") || strings.HasPrefix(lower, "tel:")) {
		return true
	}
	return !strings.Contains(lower, ":")
}

func safeMarkdownSrcset(raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return false
	}
	for _, candidate := range strings.Split(raw, ",") {
		fields := strings.Fields(strings.TrimSpace(candidate))
		if len(fields) == 0 {
			return false
		}
		if !safeMarkdownURL(fields[0], true) {
			return false
		}
		for _, descriptor := range fields[1:] {
			if strings.ContainsAny(descriptor, "\"'<>") {
				return false
			}
		}
	}
	return true
}

func stripMarkdownURLSchemeIgnoredChars(raw string) string {
	return strings.Map(func(r rune) rune {
		if r <= ' ' || r == 0x7f {
			return -1
		}
		return r
	}, raw)
}
