package tele

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf16"

	"github.com/gotd/td/tg"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// ParseMarkdown parses Markdown and returns plain text plus a slice of
// gotd tg.MessageEntityClass suitable for passing directly to SendMessage.
//
// preferBlockquote: if true, fenced code blocks are emitted as `> `-prefixed
// quoted lines and a MessageEntityBlockquote is attached; otherwise a
// MessageEntityPre is used (with language if detected).
//
// Offsets and Lengths are computed in UTF-16 code units (Telegram requirement).
// This function uses int for Offset/Length to match common gotd signatures.
func ParseMarkdown(md string, preferBlockquote bool) (string, []tg.MessageEntityClass, error) {
	src := []byte(md)
	parser := goldmark.DefaultParser()
	doc := parser.Parse(text.NewReader(src))

	var out bytes.Buffer
	ents := make([]tg.MessageEntityClass, 0)

	utf16Len := func(s string) int {
		rs := []rune(s)
		return len(utf16.Encode(rs))
	}
	curOffset := func() int { return utf16Len(out.String()) }

	// state maps
	startMap := map[ast.Node]int{}
	linkURL := map[ast.Node]string{}
	codeLang := map[ast.Node]string{}
	listCounter := map[ast.Node]int{}

	// helper: determine if node has a CodeSpan or FencedCodeBlock ancestor
	isInCode := func(n ast.Node) bool {
		for p := n.Parent(); p != nil; p = p.Parent() {
			switch p.(type) {
			case *ast.CodeSpan, *ast.FencedCodeBlock:
				return true
			}
		}
		return false
	}

	// Walk AST
	err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		switch node := n.(type) {

		// Block-level nodes
		case *ast.Paragraph:
			if !entering {
				out.WriteString("\n\n")
			}

		case *ast.Heading:
			if entering {
				startMap[n] = curOffset()
			} else {
				start := startMap[n]
				length := curOffset() - start
				if length > 0 {
					ents = append(ents, &tg.MessageEntityBold{Offset: start, Length: length})
				}
				out.WriteString("\n\n")
			}

		case *ast.Blockquote:
			if entering {
				out.WriteString("> ")
			} else {
				out.WriteString("\n\n")
			}

		case *ast.List:
			if entering {
				listCounter[n] = 0
			} else {
				delete(listCounter, n)
				out.WriteString("\n")
			}

		case *ast.ListItem:
			if entering {
				if parent := n.Parent(); parent != nil {
					if _, ok := parent.(*ast.List); ok {
						listCounter[parent]++
						if ln, ok := parent.(*ast.List); ok && ln.IsOrdered() {
							idx := listCounter[parent]
							out.WriteString(fmt.Sprintf("%d. ", idx))
						} else {
							out.WriteString("• ")
						}
					} else {
						out.WriteString("• ")
					}
				} else {
					out.WriteString("• ")
				}
			} else {
				out.WriteString("\n")
			}

		case *ast.FencedCodeBlock:
			if entering {
				startMap[n] = curOffset()
				codeLang[n] = string(node.Language(src))
			} else {
				lines := node.Lines()
				for i := 0; i < lines.Len(); i++ {
					seg := lines.At(i)
					line := string(seg.Value(src))
					line = strings.TrimRight(line, "\n")
					if preferBlockquote {
						out.WriteString("> ")
						out.WriteString(line)
					} else {
						out.WriteString(line)
					}
					if i < lines.Len()-1 {
						out.WriteString("\n")
					}
				}
				start := startMap[n]
				length := curOffset() - start
				if length > 0 {
					if preferBlockquote {
						ents = append(ents, &tg.MessageEntityBlockquote{Offset: start, Length: length})
					} else {
						ents = append(ents, &tg.MessageEntityPre{Offset: start, Length: length, Language: codeLang[n]})
					}
				}
				out.WriteString("\n\n")
			}

		case *ast.HTMLBlock:
			// ignore HTML blocks

		// Inline nodes
		case *ast.Text:
			if !entering {
				return ast.WalkContinue, nil
			}
			// skip text that belongs to code constructs (they're emitted by code handlers)
			if isInCode(n) {
				return ast.WalkContinue, nil
			}
			out.Write(node.Segment.Value(src))

		case *ast.SoftLineBreak:
			out.WriteString("\n")
		case *ast.HardLineBreak:
			out.WriteString("\n")

		case *ast.CodeSpan:
			if entering {
				startMap[n] = curOffset()
			} else {
				codeText := string(node.Text(src))
				out.WriteString(codeText)
				start := startMap[n]
				length := curOffset() - start
				if length > 0 {
					ents = append(ents, &tg.MessageEntityCode{Offset: start, Length: length})
				}
			}

		case *ast.Emphasis:
			if entering {
				startMap[n] = curOffset()
			} else {
				start := startMap[n]
				length := curOffset() - start
				if length > 0 {
					if node.Level == 2 {
						ents = append(ents, &tg.MessageEntityBold{Offset: start, Length: length})
					} else {
						ents = append(ents, &tg.MessageEntityItalic{Offset: start, Length: length})
					}
				}
			}

		case *ast.Link:
			if entering {
				startMap[n] = curOffset()
				linkURL[n] = string(node.Destination)
			} else {
				start := startMap[n]
				length := curOffset() - start
				if length > 0 {
					ents = append(ents, &tg.MessageEntityTextURL{Offset: start, Length: length, URL: linkURL[n]})
				}
			}

		case *ast.Image:
			alt := string(node.Text(src))
			if alt == "" {
				alt = "[image]"
			}
			out.WriteString(alt)

		default:
			// unhandled nodes
		}

		return ast.WalkContinue, nil
	})
	if err != nil {
		return "", nil, err
	}

	plain := out.String()

	// validate entities: ensure they fit inside UTF-16 length
	plainUTF16 := utf16Len(plain)
	valid := make([]tg.MessageEntityClass, 0, len(ents))
	for _, e := range ents {
		var off, ln int
		switch v := e.(type) {
		case *tg.MessageEntityBold:
			off, ln = v.Offset, v.Length
		case *tg.MessageEntityItalic:
			off, ln = v.Offset, v.Length
		case *tg.MessageEntityCode:
			off, ln = v.Offset, v.Length
		case *tg.MessageEntityPre:
			off, ln = v.Offset, v.Length
		case *tg.MessageEntityTextURL:
			off, ln = v.Offset, v.Length
		case *tg.MessageEntityBlockquote:
			off, ln = v.Offset, v.Length
		default:
			// include unknown entity types as-is
			valid = append(valid, e)
			continue
		}
		if off < 0 || ln <= 0 || off+ln > plainUTF16 {
			continue
		}
		valid = append(valid, e)
	}

	return plain, valid, nil
}

// utf16Len helper
func utf16Len(s string) int {
	rs := []rune(s)
	return len(utf16.Encode(rs))
}
