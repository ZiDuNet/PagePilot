package render

import (
	"bytes"
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

const markdownPageStyle = `:root{color-scheme:light dark;--bg:#f8fbff;--fg:#0f172a;--muted:#5b6b84;--brand:#2563eb;--line:#dbeafe;--soft:#eff6ff;--code:#08111f;--codefg:#e2e8f0}body{margin:0;background:radial-gradient(circle at 12% 0%,#e0f7ff 0,transparent 34%),var(--bg);color:var(--fg);font:16px/1.72 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"Helvetica Neue",Arial,"PingFang SC","Microsoft YaHei",sans-serif}.page{max-width:960px;margin:0 auto;padding:48px 22px 72px}h1,h2,h3,h4,h5,h6{line-height:1.18;margin:1.45em 0 .55em;color:#0b1b3a}h1{font-size:40px}h2{font-size:30px}h3{font-size:24px}p{margin:.75em 0;color:var(--fg)}a{color:var(--brand);text-decoration:none;border-bottom:1px solid rgba(37,99,235,.28)}img{max-width:100%;height:auto;border-radius:14px;border:1px solid var(--line);box-shadow:0 16px 42px rgba(14,116,144,.12)}pre{overflow:auto;padding:16px 18px;border-radius:14px;background:var(--code);color:var(--codefg);box-shadow:0 18px 44px rgba(15,23,42,.16)}pre code{color:inherit;background:transparent;padding:0}code{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:.92em}p code,li code,td code{padding:.12em .36em;border-radius:7px;background:#e0f2fe;color:#075985}blockquote{margin:1em 0;padding:.1em 1em;border-left:4px solid #38bdf8;background:#ecfeff;color:#334155}ul,ol{padding-left:1.35em}.task-list,.contains-task-list{list-style:none;padding-left:0}.task-list input,.contains-task-list input{margin-right:.55em}del{color:var(--muted)}hr{border:0;border-top:1px solid var(--line);margin:2em 0}table{width:100%;border-collapse:separate;border-spacing:0;margin:1.1em 0;border:1px solid var(--line);border-radius:14px;overflow:hidden;background:rgba(255,255,255,.72)}th,td{padding:10px 12px;border-bottom:1px solid var(--line);text-align:left;vertical-align:top}th{background:var(--soft);font-weight:800}tr:last-child td{border-bottom:0}.chroma{background:transparent!important;color:inherit}.chroma .k{color:#93c5fd}.chroma .s,.chroma .s1,.chroma .s2{color:#86efac}.chroma .c,.chroma .c1,.chroma .cm{color:#94a3b8}.chroma .nf,.chroma .nx{color:#fbbf24}.markdown-diagram,.markdown-math{margin:1.2em 0;padding:16px;border-radius:14px;border:1px solid var(--line);background:rgba(255,255,255,.78)}.markdown-diagram figcaption,.markdown-math figcaption{font-size:12px;font-weight:800;color:var(--brand);text-transform:uppercase;letter-spacing:.08em;margin-bottom:10px}.markdown-diagram pre,.markdown-math pre{margin:0;box-shadow:none}.markdown-diagram pre.mermaid{padding:12px;background:#fff;color:#0f172a;border:1px solid var(--line);border-radius:12px}.markdown-diagram .mermaid svg{display:block;max-width:100%;height:auto;margin:0 auto;background:#fff}.markdown-math-inline{display:inline-block;padding:0 .28em;border-radius:6px;background:#eef2ff;color:#3730a3;font-family:Georgia,serif}@media (prefers-color-scheme:dark){:root{--bg:#08111f;--fg:#dbeafe;--muted:#93a4b8;--brand:#7dd3fc;--line:#1f3a5f;--soft:#0f253d;--code:#020617;--codefg:#e2e8f0}body{background:radial-gradient(circle at 12% 0%,rgba(14,165,233,.16) 0,transparent 34%),var(--bg)}h1,h2,h3,h4,h5,h6{color:#f8fbff}table,.markdown-diagram,.markdown-math{background:rgba(15,23,42,.68)}.markdown-diagram pre.mermaid{background:#0f172a;color:#e2e8f0;border-color:#1f3a5f}.markdown-diagram .mermaid svg{background:#0f172a}p code,li code,td code{background:#0c4a6e;color:#e0f2fe}.markdown-math-inline{background:#172554;color:#bfdbfe}}`

const markdownThemeStyle = `html[data-theme="light"]{--bg:#f8fbff;--fg:#0f172a;--muted:#5b6b84;--brand:#2563eb;--line:#dbeafe;--soft:#eff6ff;--code:#08111f;--codefg:#e2e8f0}html[data-theme="dark"]{--bg:#08111f;--fg:#dbeafe;--muted:#93a4b8;--brand:#7dd3fc;--line:#1f3a5f;--soft:#0f253d;--code:#020617;--codefg:#e2e8f0}html[data-theme="dark"] body{background:radial-gradient(circle at 12% 0%,rgba(14,165,233,.16) 0,transparent 34%),var(--bg)}html[data-theme="dark"] h1,html[data-theme="dark"] h2,html[data-theme="dark"] h3,html[data-theme="dark"] h4,html[data-theme="dark"] h5,html[data-theme="dark"] h6{color:#f8fbff}html[data-theme="dark"] table,html[data-theme="dark"] .markdown-diagram,html[data-theme="dark"] .markdown-math{background:rgba(15,23,42,.68)}html[data-theme="dark"] p code,html[data-theme="dark"] li code,html[data-theme="dark"] td code{background:#0c4a6e;color:#e0f2fe}html[data-theme="dark"] .markdown-math-inline{background:#172554;color:#bfdbfe}`

const (
	DefaultMarkdownTheme     = "auto"
	MarkdownRendererVersion  = "markdown-runtime-v3"
	MarkdownNoncePlaceholder = "PAGEPILOT_MARKDOWN_NONCE"
)

// MarkdownToHTML renders a hosted Markdown document into a standalone HTML page.
func MarkdownToHTML(body []byte) string {
	return MarkdownToHTMLWithTheme(body, DefaultMarkdownTheme)
}

// MarkdownToHTMLWithTheme renders Markdown with an explicit light/dark/auto theme marker.
func MarkdownToHTMLWithTheme(body []byte, theme string) string {
	theme = NormalizeMarkdownTheme(theme)
	source, replacements := extractMarkdownSpecialBlocks(string(body))
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
		return wrapMarkdownPage("<p>Markdown 渲染失败。</p>", theme)
	}
	htmlBody := sanitizeMarkdownHTML(rendered.String())
	for _, item := range replacements {
		htmlBody = strings.ReplaceAll(htmlBody, "<p>"+item.Token+"</p>", item.HTML)
		htmlBody = strings.ReplaceAll(htmlBody, item.Token, item.HTML)
	}
	return wrapMarkdownPage(htmlBody, theme)
}

