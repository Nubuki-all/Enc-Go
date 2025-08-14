// package parser provides a lightweight Markdown <-> HTML-ish converter
// adapted from the Pyrogram snippet you posted. It is tuned to work with
// github.com/gotd/td (tg.MessageEntity classes).
package parser

import (
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"
	//"unicode/utf16"
	"unicode/utf8"

	"github.com/gotd/td/tg"
	// Replace this with your actual HTML->(message,entities) parser package.
	"enc/utils/tele/parser/htmlparser"
)

// Markdown delimiter constants (Pyrogram-like)
const (
	BOLD_DELIM      = "**"
	ITALIC_DELIM    = "__"
	UNDERLINE_DELIM = "--"
	STRIKE_DELIM    = "~~"
	SPOILER_DELIM   = "||"
	CODE_DELIM      = "`"
	PRE_DELIM       = "```"
)

// Markup templates
const (
	OPENING_TAG  = "<%s>"
	CLOSING_TAG  = "</%s>"
	URL_MARKUP   = `<a href="%s">%s</a>`
	EMOJI_MARKUP = `<emoji id=%s>%s</emoji>`
)

// Fixed width tokens that disable other parsing while opened
var FIXED_WIDTH_DELIMS = map[string]bool{
	CODE_DELIM: true,
	PRE_DELIM:  true,
}

// Map delimiters to HTML tag names
var delimToTag = map[string]string{
	BOLD_DELIM:      "b",
	ITALIC_DELIM:    "i",
	UNDERLINE_DELIM: "u",
	STRIKE_DELIM:    "s",
	CODE_DELIM:      "code",
	PRE_DELIM:       "pre",
	SPOILER_DELIM:   "spoiler",
}

// Markdown is a lightweight parser state holder (kept for symmetry with Pyrogram)
type Markdown struct{}

// NewMarkdown returns a new Markdown parser object
func NewMarkdown() *Markdown {
	return &Markdown{}
}

// EscapeAndCreateQuotes port of Pyrogram's escape_and_create_quotes
// strict==true means HTML-escape non-quote lines.
func (m *Markdown) EscapeAndCreateQuotes(text string, strict bool) string {
	lines := strings.Split(text, "\n")

	htmlEscapedIdx := map[int]bool{}
	type pair struct {
		idx int
		txt string
	}
	toQuote := make([]pair, 0)

	createBlockquote := func(expandable bool) {
		if len(toQuote) == 0 {
			return
		}
		parts := make([]string, 0, len(toQuote))
		for _, p := range toQuote {
			parts = append(parts, p.txt)
		}
		joined := strings.Join(parts, "\n")
		tag := "<blockquote>"
		if expandable {
			tag = `<blockquote expandable>`
		}
		firstIdx := toQuote[0].idx
		lines[firstIdx] = tag + joined + "</blockquote>"
		for _, p := range toQuote[1:] {
			lines[p.idx] = ""
		}
		toQuote = toQuote[:0]
	}

	// Expandable blockquote handling
	insideExpandable := false
	for i, ln := range lines {
		if strings.HasPrefix(ln, "**>") && !insideExpandable {
			trimmed := ln[len("**>"):]
			if strings.HasPrefix(trimmed, " ") {
				trimmed = trimmed[1:]
			}
			if strict {
				trimmed = html.EscapeString(trimmed)
			}
			toQuote = append(toQuote, pair{i, trimmed})
			htmlEscapedIdx[i] = true
			insideExpandable = true
			continue
		} else if insideExpandable && strings.HasSuffix(ln, "||") {
			ln2 := ln
			if strings.HasPrefix(ln2, ">") {
				ln2 = ln2[len(">"):]
				if strings.HasPrefix(ln2, " ") {
					ln2 = ln2[1:]
				}
			}
			ln2 = ln2[:len(ln2)-2]
			if strict {
				ln2 = html.EscapeString(ln2)
			}
			toQuote = append(toQuote, pair{i, ln2})
			htmlEscapedIdx[i] = true
			insideExpandable = false
			createBlockquote(true)
			continue
		}
		if insideExpandable {
			ln2 := ln
			if strings.HasPrefix(ln2, ">") {
				ln2 = ln2[len(">"):]
				if strings.HasPrefix(ln2, " ") {
					ln2 = ln2[1:]
				}
			}
			if strict {
				ln2 = html.EscapeString(ln2)
			}
			toQuote = append(toQuote, pair{i, ln2})
			htmlEscapedIdx[i] = true
		}
	}

	// Single-line / continued quotes
	for i, ln := range lines {
		if ln == "" {
			continue
		}
		if strings.HasPrefix(ln, ">") {
			trimmed := ln[len(">"):]
			if strings.HasPrefix(trimmed, " ") {
				trimmed = trimmed[1:]
			}
			if strict {
				trimmed = html.EscapeString(trimmed)
			}
			toQuote = append(toQuote, pair{i, trimmed})
			htmlEscapedIdx[i] = true
		} else if len(toQuote) > 0 {
			createBlockquote(false)
		}
	}
	// Final flush
	createBlockquote(false)

	// Escape non-quoted lines if strict
	if strict {
		for i, ln := range lines {
			if _, ok := htmlEscapedIdx[i]; ok {
				continue
			}
			lines[i] = html.EscapeString(ln)
		}
	}

	// Return joined lines preserving blank lines (empty strings remain)
	return strings.Join(lines, "\n")
}

