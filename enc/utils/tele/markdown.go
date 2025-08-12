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

// MarkdownToTelegram parses Markdown and returns plain text and a slice of
// tg.MessageEntity. Offsets/Lengths are in UTF-16 code units (Telegram format).
//
// preferBlockquote: if true, fenced code blocks are emitted as `> `-prefixed
// quoted lines and a MessageEntityBlockquote is attached. If false, code
// blocks become MessageEntityPre (monospaced).
func MarkdownToTelegram(md string, preferBlockquote bool) (string, []tg.MessageEntity, error) {
	src := []byte(md)
	parser := goldmark.DefaultParser()
	doc := parser.Parse(text.NewReader(src))

	var out bytes.Buffer
	entities := make([]tg.MessageEntity, 0)

	// helpers to compute UTF-16 lengths (Telegram expects UTF-16 code units)
	utf16Len := func(s string) int {
		rs := []rune(s)
		return len(utf16.Encode(rs))
	}
	currentOffset := func() int {
		return utf16Len(out.String())
	}

	// maps to store node start offsets and auxiliary data
	startMap := map[ast.Node]int{}
	linkURL := map[ast.Node]string{}
	codeLang := map[ast.Node]string{}

	// Walk AST
	err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		switch n.Kind() {
		// --- Block nodes ---
		case ast.KindDocument:
			// nothing
		case ast.KindParagraph:
			if !entering {
				// paragraphs separated by blank line
				out.WriteString("\n\n")
			}
		case ast.KindHeading:
			if entering {
				startMap[n] = currentOffset()
			} else {
				start := startMap[n]
				length := currentOffset() - start
				if length > 0 {
					entities = append(entities, &tg.MessageEntityBold{Offset: int32(start), Length: int32(length)})
				}
				out.WriteString("\n\n")
			}
		case ast.KindBlockquote:
			if entering {
				// prefix the block with "> " — for multi-paragraph quotes we prefix only once here.
				out.WriteString("> ")
			} else {
				out.WriteString("\n\n")
			}
		case ast.KindList:
			// nothing here - items handle bullets/numbering
		case ast.KindItem:
			if entering {
				// detect ordered list parent
				if listNode, ok := n.Parent().(*ast.List); ok {
					if listNode.IsOrdered() {
						// compute index by counting previous siblings that are list items
						idx := 1
						for sib := n.PrevSibling(); sib != nil; sib = sib.PrevSibling() {
							if _, ok := sib.(*ast.ListItem); ok {
								idx++
							}
						}
						out.WriteString(fmt.Sprintf("%d. ", idx))
					} else {
						out.WriteString("• ")
					}
				} else {
					out.WriteString("• ")
				}
			} else {
				out.WriteString("\n")
			}
		case ast.KindFencedCodeBlock:
			if entering {
				startMap[n] = currentOffset()
				if cb, ok := n.(*ast.FencedCodeBlock); ok {
					codeLang[n] = string(cb.Language(src))
				}
			} else {
				// collect lines
				if cb, ok := n.(*ast.FencedCodeBlock); ok {
					lines := cb.Lines()
					for i := 0; i < lines.Len(); i++ {
						seg := lines.At(i)
						line := string(seg.Value(src))
						// strip trailing newline(s)
						line = strings.TrimRight(line, "\n")
						if preferBlockquote {
							out.WriteString("> ")
							out.WriteString(line)
						} else {
							out.WriteString(line)
						}
						// add newline between lines
						if i < lines.Len()-1 {
							out.WriteString("\n")
						}
					}
				}
				start := startMap[n]
				length := currentOffset() - start
				if length > 0 {
					if preferBlockquote {
						entities = append(entities, &tg.MessageEntityBlockquote{Offset: int32(start), Length: int32(length)})
					} else {
						lang := codeLang[n]
						entities = append(entities, &tg.MessageEntityPre{Offset: int32(start), Length: int32(length), Language: lang})
					}
				}
				out.WriteString("\n\n")
			}
		case ast.KindHTMLBlock:
			// ignore or strip HTML blocks
		// --- Inline nodes ---
		case ast.KindText:
			if t, ok := n.(*ast.Text); ok {
				seg := t.Segment
				out.Write(seg.Value(src))
				if t.HardLineBreak() {
					out.WriteString("\n")
				}
			}
		case ast.KindCodeSpan:
			if entering {
				startMap[n] = currentOffset()
			} else {
				if cs, ok := n.(*ast.CodeSpan); ok {
					codeText := string(cs.Text(src))
					out.WriteString(codeText)
					start := startMap[n]
					length := currentOffset() - start
					if length > 0 {
						entities = append(entities, &tg.MessageEntityCode{Offset: int32(start), Length: int32(length)})
					}
				}
			}
		case ast.KindEmphasis:
			if entering {
				startMap[n] = currentOffset()
			} else {
				start := startMap[n]
				length := currentOffset() - start
				if length > 0 {
					entities = append(entities, &tg.MessageEntityItalic{Offset: int32(start), Length: int32(length)})
				}
			}
		case ast.KindStrong:
			if entering {
				startMap[n] = currentOffset()
			} else {
				start := startMap[n]
				length := currentOffset() - start
				if length > 0 {
					entities = append(entities, &tg.MessageEntityBold{Offset: int32(start), Length: int32(length)})
				}
			}
		case ast.KindLink:
			if entering {
				startMap[n] = currentOffset()
				if l, ok := n.(*ast.Link); ok {
					linkURL[n] = string(l.Destination)
				}
			} else {
				start := startMap[n]
				length := currentOffset() - start
				if length > 0 {
					u := linkURL[n]
					entities = append(entities, &tg.MessageEntityTextUrl{Offset: int32(start), Length: int32(length), Url: u})
				}
			}
		case ast.KindImage:
			if img, ok := n.(*ast.Image); ok {
				alt := string(img.Text(src))
				if alt == "" {
					alt = "[image]"
				}
				out.WriteString(alt)
			}
		case ast.KindHTMLInline:
			// ignore inline HTML
		case ast.KindSoftbreak:
			out.WriteString("\n")
		case ast.KindHardbreak:
			out.WriteString("\n")
		}

		return ast.WalkContinue, nil
	})
	if err != nil {
		return "", nil, err
	}

	plain := out.String()

	// Validate entities bounds (UTF-16 units)
	plainUTF16Len := utf16Len(plain)
	valid := make([]tg.MessageEntity, 0, len(entities))
	for _, e := range entities {
		var off, ln int32
		switch ent := e.(type) {
		case *tg.MessageEntityBold:
			off, ln = ent.Offset, ent.Length
		case *tg.MessageEntityItalic:
			off, ln = ent.Offset, ent.Length
		case *tg.MessageEntityCode:
			off, ln = ent.Offset, ent.Length
		case *tg.MessageEntityPre:
			off, ln = ent.Offset, ent.Length
		case *tg.MessageEntityTextUrl:
			off, ln = ent.Offset, ent.Length
		case *tg.MessageEntityBlockquote:
			off, ln = ent.Offset, ent.Length
		default:
			// For other entity types, attempt to append them as-is
			valid = append(valid, e)
			continue
		}
		if off < 0 || ln <= 0 || int(off+ln) > plainUTF16Len {
			// skip invalid entity
			continue
		}
		valid = append(valid, e)
	}

	return plain, valid, nil
}
