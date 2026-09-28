package main

import (
	"html"
	"strconv"
	"strings"
)

// Supported Markdown:
//   # h1 / ## h2 / ### h3
//   > blockquote
//   1. ordered list   - / * unordered list
//   --- horizontal rule
//   ``` fenced code block
//   **bold**  *italic*  `code`
//   [title](url)  ![alt](url)
//
// Parsing is two-phase: parseMD walks the lines and groups them into blocks;
// each block's text is then handed to parseInline for span-level markup.

// parseMD turns a Markdown body into HTML one block at a time. A block spans one
// or more consecutive lines; a blank line is a block separator.
func parseMD(md string) string {
	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	var b strings.Builder
	for i := 0; i < len(lines); {
		switch {
		case isBlank(lines[i]):
			i++
		case isHeading(lines[i]):
			b.WriteString(heading(lines[i]))
			i++
		case isHR(lines[i]):
			b.WriteString("<hr>")
			i++
		case isFence(lines[i]):
			i = codeBlock(&b, lines, i)
		case isQuote(lines[i]):
			i = blockquote(&b, lines, i)
		case isUL(lines[i]):
			i = list(&b, lines, i, "ul", isUL, ulContent)
		case isOL(lines[i]):
			i = list(&b, lines, i, "ol", isOL, olContent)
		default:
			i = paragraph(&b, lines, i)
		}
	}
	return b.String()
}

// --- block classifiers -------------------------------------------------------

func isBlank(line string) bool { return strings.TrimSpace(line) == "" }

func isHeading(line string) bool {
	n := 0
	for n < len(line) && line[n] == '#' {
		n++
	}
	return n >= 1 && n <= 3 && n < len(line) && line[n] == ' '
}

func isHR(line string) bool {
	s := strings.TrimSpace(line)
	return len(s) >= 3 && strings.Trim(s, "-") == ""
}

func isFence(line string) bool { return strings.HasPrefix(strings.TrimSpace(line), "```") }

func isQuote(line string) bool { return strings.HasPrefix(line, ">") }

func isUL(line string) bool {
	return strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ")
}

func isOL(line string) bool {
	n := 0
	for n < len(line) && line[n] >= '0' && line[n] <= '9' {
		n++
	}
	return n > 0 && n+1 < len(line) && line[n] == '.' && line[n+1] == ' '
}

// startsBlock reports whether a line opens a new block, so a paragraph knows to
// stop even when no blank line separates it from what follows.
func startsBlock(line string) bool {
	return isHeading(line) || isHR(line) || isFence(line) ||
		isQuote(line) || isUL(line) || isOL(line)
}

// --- block renderers ---------------------------------------------------------

func heading(line string) string {
	n := 0
	for line[n] == '#' {
		n++
	}
	tag := "h" + strconv.Itoa(n)
	content := parseInline([]byte(strings.TrimSpace(line[n:])))
	return "<" + tag + ">" + string(content) + "</" + tag + ">"
}

// codeBlock renders a fenced block: content is emitted verbatim (escaped) with
// no inline parsing, and any info string after the opening ``` is ignored.
// Returns the index past the closing fence.
func codeBlock(b *strings.Builder, lines []string, i int) int {
	i++ // skip opening fence
	start := i
	for i < len(lines) && !isFence(lines[i]) {
		i++
	}
	b.WriteString("<pre><code>")
	b.WriteString(html.EscapeString(strings.Join(lines[start:i], "\n")))
	b.WriteString("</code></pre>")
	if i < len(lines) {
		i++ // skip closing fence
	}
	return i
}

// blockquote consumes consecutive "> " lines and renders their content as
// nested blocks, so a multi-paragraph quote works.
func blockquote(b *strings.Builder, lines []string, i int) int {
	var inner []string
	for i < len(lines) && isQuote(lines[i]) {
		s := strings.TrimPrefix(lines[i], ">")
		inner = append(inner, strings.TrimPrefix(s, " "))
		i++
	}
	b.WriteString("<blockquote>")
	b.WriteString(parseMD(strings.Join(inner, "\n")))
	b.WriteString("</blockquote>")
	return i
}

// list consumes consecutive item lines matching match and renders them as <li>
// inside tag ("ul"/"ol"); content strips an item's marker.
func list(b *strings.Builder, lines []string, i int, tag string, match func(string) bool, content func(string) string) int {
	b.WriteString("<")
	b.WriteString(tag)
	b.WriteString(">")
	for i < len(lines) && match(lines[i]) {
		b.WriteString("<li>")
		b.Write(parseInline([]byte(strings.TrimSpace(content(lines[i])))))
		b.WriteString("</li>")
		i++
	}
	b.WriteString("</")
	b.WriteString(tag)
	b.WriteString(">")
	return i
}

func ulContent(line string) string { return line[2:] } // after "- " / "* "

func olContent(line string) string {
	dot := strings.IndexByte(line, '.')
	return line[dot+2:] // after "N. "
}

// paragraph consumes consecutive lines until a blank line or the start of
// another block, joining them into a single <p>.
func paragraph(b *strings.Builder, lines []string, i int) int {
	start := i
	for i < len(lines) && !isBlank(lines[i]) && !startsBlock(lines[i]) {
		i++
	}
	b.WriteString("<p>")
	b.Write(parseInline([]byte(strings.Join(lines[start:i], " "))))
	b.WriteString("</p>")
	return i
}