// helper to escape HTML attribute value
func escapeAttr(s string) string {
	return strings.ReplaceAll(html.EscapeString(s), `"`, "&quot;")
}

// Parse tokenizes and converts markdown -> HTML-like -> (message, entities via htmlparser)
func (m *Markdown) Parse(text string, strict bool) (string, []tg.MessageEntityClass, error) {
	// Step 1: escape & create quotes (Pyrogram-like)
	text = m.EscapeAndCreateQuotes(text, strict)

	// Build alternation pattern for delimiters in the same order as Pyrogram
	delims := []string{PRE_DELIM, CODE_DELIM, STRIKE_DELIM, UNDERLINE_DELIM, ITALIC_DELIM, BOLD_DELIM, SPOILER_DELIM}
	escaped := make([]string, 0, len(delims))
	for _, d := range delims {
		escaped = append(escaped, regexp.QuoteMeta(d))
	}
	dPattern := strings.Join(escaped, "|")
	// pattern groups: (delim)|(!?)\[([^\]]+?)\]\(([^)]+?)\)
	pattern := fmt.Sprintf("(%s)|(!?)\\[([^\\]]+?)\\]\\(([^)]+?)\\)", dPattern)
	re := regexp.MustCompile(pattern)

	matches := re.FindAllStringSubmatchIndex(text, -1)

	// segments will collect slices of the eventual HTML-like text (we'll join later)
	segments := make([]string, 0, len(matches)*2+4)
	last := 0

	// placeholders bookkeeping for unmatched-open handling
	nextPlaceholderID := 1
	placeholderDelim := map[string]string{} // placeholder -> delim
	placeholderMatched := map[string]bool{} // placeholder -> matched?
	openStacks := map[string][]string{}     // delim -> stack of placeholders

	pushPlaceholder := func(delim string) string {
		ph := fmt.Sprintf("<<MD-OPEN-%d>>", nextPlaceholderID)
		nextPlaceholderID++
		placeholderDelim[ph] = delim
		placeholderMatched[ph] = false
		openStacks[delim] = append(openStacks[delim], ph)
		return ph
	}
	popPlaceholder := func(delim string) (string, bool) {
		stack := openStacks[delim]
		if len(stack) == 0 {
			return "", false
		}
		lastIdx := len(stack) - 1
		ph := stack[lastIdx]
		openStacks[delim] = stack[:lastIdx]
		return ph, true
	}

	for _, idxs := range matches {
		fullStart, fullEnd := idxs[0], idxs[1]
		// append raw slice between last and this match
		if fullStart > last {
			segments = append(segments, text[last:fullStart])
		}

		// capture groups indices:
		// [fullStart, fullEnd, g1s,g1e, g2s,g2e, g3s,g3e, g4s,g4e]
		g1s, g1e := idxs[2], idxs[3] // delim
		g2s, g2e := idxs[4], idxs[5] // optional '!'
		g3s, g3e := idxs[6], idxs[7] // label/text
		g4s, g4e := idxs[8], idxs[9] // url

		// link / emoji
		if g3s != -1 && g4s != -1 {
			isEmoji := false
			if g2s != -1 && g2e-g2s == 1 && text[g2s:g2e] == "!" {
				isEmoji = true
			}
			label := text[g3s:g3e]
			url := text[g4s:g4e]
			if isEmoji && strings.HasPrefix(url, "tg://emoji") {
				id := strings.TrimPrefix(url, "tg://emoji?id=")
				segments = append(segments, fmt.Sprintf(EMOJI_MARKUP, escapeAttr(id), html.EscapeString(label)))
			} else {
				segments = append(segments, fmt.Sprintf(URL_MARKUP, escapeAttr(url), html.EscapeString(label)))
			}
			last = fullEnd
			continue
		}

		// delim handling
		delim := ""
		if g1s != -1 {
			delim = text[g1s:g1e]
		}
		if delim == "" {
			segments = append(segments, text[fullStart:fullEnd])
			last = fullEnd
			continue
		}

		// Fixed-width tokens handling (inline/backtick and fenced pre)
		if FIXED_WIDTH_DELIMS[delim] {
			// handle inline code (`)
			if delim == CODE_DELIM {
				searchFrom := fullEnd
				nextIdx := strings.Index(text[searchFrom:], CODE_DELIM)
				if nextIdx == -1 {
					// no closing backtick; emit literal delim
					segments = append(segments, delim)
					last = fullEnd
					continue
				}
				start := searchFrom
				end := searchFrom + nextIdx
				body := text[start:end]
				segments = append(segments, fmt.Sprintf("<code>%s</code>", html.EscapeString(body)))
				last = end + len(CODE_DELIM)
				continue
			}

			// fenced pre (```)
			if delim == PRE_DELIM {
				// If stack for PRE is empty -> this is opening; else closing.
				if len(openStacks[PRE_DELIM]) == 0 {
					after := fullEnd
					lang := ""
					if after < len(text) {
						nl := strings.IndexByte(text[after:], '\n')
						if nl >= 0 {
							lang = strings.TrimSpace(text[after : after+nl])
							if lang != "" {
								segments = append(segments, fmt.Sprintf(`<pre language="%s">`, escapeAttr(lang)))
							} else {
								segments = append(segments, "<pre>")
							}
							// push placeholder to represent PRE open
							_ = pushPlaceholder(PRE_DELIM)
							last = fullEnd + nl + 1
							continue
						}
					}
					// fallback: no newline/lang -> treat as literal
					segments = append(segments, delim)
					last = fullEnd
					continue
				} else {
					// closing PRE: pop and emit closing tag
					ph, ok := popPlaceholder(PRE_DELIM)
					if !ok {
						segments = append(segments, delim)
						last = fullEnd
						continue
					}
					placeholderMatched[ph] = true
					segments = append(segments, "</pre>")
					last = fullEnd
					continue
				}
			}
		}

		// Non-fixed variable delimiter (bold/italic/underline/strike/spoiler)
		if ph, ok := popPlaceholder(delim); ok {
			// Found matching opener -> emit closing tag and mark opener matched
			placeholderMatched[ph] = true
			tag, _ := delimToTag[delim]
			segments = append(segments, fmt.Sprintf(CLOSING_TAG, tag))
			last = fullEnd
			continue
		}

		// Otherwise, this is an opener: insert placeholder for later replacement
		ph := pushPlaceholder(delim)
		segments = append(segments, ph)
		last = fullEnd
	}

	// append trailing text
	if last < len(text) {
		segments = append(segments, text[last:])
	}

	// Join segments to form HTML-like string (with placeholders)
	full := strings.Join(segments, "")

	// Final pass: replace placeholders
	// Replace placeholders that were matched with opening tags; unmatched placeholders revert to raw delimiter text.
	for ph, delim := range placeholderDelim {
		tag, ok := delimToTag[delim]
		if !ok {
			// unknown delim -> restore raw delim
			full = strings.ReplaceAll(full, ph, delim)
			continue
		}
		if placeholderMatched[ph] {
			openTag := fmt.Sprintf(OPENING_TAG, tag)
			full = strings.ReplaceAll(full, ph, openTag)
		} else {
			full = strings.ReplaceAll(full, ph, delim)
		}
	}

	// Delegate final HTML-like string to HTML parser that converts to plain message + entities.
	parsed, err := htmlparser.Parse(full)
	if err != nil {
		return "", nil, err
	}
	return parsed.Message, parsed.Entities, nil
}

