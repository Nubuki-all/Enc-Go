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
	if len(entities) == 0 {
		return text, nil
	}

	// Build a list of insertions: (byteOffset, insertionString)
	type insertion struct {
		pos int
		str string
	}
	inserts := make([]insertion, 0, len(entities)*2)

	// Helper to convert utf16 index -> byte position in UTF-8 string
	utf16IndexToBytePos := func(s string, target int) int {
		if target <= 0 {
			return 0
		}
		cum := 0
		bytePos := 0
		for _, r := range s {
			if cum >= target {
				return bytePos
			}
			units := 1
			if r > 0xFFFF {
				units = 2
			}
			cum += units
			bytePos += utf8.RuneLen(r)
		}
		return bytePos
	}

	// Map gotd entity concrete types to delimiter pairs
	for _, e := range entities {
		switch v := e.(type) {
		case *tg.MessageEntityBold:
			start := int(v.Offset)
			end := int(v.Offset + v.Length)
			spos := utf16IndexToBytePos(text, start)
			epos := utf16IndexToBytePos(text, end)
			inserts = append(inserts, insertion{pos: epos, str: BOLD_DELIM})
			inserts = append(inserts, insertion{pos: spos, str: BOLD_DELIM})
		case *tg.MessageEntityItalic:
			start := int(v.Offset)
			end := int(v.Offset + v.Length)
			spos := utf16IndexToBytePos(text, start)
			epos := utf16IndexToBytePos(text, end)
			inserts = append(inserts, insertion{pos: epos, str: ITALIC_DELIM})
			inserts = append(inserts, insertion{pos: spos, str: ITALIC_DELIM})
		case *tg.MessageEntityUnderline:
			start := int(v.Offset)
			end := int(v.Offset + v.Length)
			spos := utf16IndexToBytePos(text, start)
			epos := utf16IndexToBytePos(text, end)
			inserts = append(inserts, insertion{pos: epos, str: UNDERLINE_DELIM})
			inserts = append(inserts, insertion{pos: spos, str: UNDERLINE_DELIM})
		case *tg.MessageEntityStrike:
			start := int(v.Offset)
			end := int(v.Offset + v.Length)
			spos := utf16IndexToBytePos(text, start)
			epos := utf16IndexToBytePos(text, end)
			inserts = append(inserts, insertion{pos: epos, str: STRIKE_DELIM})
			inserts = append(inserts, insertion{pos: spos, str: STRIKE_DELIM})
		case *tg.MessageEntityCode:
			start := int(v.Offset)
			end := int(v.Offset + v.Length)
			spos := utf16IndexToBytePos(text, start)
			epos := utf16IndexToBytePos(text, end)
			inserts = append(inserts, insertion{pos: epos, str: CODE_DELIM})
			inserts = append(inserts, insertion{pos: spos, str: CODE_DELIM})
		case *tg.MessageEntityPre:
			lang := v.Language

			start := int(v.Offset)
			end := int(v.Offset + v.Length)
			spos := utf16IndexToBytePos(text, start)
			epos := utf16IndexToBytePos(text, end)
			startTag := PRE_DELIM
			if lang != "" {
				startTag = PRE_DELIM + lang + "\n"
			}
			endTag := "\n" + PRE_DELIM
			inserts = append(inserts, insertion{pos: epos, str: endTag})
			inserts = append(inserts, insertion{pos: spos, str: startTag})
		case *tg.MessageEntityBlockquote:
			start := int(v.Offset)
			end := int(v.Offset + v.Length)
			spos := utf16IndexToBytePos(text, start)
			epos := utf16IndexToBytePos(text, end)
			// For blockquote we prefix each line with "> " on insertion
			blockText := text[spos:epos]
			lines := strings.Split(blockText, "\n")
			quoted := ""
			for i, ln := range lines {
				if i > 0 {
					quoted += "\n"
				}
				quoted += "> " + ln
			}
			text = text[:spos] + quoted + text[epos:]
			delta := len(quoted) - (epos - spos)
			for j := range inserts {
				if inserts[j].pos > spos {
					inserts[j].pos += delta
				}
			}
		case *tg.MessageEntitySpoiler:
			start := int(v.Offset)
			end := int(v.Offset + v.Length)
			spos := utf16IndexToBytePos(text, start)
			epos := utf16IndexToBytePos(text, end)
			inserts = append(inserts, insertion{pos: epos, str: SPOILER_DELIM})
			inserts = append(inserts, insertion{pos: spos, str: SPOILER_DELIM})
		case *tg.MessageEntityTextURL:
			start := int(v.Offset)
			end := int(v.Offset + v.Length)
			spos := utf16IndexToBytePos(text, start)
			epos := utf16IndexToBytePos(text, end)
			if tv, ok := v.(*tg.MessageEntityTextURL); ok {
				inserts = append(inserts, insertion{pos: epos, str: fmt.Sprintf("](%s)", escapeAttr(tv.URL))})
				inserts = append(inserts, insertion{pos: spos, str: "["})
			}
		case *tg.MessageEntityMention:
			start := int(v.Offset)
			end := int(v.Offset + v.Length)
			spos := utf16IndexToBytePos(text, start)
			epos := utf16IndexToBytePos(text, end)
		case *tg.MessageEntityMention:
				inserts = append(inserts, insertion{pos: epos, str: fmt.Sprintf("](tg://user?id=%d)", tv.User.GetID())})
				inserts = append(inserts, insertion{pos: spos, str: "["})
		case *tg.MessageEntityCustomEmoji:
			start := int(v.Offset)
			end := int(v.Offset + v.Length)
			spos := utf16IndexToBytePos(text, start)
			epos := utf16IndexToBytePos(text, end)
			if tv, ok := v.(*tg.MessageEntityCustomEmoji); ok {
				inserts = append(inserts, insertion{pos: epos, str: fmt.Sprintf("](tg://emoji?id=%s)", tv.CustomEmojiID)})
				inserts = append(inserts, insertion{pos: spos, str: "!["})
			}
		default:
			// Unknown entity type — ignore
		}
	}

	// Sort inserts by position descending so we can insert without shifting earlier indexes
	sort.SliceStable(inserts, func(i, j int) bool {
		if inserts[i].pos == inserts[j].pos {
			return inserts[i].str < inserts[j].str
		}
		return inserts[i].pos > inserts[j].pos
	})

	// Apply inserts
	out := text
	for _, it := range inserts {
		if it.pos < 0 {
			it.pos = 0
		}
		if it.pos > len(out) {
			it.pos = len(out)
		}
		out = out[:it.pos] + it.str + out[it.pos:]
	}

	return out, nil
}


func init() {
	m := &Markdown{}
	text, _, _ := m.Parse("**A bold\n\n Message 😂**", true)
	fmt.Println(text)
	pOut, _ := htmlparser.Parse("**A bold\n\n Message ⚠️**")
	fmt.Println(pOut.Message)
}
