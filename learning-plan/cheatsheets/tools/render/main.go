// Command render turns a cheatsheet Markdown file into a print-ready HTML file.
//
// It deliberately supports only the small Markdown subset the cheatsheets use
// (headings, fenced code, tables, lists, rules, and inline code/bold/links) so
// that it stays dependency-free: no module requirements, no network, just the
// standard library. render.sh then feeds the HTML to headless Chrome to get a PDF.
//
//	go run ./tools/render input.md output.html
package main

import (
	"bufio"
	"fmt"
	"html"
	"os"
	"regexp"
	"strings"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: render <input.md> <output.html>")
		os.Exit(2)
	}
	src, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "render:", err)
		os.Exit(1)
	}
	out, err := os.Create(os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, "render:", err)
		os.Exit(1)
	}
	defer out.Close()

	w := bufio.NewWriter(out)
	defer w.Flush()

	title, body := convert(string(src))
	fmt.Fprintf(w, pageTemplate, html.EscapeString(title), body)
}

// convert returns the document title (first H1) and the rendered HTML body.
func convert(src string) (string, string) {
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	var b strings.Builder
	var title string

	// State carried across lines: open <ul> and open <pre>.
	inList, inCode := false, false

	closeList := func() {
		if inList {
			b.WriteString("</ul>\n")
			inList = false
		}
	}

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Fenced code blocks swallow everything until the closing fence.
		if strings.HasPrefix(line, "```") {
			if inCode {
				b.WriteString("</code></pre>\n")
				inCode = false
			} else {
				closeList()
				b.WriteString("<pre><code>")
				inCode = true
			}
			continue
		}
		if inCode {
			b.WriteString(html.EscapeString(line) + "\n")
			continue
		}

		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			closeList()
			continue
		}

		// Horizontal rule.
		if trimmed == "---" || trimmed == "***" {
			closeList()
			b.WriteString("<hr>\n")
			continue
		}

		// Headings.
		if level, text, ok := heading(trimmed); ok {
			closeList()
			if level == 1 && title == "" {
				title = text
			}
			fmt.Fprintf(&b, "<h%d>%s</h%d>\n", level, inline(text), level)
			continue
		}

		// Tables: a header row followed by a |---|---| separator.
		if strings.HasPrefix(trimmed, "|") && i+1 < len(lines) && isTableSep(lines[i+1]) {
			closeList()
			i = table(&b, lines, i)
			continue
		}

		// Unordered list items.
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			if !inList {
				b.WriteString("<ul>\n")
				inList = true
			}
			fmt.Fprintf(&b, "<li>%s</li>\n", inline(trimmed[2:]))
			continue
		}

		closeList()
		fmt.Fprintf(&b, "<p>%s</p>\n", inline(trimmed))
	}

	if inCode {
		b.WriteString("</code></pre>\n")
	}
	closeList()
	return title, b.String()
}

func heading(s string) (level int, text string, ok bool) {
	n := 0
	for n < len(s) && s[n] == '#' {
		n++
	}
	if n == 0 || n > 6 || n >= len(s) || s[n] != ' ' {
		return 0, "", false
	}
	return n, strings.TrimSpace(s[n+1:]), true
}

func isTableSep(s string) bool {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "|") {
		return false
	}
	return strings.Trim(s, "|:- \t") == ""
}

// table renders the table starting at lines[start] and returns the index of its last line.
func table(b *strings.Builder, lines []string, start int) int {
	b.WriteString("<table>\n<thead><tr>")
	for _, cell := range row(lines[start]) {
		fmt.Fprintf(b, "<th>%s</th>", inline(cell))
	}
	b.WriteString("</tr></thead>\n<tbody>\n")

	i := start + 2 // skip the header and the |---| separator
	for ; i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "|"); i++ {
		b.WriteString("<tr>")
		for _, cell := range row(lines[i]) {
			fmt.Fprintf(b, "<td>%s</td>", inline(cell))
		}
		b.WriteString("</tr>\n")
	}
	b.WriteString("</tbody>\n</table>\n")
	return i - 1
}

func row(line string) []string {
	cells := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	return cells
}

var (
	codeRe   = regexp.MustCompile("`([^`]+)`")
	linkRe   = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	boldRe   = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	italicRe = regexp.MustCompile(`\*([^*]+)\*`)
)

// inline renders inline Markdown. Code spans are pulled out first and put back
// last so that ** and [] inside them are never treated as markup.
func inline(s string) string {
	var spans []string
	s = codeRe.ReplaceAllStringFunc(s, func(m string) string {
		spans = append(spans, html.EscapeString(m[1:len(m)-1]))
		return fmt.Sprintf("\x00%d\x00", len(spans)-1)
	})

	s = html.EscapeString(s)
	s = linkRe.ReplaceAllString(s, `<a href="$2">$1</a>`)
	s = boldRe.ReplaceAllString(s, "<strong>$1</strong>")
	s = italicRe.ReplaceAllString(s, "<em>$1</em>")

	for i, span := range spans {
		s = strings.ReplaceAll(s, fmt.Sprintf("\x00%d\x00", i), "<code>"+span+"</code>")
	}
	return s
}
