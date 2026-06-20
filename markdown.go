package main

// # h1
// ## h2
// ### h3
// 	> blockquote
// 1. First item
// - First item
// ---
// **bold text**
// *italicized text*
// `code`
// [title](https://www.example.com)
// ![alt text](image.jpg)

const (
	text int8 = iota
	h1
	h2
	h3
	blockquote
	horizzontalRule

	linkText
	linkURL
	imageAlt
	imageURL
	code
	bold
)

func parseMarkdown(l []byte) (out []byte) {
	state := text
	n := len(l)
	i := 0
	offset := 0
	for i < n {
		switch state {
		case text:
			// Headings
			if i == 0 && i+2 < n && l[0] == '#' {
				hlevel := 1
				for hlevel < n && l[hlevel] == '#' {
					hlevel++
				}
				switch hlevel {
				case 1:
					state = h1
					offset = 2
					i++
				case 2:
					state = h2
					offset = 3
					i = i + 2
				case 3:
					state = h3
					offset = 4
					i = i + 3
				}
			} else if i == 0 && i+3 < n && l[0] == '-' && l[1] == '-' && l[2] == '-' {
				state = blockquote
				offset = 4
				i = i + 3
			} else {
				i++
			}
		case h1:
			return wrapTag(out, "h1", l[offset:])
		case h2:
			return wrapTag(out, "h2", l[offset:])
		case h3:
			return wrapTag(out, "h3", l[offset:])
		case blockquote:
			return wrapTag(out, "blockquote", l[offset:])
		}
	}
	return wrapTag(out, "p", l)
}

func wrapTag(out []byte, tag string, content []byte) []byte {
	out = append(out, '<')
	out = append(out, tag...)
	out = append(out, '>')
	out = append(out, content...)
	out = append(out, '<', '/')
	out = append(out, tag...)
	out = append(out, '>')
	return out
}
