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

// Entity is a generic representation of a Telegram message entity.
// Offsets and Length are in UTF-16 code units (Telegram requirement).
type Entity struct {
	// Type can be "bold","italic","code","pre","text_link","blockquote", etc.
	Type     string
	Offset   int // UTF-16 code units
	Length   int // UTF-16 code units
	URL      string
	Language string // for pre/code blocks
}

// MarkdownToEntitiesGeneric parses the given Markdown and returns a plain-text
// rendering plus a slice of generic Entities (offsets/lengths are UTF-16).
//
// preferBlockquote: if true, fenced code blocks are rendered as >-prefixed quoted
// lines and an entity of type "blockquote" is emitted covering that region.
// otherwise code blocks are emitted as "pre" with Language filled.
func MarkdownToEntitiesGeneric(md string, preferBlockquote bool) (string, []Entity, error) {
	src := []byte(md)
	parser := goldmark.DefaultParser()
	doc := parser.Parse(text.NewReader(src))

	var out bytes.Buffer
	ents := make([]Entity, 0)

	// helper: compute UTF-16 length (Telegram expects UTF-16 code units)
	utf16Len := func(s string) int {
		rs := []rune(s)
		return len(utf16.Encode(rs))
	}
	curOffset := func() int { return utf16Len(out.String()) }

	// maps to store node state while walking
	startMap := map[ast.Node]int{}
	linkURL := map[ast.Node]string{}
	codeLang := map[ast.Node]string{}

	// maintain list counters for ordered lists: map[listNode]counter
	listCounter := map[ast.Node]int{}

	// Walk AST
	err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		switch n.Kind() {
		// Document / basic blocks
		case ast.KindDocument:
			// nothing

		case ast.KindParagraph:
			if !entering {
				out.WriteString("\n\n")
			}

		case ast.KindHeading:
			if entering {
				startMap[n] = curOffset()
			} else {
				start := startMap[n]
				length := curOffset() - start
				if length > 0 {
					ents = append(ents, Entity{Type: "bold", Offset: start, Length: length})
				}
				out.WriteString("\n\n")
			}

		case ast.KindBlockquote:
			if entering {
				// prefix quote marker
				out.WriteString("> ")
			} else {
				out.WriteString("\n\n")
			}

		case ast.KindList:
			if entering {
				listCounter[n] = 0
			} else {
				delete(listCounter, n)
				out.WriteString("\n")
			}

		case ast.KindListItem:
			if entering {
				// find the parent list to check ordered vs unordered
				if listNode := n.Parent(); listNode != nil && listNode.Kind() == ast.KindList {
					counter := listCounter[listNode] + 1
					listCounter[listNode] = counter
					// check if parent list is ordered by casting (goldmark uses *ast.List)
					if ln, ok := listNode.(*ast.List); ok && ln.IsOrdered() {
						out.WriteString(fmt.Sprintf("%d. ", counter))
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
				startMap[n] = curOffset()
				if cb, ok := n.(*ast.FencedCodeBlock); ok {
					codeLang[n] = string(cb.Language(src))
				}
			} else {
				// write block lines
				if cb, ok := n.(*ast.FencedCodeBlock); ok {
					lines := cb.Lines()
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
				}
				start := startMap[n]
				length := curOffset() - start
				if length > 0 {
					if preferBlockquote {
						ents = append(ents, Entity{Type: "blockquote", Offset: start, Length: length})
					} else {
						ents = append(ents, Entity{Type: "pre", Offset: start, Length: length, Language: codeLang[n]})
					}
				}
				out.WriteString("\n\n")
			}

		case ast.KindHTMLBlock:
			// ignore

		// Inline nodes
		case ast.KindText:
			if t, ok := n.(*ast.Text); ok {
				out.Write(t.Segment.Value(src))
				if t.HardLineBreak() {
					out.WriteString("\n")
				}
			}

		case ast.KindCodeSpan:
			if entering {
				startMap[n] = curOffset()
			} else {
				if cs, ok := n.(*ast.CodeSpan); ok {
					codeText := string(cs.Text(src))
					out.WriteString(codeText)
					start := startMap[n]
					length := curOffset() - start
					if length > 0 {
						ents = append(ents, Entity{Type: "code", Offset: start, Length: length})
					}
				}
			}

		case ast.KindEmphasis:
			if entering {
				startMap[n] = curOffset()
			} else {
				start := startMap[n]
				length := curOffset() - start
				if length > 0 {
					ents = append(ents, Entity{Type: "italic", Offset: start, Length: length})
				}
			}

		case ast.KindStrong:
			if entering {
				startMap[n] = curOffset()
			} else {
				start := startMap[n]
				length := curOffset() - start
				if length > 0 {
					ents = append(ents, Entity{Type: "bold", Offset: start, Length: length})
				}
			}

		case ast.KindLink:
			if entering {
				startMap[n] = curOffset()
				if l, ok := n.(*ast.Link); ok {
					linkURL[n] = string(l.Destination)
				}
			} else {
				start := startMap[n]
				length := curOffset() - start
				if length > 0 {
					ents = append(ents, Entity{Type: "text_link", Offset: start, Length: length, URL: linkURL[n]})
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

		case ast.KindSoftbreak:
			out.WriteString("\n")
		case ast.KindHardbreak:
			out.WriteString("\n")
		default:
			// Unhandled nodes are walked into — nothing to do here specifically.
		}

		return ast.WalkContinue, nil
	})
	if err != nil {
		return "", nil, err
	}

	plain := out.String()

	// Validate that entities are within bounds
	plainUTF16 := utf16Len(plain)
	valid := make([]Entity, 0, len(ents))
	for _, e := range ents {
		if e.Offset < 0 || e.Length <= 0 || e.Offset+e.Length > plainUTF16 {
			// skip bad entity
			continue
		}
		valid = append(valid, e)
	}
	return plain, valid, nil
}

func ToGotdEntities(es []Entity) []tg.MessageEntityClass {
	var out []tg.MessageEntityClass
	for _, e := range es {
		switch e.Type {
		case "bold":
			out = append(out, &tg.MessageEntityBold{Offset: int32(e.Offset), Length: int32(e.Length)})
		case "italic":
			out = append(out, &tg.MessageEntityItalic{Offset: int32(e.Offset), Length: int32(e.Length)})
		case "code":
			out = append(out, &tg.MessageEntityCode{Offset: int32(e.Offset), Length: int32(e.Length)})
		case "pre":
			out = append(out, &tg.MessageEntityPre{Offset: int32(e.Offset), Length: int32(e.Length), Language: e.Language})
		case "blockquote":
			// some versions of gotd support MessageEntityBlockquote (check your version)
			out = append(out, &tg.MessageEntityBlockquote{Offset: int32(e.Offset), Length: int32(e.Length)})
		case "text_link":
			out = append(out, &tg.MessageEntityTextUrl{Offset: int32(e.Offset), Length: int32(e.Length), Url: e.URL})
		default:
			// ignore unknown types or log
		}
	}
	return out
}
