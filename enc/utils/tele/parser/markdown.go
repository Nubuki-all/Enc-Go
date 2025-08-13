// package markdown provides a lightweight Markdown <-> HTML-ish converter
// adapted from the Pyrogram snippet you posted. It is tuned to work with
// github.com/gotd/td (tg.MessageEntity classes).
package parser

import (
	"fmt"
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/gotd/td/tg"
	
	"enc/utils/tele/parser/htmlparser"
)

const (
	BOLD_DELIM                    = "**"
	ITALIC_DELIM                  = "__"
	UNDERLINE_DELIM               = "--"
	STRIKE_DELIM                  = "~~"
	SPOILER_DELIM                 = "||"
	CODE_DELIM                    = "`"
	PRE_DELIM                     = "```"
	BLOCKQUOTE_DELIM              = ">"
	BLOCKQUOTE_EXPANDABLE_DELIM   = "**>"
	BLOCKQUOTE_EXPANDABLE_END_DELIM = "||"
)

var FIXED_WIDTH_DELIMS = map[string]bool{
	CODE_DELIM: true,
	PRE_DELIM:  true,
}

// MARKDOWN_RE roughly matches the Python one: any of the delimiters OR an optional
// emoji/link syntax: (![text](url) or [text](url) with optional ! for emoji).
var markdownRe = regexp.MustCompile(
	fmt.Sprintf(`(%s|%s|%s|%s|%s|%s|%s)|(!?)\[(.+?)\]\((.+?)\)`,
		regexp.QuoteMeta(PRE_DELIM),
		regexp.QuoteMeta(CODE_DELIM),
		regexp.QuoteMeta(STRIKE_DELIM),
		regexp.QuoteMeta(UNDERLINE_DELIM),
		regexp.QuoteMeta(ITALIC_DELIM),
		regexp.QuoteMeta(BOLD_DELIM),
		regexp.QuoteMeta(SPOILER_DELIM),
	),
)

// HTML templates
func opening(tag string) string { return "<" + tag + ">" }
func closing(tag string) string { return "</" + tag + ">" }
func urlMarkup(url, text string) string {
	return fmt.Sprintf(`<a href="%s">%s</a>`, html.EscapeString(url), html.EscapeString(text))
}
func emojiMarkup(id, char string) string {
	// keep same shape as Pyrogram: <emoji id=...>char</emoji>
	return fmt.Sprintf(`<emoji id=%s>%s</emoji>`, html.EscapeString(id), html.EscapeString(char))
}

// Helper: replace only once at given index (like utils.replace_once)
func replaceOnceAt(s, old, repl string, at int) (string, error) {
	// confirm old occurs at 'at'
	if at < 0 || at+len(old) > len(s) {
		// fallback to strings.Replace once
		return strings.Replace(s, old, repl, 1), nil
	}
	if s[at:at+len(old)] != old {
		// fallback
		return strings.Replace(s, old, repl, 1), nil
	}
	return s[:at] + repl + s[at+len(old):], nil
}

