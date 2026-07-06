package render

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

// MarkdownToHTML renders a hosted Markdown document into a standalone HTML page.
func MarkdownToHTML(body []byte) string {
	lines := strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n")
	var out strings.Builder
	out.WriteString("<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"><title>Markdown</title>")
	out.WriteString("<style>:root{color-scheme:light dark;--bg:#f8fbff;--fg:#0f172a;--muted:#5b6b84;--brand:#2563eb;--line:#dbeafe;--soft:#eff6ff;--code:#08111f;--codefg:#e2e8f0}body{margin:0;background:radial-gradient(circle at 12% 0%,#e0f7ff 0,transparent 34%),var(--bg);color:var(--fg);font:16px/1.72 -apple-system,BlinkMacSystemFont,\"Segoe UI\",sans-serif}.page{max-width:960px;margin:0 auto;padding:48px 22px 72px}h1,h2,h3,h4,h5,h6{line-height:1.18;margin:1.45em 0 .55em;color:#0b1b3a}h1{font-size:40px}h2{font-size:30px}h3{font-size:24px}p{margin:.75em 0;color:var(--fg)}a{color:var(--brand);text-decoration:none;border-bottom:1px solid rgba(37,99,235,.28)}img{max-width:100%;height:auto;border-radius:14px;border:1px solid var(--line);box-shadow:0 16px 42px rgba(14,116,144,.12)}pre{overflow:auto;padding:16px 18px;border-radius:14px;background:var(--code);color:var(--codefg);box-shadow:0 18px 44px rgba(15,23,42,.16)}pre code{color:inherit;background:transparent;padding:0}.code-title{display:block;margin:-4px 0 10px;color:#93c5fd;font-size:12px;font-weight:700;text-transform:uppercase;letter-spacing:.08em}code{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:.92em}p code,li code,td code{padding:.12em .36em;border-radius:7px;background:#e0f2fe;color:#075985}blockquote{margin:1em 0;padding:.1em 1em;border-left:4px solid #38bdf8;background:#ecfeff;color:#334155}ul,ol{padding-left:1.35em}.task-list{list-style:none;padding-left:0}.task-list input{margin-right:.55em}hr{border:0;border-top:1px solid var(--line);margin:2em 0}table{width:100%;border-collapse:separate;border-spacing:0;margin:1.1em 0;border:1px solid var(--line);border-radius:14px;overflow:hidden;background:rgba(255,255,255,.72)}th,td{padding:10px 12px;border-bottom:1px solid var(--line);text-align:left;vertical-align:top}th{background:var(--soft);font-weight:800}tr:last-child td{border-bottom:0}.markdown-diagram,.markdown-math{margin:1.2em 0;padding:16px;border-radius:14px;border:1px solid var(--line);background:rgba(255,255,255,.78)}.markdown-diagram figcaption,.markdown-math figcaption{font-size:12px;font-weight:800;color:var(--brand);text-transform:uppercase;letter-spacing:.08em;margin-bottom:10px}.markdown-diagram pre,.markdown-math pre{margin:0;box-shadow:none}@media (prefers-color-scheme:dark){:root{--bg:#08111f;--fg:#dbeafe;--muted:#93a4b8;--brand:#7dd3fc;--line:#1f3a5f;--soft:#0f253d;--code:#020617;--codefg:#e2e8f0}body{background:radial-gradient(circle at 12% 0%,rgba(14,165,233,.16) 0,transparent 34%),var(--bg)}h1,h2,h3,h4,h5,h6{color:#f8fbff}table,.markdown-diagram,.markdown-math{background:rgba(15,23,42,.68)}p code,li code,td code{background:#0c4a6e;color:#e0f2fe}}</style></head><body><main class=\"page\">")
	inCode := false
	codeLang := ""
	var codeLines []string
	var paragraph []string
	listOpen := false
	flushParagraph := func() {
		if len(paragraph) == 0 {
			return
		}
		out.WriteString("<p>")
		out.WriteString(renderMarkdownInline(strings.Join(paragraph, " ")))
		out.WriteString("</p>")
		paragraph = paragraph[:0]
	}
	closeList := func() {
		if !listOpen {
			return
		}
		out.WriteString("</ul>")
		listOpen = false
	}
	flushCode := func() {
		escaped := html.EscapeString(strings.Join(codeLines, "\n"))
		switch strings.ToLower(codeLang) {
		case "mermaid":
			out.WriteString("<figure class=\"markdown-diagram\"><figcaption>Mermaid</figcaption><pre><code class=\"language-mermaid\">")
			out.WriteString(escaped)
			out.WriteString("</code></pre></figure>")
		case "math", "katex", "latex":
			out.WriteString("<figure class=\"markdown-math\"><figcaption>KaTeX</figcaption><pre><code class=\"language-math\">")
			out.WriteString(escaped)
			out.WriteString("</code></pre></figure>")
		default:
			out.WriteString("<pre><code")
			if codeLang != "" {
				out.WriteString(" class=\"language-")
				out.WriteString(html.EscapeString(codeLang))
				out.WriteString("\"><span class=\"code-title\">")
				out.WriteString(html.EscapeString(codeLang))
				out.WriteString("</span>")
			} else {
				out.WriteString(">")
			}
			out.WriteString(escaped)
			out.WriteString("</code></pre>")
		}
		codeLines = codeLines[:0]
		codeLang = ""
	}
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			flushParagraph()
			closeList()
			if inCode {
				flushCode()
				inCode = false
			} else {
				inCode = true
				codeLang = cleanMarkdownFenceLang(strings.TrimSpace(strings.TrimPrefix(trimmed, "```")))
			}
			continue
		}
		if inCode {
			codeLines = append(codeLines, line)
			continue
		}
		if trimmed == "" {
			flushParagraph()
			closeList()
			continue
		}
		if isMarkdownTableHeader(lines, i) {
			flushParagraph()
			closeList()
			out.WriteString(renderMarkdownTable(lines[i], lines[i+2:]))
			i = markdownTableEnd(lines, i+2) - 1
			continue
		}
		if trimmed == "---" || trimmed == "***" {
			flushParagraph()
			closeList()
			out.WriteString("<hr>")
			continue
		}
		if strings.HasPrefix(trimmed, ">") {
			flushParagraph()
			closeList()
			out.WriteString("<blockquote>")
			out.WriteString(renderMarkdownInline(strings.TrimSpace(strings.TrimPrefix(trimmed, ">"))))
			out.WriteString("</blockquote>")
			continue
		}
		if level, text := markdownHeading(trimmed); level > 0 {
			flushParagraph()
			closeList()
			out.WriteString(fmt.Sprintf("<h%d>%s</h%d>", level, renderMarkdownInline(text), level))
			continue
		}
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			flushParagraph()
			if !listOpen {
				out.WriteString("<ul")
				if isTaskListItem(strings.TrimSpace(trimmed[2:])) {
					out.WriteString(" class=\"task-list\"")
				}
				out.WriteString(">")
				listOpen = true
			}
			item := strings.TrimSpace(trimmed[2:])
			out.WriteString("<li>")
			if checked, text, ok := parseTaskListItem(item); ok {
				out.WriteString("<input type=\"checkbox\" disabled")
				if checked {
					out.WriteString(" checked")
				}
				out.WriteString(">")
				out.WriteString(renderMarkdownInline(text))
			} else {
				out.WriteString(renderMarkdownInline(item))
			}
			out.WriteString("</li>")
			continue
		}
		closeList()
		paragraph = append(paragraph, trimmed)
	}
	flushParagraph()
	if inCode {
		flushCode()
	}
	closeList()
	out.WriteString("</main></body></html>")
	return out.String()
}

