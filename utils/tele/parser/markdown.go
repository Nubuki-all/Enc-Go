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
/*var markdownRe = regexp.MustCompile(
	fmt.Sprintf(`(%s|%s|%s|%s|%s|%s|%s)|(!?)\[(.+?)\]\((.+?)\)`,
		regexp.QuoteMeta(PRE_DELIM),
		regexp.QuoteMeta(CODE_DELIM),
		regexp.QuoteMeta(STRIKE_DELIM),
		regexp.QuoteMeta(UNDERLINE_DELIM),
		regexp.QuoteMeta(ITALIC_DELIM),
		regexp.QuoteMeta(BOLD_DELIM),
		regexp.QuoteMeta(SPOILER_DELIM),
	),
)*/

// HTML templates
// func opening(tag string) string { return "<" + tag + ">" }

func escapeAttr(s string) string {
	return strings.ReplaceAll(html.EscapeString(s), `"`, "&quot;")
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
			strip = strings.TrimPrefix(strip, " ")
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
				line = strings.TrimPrefix(line, " ")
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
			strip = strings.TrimPrefix(strip, " ")
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
			strip = strings.TrimPrefix(strip, " ")
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
	// helper: validate language token (only allow simple tokens, no spaces)
	langValidRe := regexp.MustCompile(`^[A-Za-z0-9_\-\+]{1,25}$`)

	// Finished code blocks and link targets are swapped for placeholders so
	// the bold/italic/... regexes below can't inject tags into them (e.g.
	// "--" or "__" in a file name or URL).
	var protected []string
	protect := func(s string) string {
		protected = append(protected, s)
		return "\x00" + strconv.Itoa(len(protected)-1) + "\x00"
	}

	// fenced pre with optional language (DOTALL) - same regex, but we'll validate the captured lang
	rePre := regexp.MustCompile("(?s)```([^\n]*)\n(.*?)```")
	text = rePre.ReplaceAllStringFunc(text, func(m string) string {
		parts := rePre.FindStringSubmatch(m)
		lang := strings.TrimSpace(parts[1])
		body := parts[2]

		// Prevent out of bounds errors
		// if strings.HasSuffix(body, "\n") {
		// 	body = strings.TrimRight(body, "\n")
		// }
		// If the captured "lang" looks suspicious (contains spaces or too long), treat it as part of the body.
		if !langValidRe.MatchString(lang) {
			// Put it back into the body (we need original newline between lang and body)
			if lang != "" {
				body = lang + "\n" + body
			}
			lang = ""
		}

		// Avoid double-escaping: when strict==true the input has already been escaped by
		// escapeAndCreateQuotes. Only escape now if strict==false.
		if !strict {
			body = html.EscapeString(body)
		} // else {
		// If strict is true, we assume the body has already been escaped by prior step.
		// However as a safety measure, if body contains raw '<' or '&' sequences that look unescaped,
		// you could optionally call html.EscapeString here. (Not done to avoid double-escape.)
		// }

		if lang != "" {
			// Escape attribute value safely (quotes etc.)
			return protect(`<pre language="` + escapeAttr(lang) + `">` + body + `</pre>`)
		}
		return protect(`<pre>` + body + `</pre>`)
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
		return protect(`<a href="`+url+`">`) + label + `</a>`
	})

	// inline code: `code`  (explicitly disallow newlines inside inline code)
	reCode := regexp.MustCompile("`([^`\\n]+)`")
	text = reCode.ReplaceAllStringFunc(text, func(m string) string {
		return protect(`<code>` + m[1:len(m)-1] + `</code>`)
	})

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

	// Restore protected pieces; repeat since a code span can hold a link.
	reProtected := regexp.MustCompile("\x00(\\d+)\x00")
	for reProtected.MatchString(text) {
		text = reProtected.ReplaceAllStringFunc(text, func(m string) string {
			i, _ := strconv.Atoi(m[1 : len(m)-1])
			return protected[i]
		})
	}

	// 3) Now pass HTML-like result to htmlparser which will produce message + entities.
	// fmt.Println(text)
	return htmlparser.Parse(text)
	// if err != nil {
	// 	return "", nil, err
	// }
	// fmt.Println(pOut.Message)
	// // pOut.Message is UTF-8; pOut.Entities is []tg.MessageEntityClass
	// return pOut.Message, pOut.Entities, nil
}

// Unparse converts a text and tg message entities into Markdown-ish plaintext (reconstructing delimiters).
// Entities must be in the format produced by gotd/td (e.g. *tg.MessageEntityBold, *tg.MessageEntityItalic, ...).
func Unparse(text string, entities []tg.MessageEntityClass) (string, error) {
	// Entity offsets count UTF-16 code units, so work on those directly.
	u16 := utf16.Encode([]rune(text))
	clamp := func(off int) int { return max(0, min(off, len(u16))) }

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
			start, end := clamp(int(ent.Offset)), clamp(int(ent.Offset+ent.Length))
			insertions = append(insertions, ins{start, BLOCKQUOTE_DELIM + " "})
			for i := start; i < end; i++ {
				if u16[i] == '\n' {
					// each quoted line starts with the delimiter
					insertions = append(insertions, ins{i + 1, BLOCKQUOTE_DELIM + " "})
				}
			}
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
	sort.SliceStable(insertions, func(i, j int) bool { return insertions[i].off > insertions[j].off })

	out := u16
	for _, it := range insertions {
		off := max(0, min(it.off, len(out)))
		tag := utf16.Encode([]rune(it.s))
		out = append(out[:off], append(tag, out[off:]...)...)
	}
	return string(utf16.Decode(out)), nil
}