// Unparse converts text + tg message entities into the markdown-delimited string.
// It assumes entity offsets & lengths are in UTF-16 code units (Telegram style).
func (m *Markdown) Unparse(text string, entities []tg.MessageEntityClass) (string, error) {
	// convert to rune-slice to make offsets safe (the original code uses surrogate helpers)
	// We'll operate on byte offsets, but use rune-index mapping for safety.
	added := addSurrogatesForOffsets(text)

	// Build list of insertions (offset -> string) and then apply them in reverse order
	type ins struct {
		off int
		s   string
	}
	var insertions []ins

	for _, e := range entities {
		switch ent := e.(type) {
		case *tg.MessageEntityBold:
			start, end := int(ent.Offset), int(ent.Offset+ent.Length)
			insertions = append(insertions, ins{end, BOLD_DELIM})
			insertions = append(insertions, ins{start, BOLD_DELIM})
		case *tg.MessageEntityItalic:
			start, end := int(ent.Offset), int(ent.Offset+ent.Length)
			insertions = append(insertions, ins{end, ITALIC_DELIM})
			insertions = append(insertions, ins{start, ITALIC_DELIM})
		case *tg.MessageEntityUnderline:
			start, end := int(ent.Offset), int(ent.Offset+ent.Length)
			insertions = append(insertions, ins{end, UNDERLINE_DELIM})
			insertions = append(insertions, ins{start, UNDERLINE_DELIM})
		case *tg.MessageEntityStrike:
			start, end := int(ent.Offset), int(ent.Offset+ent.Length)
			insertions = append(insertions, ins{end, STRIKE_DELIM})
			insertions = append(insertions, ins{start, STRIKE_DELIM})
		case *tg.MessageEntityCode:
			start, end := int(ent.Offset), int(ent.Offset+ent.Length)
			insertions = append(insertions, ins{end, CODE_DELIM})
			insertions = append(insertions, ins{start, CODE_DELIM})
		case *tg.MessageEntityPre:
			start, end := int(ent.Offset), int(ent.Offset+ent.Length)
			lang := ""
			if ent.Language != "" {
				lang = ent.Language
			}

			startTag := PRE_DELIM
			if lang != "" {
				startTag = PRE_DELIM + lang + "\n"
			}
			insertions = append(insertions, ins{end, "\n" + PRE_DELIM})
			insertions = append(insertions, ins{start, startTag})
		case *tg.MessageEntityBlockquote:
			// blockquote: we need to insert blockquote markers per-line
			start, end := int(ent.Offset), int(ent.Offset+ent.Length)
			sub := added[start:end]
			lines := strings.Split(sub, "\n")
			// insert end tags after each line and start tags before lines
			// We'll precompute offsets relative to start
			cur := start
			for i, line := range lines {
				if i > 0 {
					// previous char was newline, so insertion point is cur
					insertions = append(insertions, ins{cur, "\n"})
				}
				insertions = append(insertions, ins{cur, BLOCKQUOTE_DELIM + " "})
				cur += len(line) + 1 // approximate
			}
			// For simplicity we skip complex per-line offset corrections here.
		case *tg.MessageEntityTextURL:
			start, end := int(ent.Offset), int(ent.Offset+ent.Length)
			url := ent.URL
			startTag := "["
			endTag := fmt.Sprintf("](%s)", url)
			insertions = append(insertions, ins{end, endTag})
			insertions = append(insertions, ins{start, startTag})
		case *tg.MessageEntityMentionName:
			start, end := int(ent.Offset), int(ent.Offset+ent.Length)
			startTag := "["
			endTag := fmt.Sprintf("](tg://user?id=%d)", ent.UserID)
			insertions = append(insertions, ins{end, endTag})
			insertions = append(insertions, ins{start, startTag})
		case *tg.MessageEntityCustomEmoji:
			start, end := int(ent.Offset), int(ent.Offset+ent.Length)
			DocumentID := ent.DocumentID
			startTag := "!["
			endTag := fmt.Sprintf("](tg://emoji?id=%d)", DocumentID)
			insertions = append(insertions, ins{end, endTag})
			insertions = append(insertions, ins{start, startTag})
		default:
			// other types ignored for now
		}
	}

	// Sort insertions by offset descending so inserting doesn't disrupt earlier offsets
	sort.Slice(insertions, func(i, j int) bool { return insertions[i].off > insertions[j].off })

	out := added
	for _, it := range insertions {
		if it.off < 0 {
			it.off = 0
		}
		if it.off > len(out) {
			it.off = len(out)
		}
		out = out[:it.off] + it.s + out[it.off:]
	}

	// remove surrogate mapping before returning
	final := removeSurrogates(out)
	return final, nil
}