func markdownHeading(line string) (int, string) {
	count := 0
	for count < len(line) && count < 6 && line[count] == '#' {
		count++
	}
	if count == 0 || count >= len(line) || line[count] != ' ' {
		return 0, ""
	}
	return count, strings.TrimSpace(line[count+1:])
}

func renderMarkdownInline(text string) string {
	escaped := html.EscapeString(text)
	escaped = markdownImagesRe.ReplaceAllStringFunc(escaped, func(match string) string {
		parts := markdownImagesRe.FindStringSubmatch(match)
		if len(parts) != 3 || !safeMarkdownURL(parts[2], true) {
			return match
		}
		return `<img src="` + parts[2] + `" alt="` + parts[1] + `">`
	})
	escaped = markdownLinksRe.ReplaceAllStringFunc(escaped, func(match string) string {
		parts := markdownLinksRe.FindStringSubmatch(match)
		if len(parts) != 3 || !safeMarkdownURL(parts[2], false) {
			return match
		}
		return `<a href="` + parts[2] + `" rel="noreferrer">` + parts[1] + `</a>`
	})
	escaped = markdownCodeRe.ReplaceAllString(escaped, `<code>$1</code>`)
	return escaped
}

var (
	markdownImagesRe = regexp.MustCompile(`!\[([^\]]*)\]\(([^)\s]+)\)`)
	markdownLinksRe  = regexp.MustCompile(`\[(.*?)\]\((https?://[^)\s]+|[^)\s]+)\)`)
	markdownCodeRe   = regexp.MustCompile("`([^`]+)`")
)

