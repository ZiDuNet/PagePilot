package render

import (
	"strings"
	"testing"
)

func TestMarkdownToHTMLAdvancedBlocksAreSafe(t *testing.T) {
	body := []byte(`# Guide

![logo](images/logo.png)
[bad](javascript:alert(1))

| Name | Value |
| --- | --- |
| HTML | <script>alert(1)</script> |

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
		`<table>`,
		`&lt;script&gt;alert(1)&lt;/script&gt;`,
		`class="task-list"`,
		`type="checkbox" disabled checked`,
		`class="language-go"`,
		`fmt.Println(&#34;&lt;safe&gt;&#34;)`,
		`class="markdown-diagram"`,
		`class="language-mermaid"`,
		`class="markdown-math"`,
		`class="language-math"`,
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