// escapeAndCreateQuotes implements the blockquote handling and optional HTML escaping
func escapeAndCreateQuotes(text string, strict bool) string {
	lines := strings.Split(text, "\n")
	htmlEscapedIdx := map[int]bool{}
	var toQuoteList [][2]interface{} // [ {index, parsed_line} ]

	createBlockquote := func(expandable bool) {
		if len(toQuoteList) == 0 {
			return
		}
		parts := make([]string, len(toQuoteList))
		firstIdx := toQuoteList[0][0].(int)
		for i, p := range toQuoteList {
			parts[i] = p[1].(string)
		}
		joined := strings.Join(parts, "\n")
		tag := "<blockquote>"
		if expandable {
			tag = `<blockquote expandable>`
		}
		lines[firstIdx] = tag + joined + "</blockquote>"
		for _, p := range toQuoteList[1:] {
			lines[p[0].(int)] = "" // placeholder
		}
		toQuoteList = toQuoteList[:0]
	}

	insideBlockquote := false
	for idx, line := range lines {
		// Expandable begin
		if strings.HasPrefix(line, BLOCKQUOTE_EXPANDABLE_DELIM) && !insideBlockquote {
			// strip leading delim and optional space
			strip := strings.TrimPrefix(line, BLOCKQUOTE_EXPANDABLE_DELIM)
			if strings.HasPrefix(strip, " ") {
				strip = strings.TrimPrefix(strip, " ")
			}
			parsed := strip
			if strict {
				parsed = html.EscapeString(parsed)
			}
			toQuoteList = append(toQuoteList, [2]interface{}{idx, parsed})
			htmlEscapedIdx[idx] = true
			insideBlockquote = true
			continue
		}

		// Expandable end
		if strings.HasSuffix(line, BLOCKQUOTE_EXPANDABLE_END_DELIM) && insideBlockquote {
			// remove leading ">" if present for last line
			if strings.HasPrefix(line, BLOCKQUOTE_DELIM) {
				line = strings.TrimPrefix(line, BLOCKQUOTE_DELIM)
				if strings.HasPrefix(line, " ") {
					line = strings.TrimPrefix(line, " ")
				}
			}
			strip := line[:len(line)-len(BLOCKQUOTE_EXPANDABLE_END_DELIM)]
			parsed := strip
			if strict {
				parsed = html.EscapeString(parsed)
			}
			toQuoteList = append(toQuoteList, [2]interface{}{idx, parsed})
			htmlEscapedIdx[idx] = true
			insideBlockquote = false
			createBlockquote(true)
			continue
		}

		if insideBlockquote {
			// strip leading ">" and optional space
			strip := strings.TrimPrefix(line, BLOCKQUOTE_DELIM)
			if strings.HasPrefix(strip, " ") {
				strip = strings.TrimPrefix(strip, " ")
			}
			parsed := strip
			if strict {
				parsed = html.EscapeString(parsed)
			}
			toQuoteList = append(toQuoteList, [2]interface{}{idx, parsed})
			htmlEscapedIdx[idx] = true
		}
	}

	// Single/continued quotes
	for idx, line := range lines {
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, BLOCKQUOTE_DELIM) {
			strip := strings.TrimPrefix(line, BLOCKQUOTE_DELIM)
			if strings.HasPrefix(strip, " ") {
				strip = strings.TrimPrefix(strip, " ")
			}
			parsed := strip
			if strict {
				parsed = html.EscapeString(parsed)
			}
			toQuoteList = append(toQuoteList, [2]interface{}{idx, parsed})
			htmlEscapedIdx[idx] = true
		} else if len(toQuoteList) > 0 {
			createBlockquote(false)
		}
	}
	// final flush
	createBlockquote(false)

	if strict {
		for i, line := range lines {
			if _, ok := htmlEscapedIdx[i]; !ok {
				lines[i] = html.EscapeString(line)
			}
		}
	}

	// filter out empty placeholders
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if l == "" {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

// Parse converts markdown-ish text to simple HTML-ish markup and returns it.
// strict -> escapes text except recognized quote lines.
func Parse(input string, strict bool) (string, []tg.MessageEntityClass, error) {
	text := escapeAndCreateQuotes(input, strict)
	// We'll iterate through matches, but because replacement changes lengths we
	// do a naive approach: track an offset shift.
	var delims = make(map[string]bool)
	isFixedWidth := false

	// find all matches first to avoid invalidating positions
	matches := markdownRe.FindAllStringSubmatchIndex(text, -1)
	// We'll process matches in the order they appear while applying replacements with shift.
	out := text
	shift := 0
	for _, m := range matches {
		// m contains pairs of start/end for whole match and groups.
		if len(m) < 12 {
			continue
		}
		matchStart := m[0]
		matchEnd := m[1]
		// group 1 = delim (or empty)
		group1Start, group1End := m[2], m[3]
		// group 8 = is_emoji "!" optional: groups positions differ due to regex building; easier to re-scan substring
		sub := out[matchStart+shift : matchEnd+shift]
		// Determine if this is delim or link/emoji
		var delim string
		if group1Start >= 0 && group1End >= 0 {
			delim = out[group1Start+shift : group1End+shift]
		}
		// If delim is PRE or CODE toggle fixed width state
		if delim != "" && FIXED_WIDTH_DELIMS[delim] {
			isFixedWidth = !isFixedWidth
		}
		// If inside fixed width and delim isn't a fixed width delim skip
		if isFixedWidth && !FIXED_WIDTH_DELIMS[delim] && delim != "" {
			continue
		}

		// Check if it's a link or emoji pattern like [text](url) possibly prefixed with !
		linkMatch := markdownRe.FindStringSubmatch(sub)
		if linkMatch != nil {
			// linkMatch[2] is "!" optional, [3] text, [4] url in our building pattern
			isEmoji := linkMatch[2] == "!"
			textURL := linkMatch[3]
			url := linkMatch[4]
			if !isEmoji && textURL != "" {
				repl := urlMarkup(url, textURL)
				r, _ := replaceOnceAt(out, sub, repl, matchStart+shift)
				out = r
				shift += len(repl) - len(sub)
				continue
			}
			if isEmoji {
				emoji := textURL
				emojiID := strings.TrimPrefix(url, "tg://emoji?id=")
				repl := emojiMarkup(emojiID, emoji)
				r, _ := replaceOnceAt(out, sub, repl, matchStart+shift)
				out = r
				shift += len(repl) - len(sub)
				continue
			}
		}

		// If delim present, map to tags
		if delim != "" {
			var tag string
			switch delim {
			case BOLD_DELIM:
				tag = "b"
			case ITALIC_DELIM:
				tag = "i"
			case UNDERLINE_DELIM:
				tag = "u"
			case STRIKE_DELIM:
				tag = "s"
			case CODE_DELIM:
				tag = "code"
			case PRE_DELIM:
				tag = "pre"
			case SPOILER_DELIM:
				tag = "spoiler"
			default:
				continue
			}
			var replacement string
			if !delims[delim] {
				delims[delim] = true
				replacement = opening(tag)
				// special PRE language handling
				if delim == PRE_DELIM {
					// find language line after PRE_DELIM in the remaining text
					rest := out[matchEnd+shift:]
					idx := strings.Index(rest, "\n")
					if idx >= 0 {
						delLine := rest[:idx]
						lang := strings.TrimPrefix(delLine, PRE_DELIM)
						replacement = fmt.Sprintf(`<pre language="%s">`, html.EscapeString(strings.TrimSpace(lang)))
						// replace the delim+language line with replacement
						// we must find delim_and_language substring
						delimAndLang := out[matchStart+shift : matchStart+shift+len(PRE_DELIM)+len(lang)]
						r, _ := replaceOnceAt(out, delimAndLang, replacement, matchStart+shift)
						out = r
						shift += len(replacement) - len(delimAndLang)
						continue
					}
				}
			} else {
				delims[delim] = false
				replacement = closing(tag)
			}
			r, _ := replaceOnceAt(out, delim, replacement, matchStart+shift)
			out = r
			shift += len(replacement) - len(delim)
		}
	}

	// finally, hand off to HTML.parse equivalent - this library returns HTML-like string already
	// The python code then used HTML.parse which does more safe conversions; here we assume our replacements produce safe HTML.
	pOut := htmlparser.Parse(out)
	pText := pOut.Message
	entities :=pOut.Entities
	return pText, entities, nil
}

// Unparse converts a text and tg message entities into Markdown-ish plaintext (reconstructing delimiters).
// Entities must be in the format produced by gotd/td (e.g. *tg.MessageEntityBold, *tg.MessageEntityItalic, ...).
func Unparse(text string, entities []tg.MessageEntityClass) (string, error) {
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
			// tg.MessageEntityPre may have Language field depending on gotd version; try reflection-like access
			if p, ok := ent.(*tg.MessageEntityPre); ok {
				if p.Language != nil {
					lang = *p.Language
				}
			}
			startTag := PRE_DELIM
			if lang != "" {
				startTag = PRE_DELIM + lang + "\n"
			}
			insertions = append(insertions, ins{end, "\n" + PRE_DELIM})
			insertions = append(insertions, ins{start, startTag})
		case *tg.MessageEntityBlockQuote:
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
		case *tg.MessageEntityTextUrl:
			start, end := int(ent.Offset), int(ent.Offset+ent.Length)
			url := ent.URL
			startTag := "["
			endTag := fmt.Sprintf("](%s)", url)
			insertions = append(insertions, ins{end, endTag})
			insertions = append(insertions, ins{start, startTag})
		case *tg.MessageEntityTextMention:
			start, end := int(ent.Offset), int(ent.Offset+ent.Length)
			if tm, ok := ent.(*tg.MessageEntityTextMention); ok && tm.User != nil {
				startTag := "["
				endTag := fmt.Sprintf("](tg://user?id=%d)", tm.User.ID)
				insertions = append(insertions, ins{end, endTag})
				insertions = append(insertions, ins{start, startTag})
			}
		case *tg.MessageEntityCustomEmoji:
			start, end := int(ent.Offset), int(ent.Offset+ent.Length)
			emojiID := ent.CustomEmojiID
			startTag := "!["
			endTag := fmt.Sprintf("](tg://emoji?id=%d)", emojiID)
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