func cleanMarkdownFenceLang(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if lang == "" {
		return ""
	}
	var out strings.Builder
	for _, r := range lang {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func parseTaskListItem(item string) (bool, string, bool) {
	if len(item) < 4 || item[0] != '[' || item[2] != ']' || item[3] != ' ' {
		return false, "", false
	}
	mark := item[1]
	if mark != ' ' && mark != 'x' && mark != 'X' {
		return false, "", false
	}
	return mark == 'x' || mark == 'X', strings.TrimSpace(item[4:]), true
}

func isTaskListItem(item string) bool {
	_, _, ok := parseTaskListItem(item)
	return ok
}

func isMarkdownTableHeader(lines []string, i int) bool {
	if i+1 >= len(lines) {
		return false
	}
	header := strings.TrimSpace(lines[i])
	sep := strings.TrimSpace(lines[i+1])
	return strings.Contains(header, "|") && markdownTableSeparator(sep)
}

func markdownTableSeparator(line string) bool {
	if !strings.Contains(line, "|") {
		return false
	}
	for _, cell := range splitMarkdownTableRow(line) {
		cell = strings.Trim(cell, " :-")
		if cell != "" {
			return false
		}
	}
	return true
}

func markdownTableEnd(lines []string, start int) int {
	i := start
	for i < len(lines) && strings.Contains(strings.TrimSpace(lines[i]), "|") && strings.TrimSpace(lines[i]) != "" {
		i++
	}
	return i
}

func renderMarkdownTable(header string, body []string) string {
	var out strings.Builder
	out.WriteString("<table><thead><tr>")
	for _, cell := range splitMarkdownTableRow(header) {
		out.WriteString("<th>")
		out.WriteString(renderMarkdownInline(strings.TrimSpace(cell)))
		out.WriteString("</th>")
	}
	out.WriteString("</tr></thead><tbody>")
	for _, line := range body[:markdownTableEnd(body, 0)] {
		out.WriteString("<tr>")
		for _, cell := range splitMarkdownTableRow(line) {
			out.WriteString("<td>")
			out.WriteString(renderMarkdownInline(strings.TrimSpace(cell)))
			out.WriteString("</td>")
		}
		out.WriteString("</tr>")
	}
	out.WriteString("</tbody></table>")
	return out.String()
}

func splitMarkdownTableRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	return strings.Split(line, "|")
}

func safeMarkdownURL(raw string, image bool) bool {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" || strings.ContainsAny(raw, "\"'<>") {
		return false
	}
	if strings.HasPrefix(raw, "#") || strings.HasPrefix(raw, "./") || strings.HasPrefix(raw, "../") || strings.HasPrefix(raw, "/") {
		return true
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return true
	}
	if !image && (strings.HasPrefix(raw, "mailto:") || strings.HasPrefix(raw, "tel:")) {
		return true
	}
	return !strings.Contains(raw, ":")
}