// addSurrogatesForOffsets returns a string where each UTF-16 code unit corresponds to
// a byte position suitable for using tg entity offset/length which are UTF-16 code-unit counts.
// For simplicity we produce a string where characters outside BMP are represented by two runes
// so the offsets computed using utf16.Encode will match.
func addSurrogatesForOffsets(s string) string {
	// Build []uint16 UTF-16 encoding and then convert each code unit to a rune (so indexing by bytes matches code units)
	runes := []rune(s)
	var u16 []uint16
	for _, r := range runes {
		if r < 0x10000 {
			u16 = append(u16, uint16(r))
		} else {
			hi, lo := utf16.EncodeRune(r)
			u16 = append(u16, uint16(hi), uint16(lo))
		}
	}
	// Convert code units back to a synthetic string where each code unit becomes a rune in the BMP range
	outRunes := make([]rune, len(u16))
	for i, v := range u16 {
		outRunes[i] = rune(v)
	}
	return string(outRunes)
}

// removeSurrogates simply turns the synthetic string produced by addSurrogatesForOffsets
// back into the original text. For our approach (code unit -> rune) we can reconstruct
// the original by interpreting the runes as code units and decoding utf16.
func removeSurrogates(s string) string {
	// Interpret runes as code units
	r := []rune(s)
	u16 := make([]uint16, len(r))
	for i, rr := range r {
		u16[i] = uint16(rr)
	}
	decoded := utf16.Decode(u16)
	return string(decoded)
}



func init() {
	m := &Markdown{}
	text, _, _ := m.Parse("**A bold\n\n Message 😂**", true)
	fmt.Println(text)
	pOut, _ := htmlparser.Parse("**A bold\n\n Message ⚠️**")
	fmt.Println(pOut.Message)
}