func NormalizeMarkdownTheme(theme string) string {
	switch strings.ToLower(strings.TrimSpace(theme)) {
	case "light", "dark":
		return strings.ToLower(strings.TrimSpace(theme))
	default:
		return DefaultMarkdownTheme
	}
}

func wrapMarkdownPage(body string, theme string) string {
	theme = NormalizeMarkdownTheme(theme)
	return "<!doctype html><html lang=\"zh-CN\" data-theme=\"" + theme + "\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"><title>Markdown</title><style nonce=\"" +
		MarkdownNoncePlaceholder +
		"\">" +
		markdownPageStyle +
		markdownThemeStyle +
		"</style><link rel=\"stylesheet\" href=\"/markdown-assets/katex/katex.min.css\">" +
		"<script defer src=\"/markdown-assets/katex/katex.min.js\" nonce=\"" + MarkdownNoncePlaceholder + "\"></script>" +
		"<script defer src=\"/markdown-assets/katex/contrib/auto-render.min.js\" nonce=\"" + MarkdownNoncePlaceholder + "\"></script>" +
		"<script defer src=\"/markdown-assets/mermaid/mermaid.min.js\" nonce=\"" + MarkdownNoncePlaceholder + "\"></script>" +
		"<script defer nonce=\"" + MarkdownNoncePlaceholder + "\">" +
		markdownRuntimeScript() +
		"</script></head><body><main class=\"page\">" +
		body +
		"</main></body></html>"
}

func ApplyMarkdownNonce(pageHTML, nonce string) string {
	return strings.ReplaceAll(pageHTML, MarkdownNoncePlaceholder, html.EscapeString(nonce))
}

func markdownRuntimeScript() string {
	return `(function(){
function ready(fn){if(document.readyState==='loading'){document.addEventListener('DOMContentLoaded',fn,{once:true});}else{fn();}}
ready(function(){
 if(window.renderMathInElement){
  window.renderMathInElement(document.body,{throwOnError:false,strict:false,delimiters:[{left:'$$',right:'$$',display:true},{left:'$',right:'$',display:false}]});
 }
 if(window.mermaid){
  var dark=window.matchMedia&&window.matchMedia('(prefers-color-scheme: dark)').matches;
  var themeVariables=dark?
   {background:'#0f172a',primaryColor:'#1e293b',primaryTextColor:'#e2e8f0',primaryBorderColor:'#38bdf8',lineColor:'#38bdf8',secondaryColor:'#0f766e',tertiaryColor:'#172554',clusterBkg:'#0f172a',clusterBorder:'#38bdf8'}:
   {background:'#ffffff',primaryColor:'#e0f2fe',primaryTextColor:'#0f172a',primaryBorderColor:'#0284c7',lineColor:'#0284c7',secondaryColor:'#dcfce7',tertiaryColor:'#eef2ff',clusterBkg:'#ffffff',clusterBorder:'#93c5fd'};
  window.mermaid.initialize({startOnLoad:false,securityLevel:'strict',theme:'base',themeVariables:themeVariables});
  window.mermaid.run({querySelector:'.mermaid'}).catch(function(err){console.error('PagePilot Mermaid render failed',err);});
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
		return `<figure class="markdown-diagram"><figcaption>Mermaid</figcaption><pre class="mermaid">` +
			html.EscapeString(strings.TrimSpace(body)) +
			`</pre></figure>`
	default:
		return mathBlockHTML(body)
	}
}

func mathBlockHTML(body string) string {
	return `<figure class="markdown-math"><figcaption>KaTeX</figcaption><div data-pagepilot-math-block>` +
		html.EscapeString("$$\n"+strings.TrimSpace(body)+"\n$$") +
		`</div></figure>`
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
