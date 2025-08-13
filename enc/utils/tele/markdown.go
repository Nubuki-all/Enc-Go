package tele

import (
	"bytes"
	"fmt"
	"sort"
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

	curOffset := func() int { return utf16Len(out.String()) }

	// State tracking
	startMap := map[ast.Node]int{}
	linkURL := map[ast.Node]string{}
	codeLang := map[ast.Node]string{}
	listCounter := map[ast.Node]int{}
	blockquoteLevel := 0
	blockquotePrefix := ""

	// Update blockquote prefix based on current level
	updateBlockquotePrefix := func() {
		if blockquoteLevel > 0 {
			blockquotePrefix = strings.Repeat("> ", blockquoteLevel)
		} else {
			blockquotePrefix = ""
		}
	}

	// Helper: determine if node has a CodeSpan or FencedCodeBlock ancestor
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
				out.WriteString("\n")
				if blockquoteLevel > 0 {
					out.WriteString(blockquotePrefix)
				}
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
				if blockquoteLevel > 0 {
					out.WriteString(blockquotePrefix)
				}
			}

		case *ast.Blockquote:
			if entering {
				blockquoteLevel++
				updateBlockquotePrefix()
				// Write prefix at the start of the blockquote
				out.WriteString(blockquotePrefix)
			} else {
				blockquoteLevel--
				updateBlockquotePrefix()
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
				if blockquoteLevel > 0 {
					out.WriteString(blockquotePrefix)
				}
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
					
					if blockquoteLevel > 0 {
						out.WriteString(blockquotePrefix)
					}
					if preferBlockquote {
						out.WriteString("> ")
					}
					out.WriteString(line)
					
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
				if blockquoteLevel > 0 {
					out.WriteString(blockquotePrefix)
				}
			}

		// Inline nodes
		case *ast.Text:
			if !entering || isInCode(n) {
				return ast.WalkContinue, nil
			}
			seg := node.Segment
			value := seg.Value(src)
			out.Write(value)

		case *ast.Softbreak:
			out.WriteString(" ")

		case *ast.Hardbreak:
			out.WriteString("\n")
			if blockquoteLevel > 0 {
				out.WriteString(blockquotePrefix)
			}

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
				// Write URL if no text content
				if curOffset() == startMap[n] {
					out.WriteString(linkURL[n])
				}
				start := startMap[n]
				length := curOffset() - start
				if length > 0 {
					ents = append(ents, &tg.MessageEntityTextURL{
						Offset: start,
						Length: length,
						URL:    linkURL[n],
					})
				}
			}

		case *ast.AutoLink:
			if entering {
				startMap[n] = curOffset()
				url := string(node.URL(src))
				linkURL[n] = url
				out.WriteString(url)
			} else {
				start := startMap[n]
				length := curOffset() - start
				if length > 0 {
					ents = append(ents, &tg.MessageEntityTextURL{
						Offset: start,
						Length: length,
						URL:    linkURL[n],
					})
				}
			}

		case *ast.Strikethrough:
			if entering {
				startMap[n] = curOffset()
			} else {
				start := startMap[n]
				length := curOffset() - start
				if length > 0 {
					ents = append(ents, &tg.MessageEntityStrike{Offset: start, Length: length})
				}
			}

		case *ast.Image:
			if entering {
				alt := string(node.Text(src))
				if alt == "" {
					alt = "[image]"
				}
				out.WriteString(alt)
			}

		default:
			// unhandled nodes
		}

		return ast.WalkContinue, nil
	})
	if err != nil {
		return "", nil, err
	}

	plain := out.String()

	// Validate and filter entities
	plainUTF16 := utf16Len(plain)
	valid := make([]tg.MessageEntityClass, 0, len(ents))
	for _, e := range ents {
		offset, length := getEntityBounds(e)
		if offset < 0 || length <= 0 || offset+length > plainUTF16 {
			continue
		}
		valid = append(valid, e)
	}

	// Sort entities by offset and length (Telegram requirement)
	sort.Slice(valid, func(i, j int) bool {
		ioff, ilen := getEntityBounds(valid[i])
		joff, jlen := getEntityBounds(valid[j])
		if ioff != joff {
			return ioff < joff
		}
		return ilen < jlen
	})

	return plain, valid, nil
}

// Helper to get entity bounds
func getEntityBounds(e tg.MessageEntityClass) (offset, length int) {
	switch v := e.(type) {
	case *tg.MessageEntityBold:
		return v.Offset, v.Length
	case *tg.MessageEntityItalic:
		return v.Offset, v.Length
	case *tg.MessageEntityCode:
		return v.Offset, v.Length
	case *tg.MessageEntityPre:
		return v.Offset, v.Length
	case *tg.MessageEntityTextURL:
		return v.Offset, v.Length
	case *tg.MessageEntityBlockquote:
		return v.Offset, v.Length
	case *tg.MessageEntityStrike:
		return v.Offset, v.Length
	default:
		return 0, 0
	}
}

// Calculate UTF-16 length of a string
func utf16Len(s string) int {
	rs := []rune(s)
	return len(utf16.Encode(rs))
}