// parseInline scans a block's text content for inline markup and emits the
// corresponding HTML. Anything it doesn't recognise is copied through verbatim.
// Order matters: code spans first (they suppress all markup inside), then image
// before link (the image marker is a link marker with a leading '!'), then bold
// before italic (so "**" isn't mistaken for two "*"). A marker with no valid
// closing degrades to literal text.
func parseInline(s []byte) []byte {
	var out []byte
	n := len(s)
	for i := 0; i < n; {
		switch {
		case s[i] == '`':
			if span, next, ok := parseCode(s, i); ok {
				out = append(out, span...)
				i = next
				continue
			}
		case s[i] == '!' && i+1 < n && s[i+1] == '[':
			if img, next, ok := parseImage(s, i); ok {
				out = append(out, img...)
				i = next
				continue
			}
		case s[i] == '[':
			if link, next, ok := parseLink(s, i); ok {
				out = append(out, link...)
				i = next
				continue
			}
		case i+1 < n && s[i] == '*' && s[i+1] == '*':
			if em, next, ok := parseEmphasis(s, i, "**", "strong"); ok {
				out = append(out, em...)
				i = next
				continue
			}
		case s[i] == '*':
			if em, next, ok := parseEmphasis(s, i, "*", "em"); ok {
				out = append(out, em...)
				i = next
				continue
			}
		}
		out = append(out, s[i])
		i++
	}
	return out
}

// parseLinkSpan parses the shared "[text](url)" shape. bracket is the index of
// the opening '['. It returns the inner text, the url, the index just past the
// closing ')', and ok=false (leaving the caller to treat '[' as literal) when
// the span is malformed.
func parseLinkSpan(s []byte, bracket int) (text, url []byte, next int, ok bool) {
	i := bracket + 1 // skip '['
	textStart := i
	// Balance nested '[' ']' so a link can wrap an image: [![alt](img)](url).
	depth := 1
	for i < len(s) {
		if s[i] == '[' {
			depth++
		} else if s[i] == ']' {
			depth--
			if depth == 0 {
				break
			}
		}
		i++
	}
	if i >= len(s) {
		return nil, nil, 0, false // no matching closing ']'
	}
	text = s[textStart:i]
	i++ // skip ']'
	if i >= len(s) || s[i] != '(' {
		return nil, nil, 0, false // not followed by '('
	}
	i++ // skip '('
	urlStart := i
	for i < len(s) && s[i] != ')' {
		i++
	}
	if i >= len(s) {
		return nil, nil, 0, false // no closing ')'
	}
	url = s[urlStart:i]
	i++ // skip ')'
	return text, url, i, true
}

// parseImage parses an image span starting at start (where s[start] == '!' and
// s[start+1] == '['). On success it returns <img src="url" alt="alt"> and the
// index past the closing ')'.
func parseImage(s []byte, start int) (out []byte, next int, ok bool) {
	alt, url, next, ok := parseLinkSpan(s, start+1) // '[' sits at start+1
	if !ok {
		return nil, 0, false
	}
	out = append(out, `<img src="`...)
	out = append(out, html.EscapeString(string(url))...)
	out = append(out, `" alt="`...)
	out = append(out, html.EscapeString(string(alt))...)
	out = append(out, `">`...)
	return out, next, true
}

// parseLink parses a link span starting at start (where s[start] == '['). On
// success it returns <a href="url">text</a> and the index past the closing ')'.
// The link text is run back through parseInline so nested markup (e.g. a linked
// image) is rendered too.
func parseLink(s []byte, start int) (out []byte, next int, ok bool) {
	text, url, next, ok := parseLinkSpan(s, start)
	if !ok {
		return nil, 0, false
	}
	out = append(out, `<a href="`...)
	out = append(out, html.EscapeString(string(url))...)
	out = append(out, `">`...)
	out = append(out, parseInline(text)...)
	out = append(out, `</a>`...)
	return out, next, true
}

// parseCode parses an inline code span `...` starting at start (s[start] == '`').
// Its content is emitted verbatim (escaped) with no further inline parsing, so a
// code span is a hard boundary. Returns ok=false (literal backtick) if unclosed.
func parseCode(s []byte, start int) (out []byte, next int, ok bool) {
	i := start + 1 // skip opening '`'
	contentStart := i
	for i < len(s) && s[i] != '`' {
		i++
	}
	if i >= len(s) {
		return nil, 0, false // no closing '`'
	}
	content := s[contentStart:i]
	i++ // skip closing '`'
	out = append(out, "<code>"...)
	out = append(out, html.EscapeString(string(content))...)
	out = append(out, "</code>"...)
	return out, i, true
}

// parseEmphasis parses a span delimited by marker ("*" or "**") starting at
// start, wrapping the inner text in <tag>. The inner text is run back through
// parseInline so nested markup works. Returns ok=false (literal) if the marker
// has no closing match or wraps no content.
func parseEmphasis(s []byte, start int, marker, tag string) (out []byte, next int, ok bool) {
	mlen := len(marker)
	contentStart := start + mlen
	for i := contentStart; i+mlen <= len(s); i++ {
		if string(s[i:i+mlen]) != marker {
			continue
		}
		inner := s[contentStart:i]
		if len(inner) == 0 {
			return nil, 0, false // empty span, e.g. "**" or "*"
		}
		out = append(out, '<')
		out = append(out, tag...)
		out = append(out, '>')
		out = append(out, parseInline(inner)...)
		out = append(out, '<', '/')
		out = append(out, tag...)
		out = append(out, '>')
		return out, i + mlen, true
	}
	return nil, 0, false // no closing marker
}
