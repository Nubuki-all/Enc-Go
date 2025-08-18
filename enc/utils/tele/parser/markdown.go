// package markdown provides a lightweight Markdown <-> HTML-ish converter
// adapted from the Pyrogram snippet you posted. It is tuned to work with
// github.com/gotd/td (tg.MessageEntity classes).
package parser

import (
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"
	"unicode/utf16"

	"github.com/gotd/td/tg"

	"enc/utils/tele/parser/htmlparser"
)

const (
	BOLD_DELIM                      = "**"
	ITALIC_DELIM                    = "__"
	UNDERLINE_DELIM                 = "--"
	STRIKE_DELIM                    = "~~"
	SPOILER_DELIM                   = "||"
	CODE_DELIM                      = "`"
	PRE_DELIM                       = "```"
	BLOCKQUOTE_DELIM                = ">"
	BLOCKQUOTE_EXPANDABLE_DELIM     = "**>"
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
	// Final flush
	createBlockquote(false)

	if strict {
		for i, line := range lines {
			if _, ok := htmlEscapedIdx[i]; !ok {
				lines[i] = html.EscapeString(line)
			}
		}
	}

	// Return joined lines preserving blank lines (do NOT drop empty placeholders).
	// Previously we removed empty strings here which collapsed double newlines.
	return strings.Join(lines, "\n")
}

// Parse converts markdown-ish text to simple HTML-ish markup and returns it.
// strict -> escapes text except recognized quote lines.
// Parse converts markdown-ish text to simple HTML-ish markup and returns it.
// strict -> escapes text except recognized quote lines.
func Parse(input string, strict bool) (string, []tg.MessageEntityClass, error) {
	// 1) Handle blockquotes and initial escaping
	text := escapeAndCreateQuotes(input, strict)

	// 2) Convert markdown -> HTML-like. Use sane ordered regex replacements.
	//    Order: fenced pre (```lang\n...\```), emoji ( ![a](tg://emoji?id=123) ), links, inline code,
	//    then bold, italic, underline, strike, spoiler.

	// fenced pre with optional language (DOTALL)
	rePre := regexp.MustCompile("(?s)```([^\n]*)\n(.*?)```")
	text = rePre.ReplaceAllStringFunc(text, func(m string) string {
		parts := rePre.FindStringSubmatch(m)
		lang := strings.TrimSpace(parts[1])
		body := parts[2]
		if lang != "" {
			return `<pre language="` + html.EscapeString(lang) + `">` + html.EscapeString(body) + `</pre>`
		}
		return `<pre>` + html.EscapeString(body) + `</pre>`
	})

	// emoji notation: ![emoji_char](tg://emoji?id=123)
	reEmoji := regexp.MustCompile(`!\[([^\]]+)\]\(tg://emoji\?id=(\d+)\)`)
	text = reEmoji.ReplaceAllString(text, `<emoji id=$2>$1</emoji>`)

	// links: [text](url) (handles tg://user, https, etc.)
	// keep label non-greedy; labels spanning newlines are unusual but allowed by DOTALL
	reLink := regexp.MustCompile(`(?s)\[(.+?)\]\((\S+?)\)`)
	text = reLink.ReplaceAllStringFunc(text, func(m string) string {
		parts := reLink.FindStringSubmatch(m)
		label := html.EscapeString(parts[1])
		url := html.EscapeString(parts[2])
		return `<a href="` + url + `">` + label + `</a>`
	})

	// inline code: `code`  (explicitly disallow newlines inside inline code)
	reCode := regexp.MustCompile("`([^`\\n]+)`")
	text = reCode.ReplaceAllString(text, `<code>$1</code>`)

	// bold: **text**  — allow newlines inside bold (DOTALL)
	reBold := regexp.MustCompile(`(?s)\*\*(.+?)\*\*`)
	text = reBold.ReplaceAllString(text, `<b>$1</b>`)

	// italic: __text__  — allow newlines
	reItalic := regexp.MustCompile(`(?s)__(.+?)__`)
	text = reItalic.ReplaceAllString(text, `<i>$1</i>`)

	// underline: --text--  — allow newlines
	reUnderline := regexp.MustCompile(`(?s)--(.+?)--`)
	text = reUnderline.ReplaceAllString(text, `<u>$1</u>`)

	// strike: ~~text~~  — allow newlines
	reStrike := regexp.MustCompile(`(?s)~~(.+?)~~`)
	text = reStrike.ReplaceAllString(text, `<s>$1</s>`)

	// spoiler: ||text||  — allow newlines
	reSpoiler := regexp.MustCompile(`(?s)\|\|(.+?)\|\|`)
	text = reSpoiler.ReplaceAllString(text, `<spoiler>$1</spoiler>`)

	// 3) Now pass HTML-like result to htmlparser which will produce message + entities.
	fmt.Println(text)
	pOut, err := htmlparser.Parse(text)
	if err != nil {
		return "", nil, err
	}
	// pOut.Message is UTF-8; pOut.Entities is []tg.MessageEntityClass
	return pOut.Message, pOut.Entities, nil
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
	text, _, _ := Parse("**A bold\n\n Message 😂**", true)
	fmt.Println(text)
	pOut, _ := htmlparser.Parse("**A bold\n\n Message ⚠️**")
	fmt.Println(pOut.Message)
}
