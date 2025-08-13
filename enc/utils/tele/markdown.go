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

// Entity is a neutral representation of a Telegram entity.
// Offset and Length are measured in UTF-16 code units (Telegram expectation),
// and use int to match varying library signatures.
type Entity struct {
	Type     string // "bold","italic","code","pre","text_link","blockquote", etc.
	Offset   int    // UTF-16 code units
	Length   int    // UTF-16 code units
	URL      string // for text_link
	Language string // for pre blocks
}

// MarkdownToEntities parses markdown and returns plain text + generic Entities.
// preferBlockquote: if true fenced code blocks are emitted as quoted lines prefixed
// with "> " and annotated with a "blockquote" entity; otherwise "pre" is emitted.
func MarkdownToEntities(md string, preferBlockquote bool) (string, []Entity, error) {
	fmt.Println(md)
	src := []byte(md)
	parser := goldmark.DefaultParser()
	doc := parser.Parse(text.NewReader(src))

	var out bytes.Buffer
	ents := make([]Entity, 0)

	// helpers for UTF-16 length (Telegram expects UTF-16 code units)
	utf16Len := func(s string) int {
		rs := []rune(s)
		return len(utf16.Encode(rs))
	}
	curOffset := func() int { return utf16Len(out.String()) }

	// state maps while walking
	startMap := map[ast.Node]int{}
	linkURL := map[ast.Node]string{}
	codeLang := map[ast.Node]string{}
	listCounter := map[ast.Node]int{}

	// helper: detect if node is inside a CodeSpan or FencedCodeBlock ancestor
	isInCode := func(n ast.Node) bool {
		for p := n.Parent(); p != nil; p = p.Parent() {
			switch p.(type) {
			case *ast.CodeSpan, *ast.FencedCodeBlock:
				return true
			}
		}
		return false
	}

	// Walk AST with type assertions to avoid depending on Kind constants
	err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		// block node type switch
		switch node := n.(type) {
		// DOCUMENT: nothing specific

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
					ents = append(ents, Entity{Type: "bold", Offset: start, Length: length})
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
						// increment counter for that list
						listCounter[parent]++
						if ln, ok := parent.(*ast.List); ok && ln.IsOrdered() {
							out.WriteString(fmt.Sprintf("%d. ", listCounter[parent]))
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
				// write lines
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
						ents = append(ents, Entity{Type: "blockquote", Offset: start, Length: length})
					} else {
						ents = append(ents, Entity{Type: "pre", Offset: start, Length: length, Language: codeLang[n]})
					}
				}
				out.WriteString("\n\n")
			}

		case *ast.HTMLBlock:
			// ignore

		// Inline nodes
		case *ast.Text:
			// Skip writing text if it's inside an inline code or a fenced code block; those are handled separately
			if isInCode(n) {
				// do not write here — handled by CodeSpan / FencedCodeBlock logic
				return ast.WalkContinue, nil
			}
			out.Write(node.Segment.Value(src))
			if node.HardLineBreak() {
				out.WriteString("\n")
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
					ents = append(ents, Entity{Type: "code", Offset: start, Length: length})
				}
			}

		case *ast.Emphasis:
			// goldmark uses Emphasis with Level (1 = italic, 2 = bold)
			if entering {
				startMap[n] = curOffset()
			} else {
				start := startMap[n]
				length := curOffset() - start
				if length > 0 {
					if node.Level == 2 {
						ents = append(ents, Entity{Type: "bold", Offset: start, Length: length})
					} else {
						ents = append(ents, Entity{Type: "italic", Offset: start, Length: length})
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
					ents = append(ents, Entity{Type: "text_link", Offset: start, Length: length, URL: linkURL[n]})
				}
			}

		case *ast.Image:
			alt := string(node.Text(src))
			if alt == "" {
				alt = "[image]"
			}
			out.WriteString(alt)

		// ignore inline HTML

		default:
			// other node kinds currently not specially handled
		}

		return ast.WalkContinue, nil
	})
	if err != nil {
		return "", nil, err
	}

	plain := out.String()
	fmt.Println(plain)

	// validate entity bounds
	plainUTF16 := utf16Len(plain)
	valid := make([]Entity, 0, len(ents))
	for _, e := range ents {
		if e.Offset < 0 || e.Length <= 0 || e.Offset+e.Length > plainUTF16 {
			continue
		}
		valid = append(valid, e)
	}
	return plain, valid, nil
}

// ToGotdEntities converts generic Entities to gotd (tg) entities.
// This adapter assumes tg.MessageEntity structs accept int Offset/Length fields.
// If your gotd version expects different types, tweak the literals accordingly.
func ToGotdEntities(generic []Entity) []tg.MessageEntityClass {
	out := make([]tg.MessageEntityClass, 0, len(generic))
	for _, e := range generic {
		switch e.Type {
		case "bold":
			out = append(out, &tg.MessageEntityBold{Offset: e.Offset, Length: e.Length})
		case "italic":
			out = append(out, &tg.MessageEntityItalic{Offset: e.Offset, Length: e.Length})
		case "code":
			out = append(out, &tg.MessageEntityCode{Offset: e.Offset, Length: e.Length})
		case "pre":
			out = append(out, &tg.MessageEntityPre{Offset: e.Offset, Length: e.Length, Language: e.Language})
		case "blockquote":
			// MessageEntityBlockquote may or may not exist in your gotd version; if it does, this compiles.
			out = append(out, &tg.MessageEntityBlockquote{Offset: e.Offset, Length: e.Length})
		case "text_link":
			out = append(out, &tg.MessageEntityTextURL{Offset: e.Offset, Length: e.Length, URL: e.URL})
		default:
			// unknown: ignore
		}
	}
	return out
}

// utf16Len helper
func utf16Len(s string) int {
	rs := []rune(s)
	return len(utf16.Encode(rs))
}

func init() {
	text, _, _ := MarkdownToEntities("Some text")
	fmt.Println(text)
}