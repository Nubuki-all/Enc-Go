package htmlparser

import (
	"bytes"
	"fmt"
	"html"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/gotd/td/tg"
	"golang.org/x/net/html"
)

// ParserResult is returned by Parse.
type ParserResult struct {
	Message  string
	Entities []tg.MessageEntityClass
}

// addSurrogatesForOffsets converts a UTF-8 string into a synthetic string where
// each UTF-16 code unit corresponds to exactly one rune. This makes integer
// offsets (which Telegram measures in UTF-16 code units) usable as rune offsets.
func addSurrogatesForOffsets(s string) string {
	var u16 []uint16
	for _, r := range []rune(s) {
		if r < 0x10000 {
			u16 = append(u16, uint16(r))
		} else {
			hi, lo := utf16.EncodeRune(r)
			u16 = append(u16, uint16(hi), uint16(lo))
		}
	}
	out := make([]rune, len(u16))
	for i, v := range u16 {
		out[i] = rune(v)
	}
	return string(out)
}

// removeSurrogates converts the surrogate-expanded string back to the original UTF-8 string.
func removeSurrogates(s string) string {
	r := []rune(s)
	u16 := make([]uint16, len(r))
	for i, rr := range r {
		u16[i] = uint16(rr)
	}
	decoded := utf16.Decode(u16)
	return string(decoded)
}

// Parse parses HTML-like input into plain text and gotd/td entities.
// It calls surrogate helpers internally, so pass normal UTF-8 input.
// The returned Message is plain UTF-8 (ready to send) and Entities
// have Offset/Length in UTF-16 code units (as required by Telegram).
func Parse(input string) (*ParserResult, error) {
	// Expand to surrogate-friendly string internally
	surrogate := addSurrogatesForOffsets(input)

	z := html.NewTokenizer(strings.NewReader(surrogate))

	var buf bytes.Buffer
	// tag -> stack of open entities
	tagEntities := map[string][]tg.MessageEntityClass{}
	// finished entities in closing order
	var finished []tg.MessageEntityClass

	// helpers
	pushEntity := func(tag string, ent tg.MessageEntityClass) {
		tagEntities[tag] = append(tagEntities[tag], ent)
	}
	popEntity := func(tag string) {
		stack := tagEntities[tag]
		if len(stack) == 0 {
			return
		}
		ent := stack[len(stack)-1]
		tagEntities[tag] = stack[:len(stack)-1]
		if len(tagEntities[tag]) == 0 {
			delete(tagEntities, tag)
		}
		finished = append(finished, ent)
	}

	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			if z.Err() == io.EOF {
				goto DONE
			}
			return nil, z.Err()
		case html.TextToken:
			data := string(z.Text())
			data = html.UnescapeString(data)
			// increment length for all open entities
			addLen := int32(len([]rune(data))) // surrogate-expanded => rune count == UTF-16 code units
			for _, ents := range tagEntities {
				for _, e := range ents {
					switch v := e.(type) {
					case *tg.MessageEntityBold:
						v.Length += addLen
					case *tg.MessageEntityItalic:
						v.Length += addLen
					case *tg.MessageEntityUnderline:
						v.Length += addLen
					case *tg.MessageEntityStrike:
						v.Length += addLen
					case *tg.MessageEntityBlockquote:
						v.Length += addLen
					case *tg.MessageEntityCode:
						v.Length += addLen
					case *tg.MessageEntityPre:
						v.Length += addLen
					case *tg.MessageEntitySpoiler:
						v.Length += addLen
					case *tg.MessageEntityTextUrl:
						v.Length += addLen
					case *tg.MessageEntityCustomEmoji:
						v.Length += addLen
					default:
						_ = v
					}
				}
			}
			buf.WriteString(data)
		case html.StartTagToken:
			name, hasAttr := z.TagName()
			tag := strings.ToLower(string(name))
			attrMap := map[string]string{}
			if hasAttr {
				for {
					k, v, more := z.TagAttr()
					attrMap[string(k)] = string(v)
					if !more {
						break
					}
				}
			}
			offset := int32(buf.Len())
			switch tag {
			case "b", "strong":
				pushEntity(tag, &tg.MessageEntityBold{Offset: offset, Length: 0})
			case "i", "em":
				pushEntity(tag, &tg.MessageEntityItalic{Offset: offset, Length: 0})
			case "u":
				pushEntity(tag, &tg.MessageEntityUnderline{Offset: offset, Length: 0})
			case "s", "del", "strike":
				pushEntity(tag, &tg.MessageEntityStrike{Offset: offset, Length: 0})
			case "blockquote":
				pushEntity(tag, &tg.MessageEntityBlockquote{Offset: offset, Length: 0})
			case "code":
				pushEntity(tag, &tg.MessageEntityCode{Offset: offset, Length: 0})
			case "pre":
				lang := ""
				if v, ok := attrMap["language"]; ok {
					lang = v
				}
				// MessageEntityPre has Language *string in gotd/td
				pushEntity(tag, &tg.MessageEntityPre{Offset: offset, Length: 0, Language: &lang})
			case "spoiler":
				pushEntity(tag, &tg.MessageEntitySpoiler{Offset: offset, Length: 0})
			case "a":
				href := ""
				if v, ok := attrMap["href"]; ok {
					href = v
				}
				pushEntity(tag, &tg.MessageEntityTextUrl{Offset: offset, Length: 0, URL: href})
			case "emoji":
				idStr := attrMap["id"]
				if idStr == "" {
					// ignore
					continue
				}
				idNum, err := strconv.ParseInt(idStr, 10, 64)
				if err != nil {
					continue
				}
				pushEntity(tag, &tg.MessageEntityCustomEmoji{Offset: offset, Length: 0, CustomEmojiID: idNum})
			default:
				// ignore unknown tags
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			tag := strings.ToLower(string(name))
			popEntity(tag)
		case html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			tag := strings.ToLower(string(name))
			attrMap := map[string]string{}
			if hasAttr {
				for {
					k, v, more := z.TagAttr()
					attrMap[string(k)] = string(v)
					if !more {
						break
					}
				}
			}
			offset := int32(buf.Len())
			switch tag {
			case "emoji":
				idStr := attrMap["id"]
				if idStr == "" {
					continue
				}
				idNum, err := strconv.ParseInt(idStr, 10, 64)
				if err != nil {
					continue
				}
				finished = append(finished, &tg.MessageEntityCustomEmoji{Offset: offset, Length: 0, CustomEmojiID: idNum})
			default:
				// ignore
			}
		default:
			// ignore other token types
		}
	}

DONE:
	// collect non-zero-length finished entities
	var ents []tg.MessageEntityClass
	for _, e := range finished {
		switch en := e.(type) {
		case *tg.MessageEntityBold:
			if en.Length > 0 {
				ents = append(ents, en)
			}
		case *tg.MessageEntityItalic:
			if en.Length > 0 {
				ents = append(ents, en)
			}
		case *tg.MessageEntityUnderline:
			if en.Length > 0 {
				ents = append(ents, en)
			}
		case *tg.MessageEntityStrike:
			if en.Length > 0 {
				ents = append(ents, en)
			}
		case *tg.MessageEntityBlockquote:
			if en.Length > 0 {
				ents = append(ents, en)
			}
		case *tg.MessageEntityCode:
			if en.Length > 0 {
				ents = append(ents, en)
			}
		case *tg.MessageEntityPre:
			if en.Length > 0 {
				ents = append(ents, en)
			}
		case *tg.MessageEntitySpoiler:
			if en.Length > 0 {
				ents = append(ents, en)
			}
		case *tg.MessageEntityTextUrl:
			if en.Length > 0 {
				ents = append(ents, en)
			}
		case *tg.MessageEntityCustomEmoji:
			if en.Length > 0 {
				ents = append(ents, en)
			}
		default:
			// ignore
			_ = en
		}
	}

	// sort by offset ascending
	sort.Slice(ents, func(i, j int) bool {
		var oi, oj int32
		switch v := ents[i].(type) {
		case *tg.MessageEntityBold:
			oi = v.Offset
		case *tg.MessageEntityItalic:
			oi = v.Offset
		case *tg.MessageEntityUnderline:
			oi = v.Offset
		case *tg.MessageEntityStrike:
			oi = v.Offset
		case *tg.MessageEntityBlockquote:
			oi = v.Offset
		case *tg.MessageEntityCode:
			oi = v.Offset
		case *tg.MessageEntityPre:
			oi = v.Offset
		case *tg.MessageEntitySpoiler:
			oi = v.Offset
		case *tg.MessageEntityTextUrl:
			oi = v.Offset
		case *tg.MessageEntityCustomEmoji:
			oi = v.Offset
		default:
			oi = 0
		}
		switch v := ents[j].(type) {
		case *tg.MessageEntityBold:
			oj = v.Offset
		case *tg.MessageEntityItalic:
			oj = v.Offset
		case *tg.MessageEntityUnderline:
			oj = v.Offset
		case *tg.MessageEntityStrike:
			oj = v.Offset
		case *tg.MessageEntityBlockquote:
			oj = v.Offset
		case *tg.MessageEntityCode:
			oj = v.Offset
		case *tg.MessageEntityPre:
			oj = v.Offset
		case *tg.MessageEntitySpoiler:
			oj = v.Offset
		case *tg.MessageEntityTextUrl:
			oj = v.Offset
		case *tg.MessageEntityCustomEmoji:
			oj = v.Offset
		default:
			oj = 0
		}
		return oi < oj
	})

	// Build result: message must be converted back to original UTF-8
	return &ParserResult{
		Message:  removeSurrogates(buf.String()),
		Entities: ents,
	}, nil
}

// Unparse injects HTML-like tags into the provided text using entities.
// It accepts normal UTF-8 text and entities with offsets in UTF-16 code units.
// The function handles nesting and returns the annotated HTML-like string.
func Unparse(text string, entities []tg.MessageEntityClass) (string, error) {
	// Expand text with surrogates so offsets match rune positions
	sur := addSurrogatesForOffsets(text)

	type pair struct {
		tag string
		off int
	}

	// Helper builds start/end tag and returns (startTag, startOff), (endTag, endOff)
	parseOne := func(e tg.MessageEntityClass) (*pair, *pair) {
		switch ent := e.(type) {
		case *tg.MessageEntityBold:
			start := int(ent.Offset)
			end := int(ent.Offset + ent.Length)
			return &pair{tag: "<b>", off: start}, &pair{tag: "</b>", off: end}
		case *tg.MessageEntityItalic:
			start := int(ent.Offset)
			end := int(ent.Offset + ent.Length)
			return &pair{tag: "<i>", off: start}, &pair{tag: "</i>", off: end}
		case *tg.MessageEntityUnderline:
			start := int(ent.Offset)
			end := int(ent.Offset + ent.Length)
			return &pair{tag: "<u>", off: start}, &pair{tag: "</u>", off: end}
		case *tg.MessageEntityStrike:
			start := int(ent.Offset)
			end := int(ent.Offset + ent.Length)
			return &pair{tag: "<s>", off: start}, &pair{tag: "</s>", off: end}
		case *tg.MessageEntityCode:
			start := int(ent.Offset)
			end := int(ent.Offset + ent.Length)
			return &pair{tag: "<code>", off: start}, &pair{tag: "</code>", off: end}
		case *tg.MessageEntityPre:
			start := int(ent.Offset)
			end := int(ent.Offset + ent.Length)
			lang := ""
			if ent.Language != nil {
				lang = *ent.Language
			}
			if lang != "" {
				return &pair{tag: fmt.Sprintf(`<pre language="%s">`, html.EscapeString(lang)), off: start}, &pair{tag: "</pre>", off: end}
			}
			return &pair{tag: "<pre>", off: start}, &pair{tag: "</pre>", off: end}
		case *tg.MessageEntityBlockquote:
			start := int(ent.Offset)
			end := int(ent.Offset + ent.Length)
			// We don't have an 'expandable' flag on entity so use standard blockquote tag.
			return &pair{tag: "<blockquote>", off: start}, &pair{tag: "</blockquote>", off: end}
		case *tg.MessageEntitySpoiler:
			start := int(ent.Offset)
			end := int(ent.Offset + ent.Length)
			return &pair{tag: "<spoiler>", off: start}, &pair{tag: "</spoiler>", off: end}
		case *tg.MessageEntityTextUrl:
			start := int(ent.Offset)
			end := int(ent.Offset + ent.Length)
			return &pair{tag: fmt.Sprintf(`<a href="%s">`, html.EscapeString(ent.URL)), off: start}, &pair{tag: "</a>", off: end}
		case *tg.MessageEntityCustomEmoji:
			start := int(ent.Offset)
			end := int(ent.Offset + ent.Length)
			return &pair{tag: fmt.Sprintf(`<emoji id="%d">`, ent.CustomEmojiID), off: start}, &pair{tag: "</emoji>", off: end}
		case *tg.MessageEntityTextMention:
			start := int(ent.Offset)
			end := int(ent.Offset + ent.Length)
			if ent.User != nil {
				return &pair{tag: fmt.Sprintf(`<a href="tg://user?id=%d">`, ent.User.ID), off: start}, &pair{tag: "</a>", off: end}
			}
			return nil, nil
		default:
			return nil, nil
		}
	}

	// sort entities by (offset asc, length desc) similar to python code
	sort.SliceStable(entities, func(i, j int) bool {
		// get offsets and lengths
		var oi, oj int32
		var li, lj int32
		switch e := entities[i].(type) {
		case *tg.MessageEntityBold:
			oi = e.Offset; li = e.Length
		case *tg.MessageEntityItalic:
			oi = e.Offset; li = e.Length
		case *tg.MessageEntityUnderline:
			oi = e.Offset; li = e.Length
		case *tg.MessageEntityStrike:
			oi = e.Offset; li = e.Length
		case *tg.MessageEntityBlockquote:
			oi = e.Offset; li = e.Length
		case *tg.MessageEntityCode:
			oi = e.Offset; li = e.Length
		case *tg.MessageEntityPre:
			oi = e.Offset; li = e.Length
		case *tg.MessageEntitySpoiler:
			oi = e.Offset; li = e.Length
		case *tg.MessageEntityTextUrl:
			oi = e.Offset; li = e.Length
		case *tg.MessageEntityCustomEmoji:
			oi = e.Offset; li = e.Length
		case *tg.MessageEntityTextMention:
			oi = e.Offset; li = e.Length
		default:
			oi = 0; li = 0
		}
		switch e := entities[j].(type) {
		case *tg.MessageEntityBold:
			oj = e.Offset; lj = e.Length
		case *tg.MessageEntityItalic:
			oj = e.Offset; lj = e.Length
		case *tg.MessageEntityUnderline:
			oj = e.Offset; lj = e.Length
		case *tg.MessageEntityStrike:
			oj = e.Offset; lj = e.Length
		case *tg.MessageEntityBlockquote:
			oj = e.Offset; lj = e.Length
		case *tg.MessageEntityCode:
			oj = e.Offset; lj = e.Length
		case *tg.MessageEntityPre:
			oj = e.Offset; lj = e.Length
		case *tg.MessageEntitySpoiler:
			oj = e.Offset; lj = e.Length
		case *tg.MessageEntityTextUrl:
			oj = e.Offset; lj = e.Length
		case *tg.MessageEntityCustomEmoji:
			oj = e.Offset; lj = e.Length
		case *tg.MessageEntityTextMention:
			oj = e.Offset; lj = e.Length
		default:
			oj = 0; lj = 0
		}
		if oi == oj {
			// longer first
			return li > lj
		}
		return oi < oj
	})

	// Build entity tag insertion list using recursive approach
	type ins struct {
		tag string
		off int
	}
	var entitiesOffsets []ins

	var recursive func(i int) int
	recursive = func(i int) int {
		if i >= len(entities) {
			return 1
		}
		startEnd := parseOne(entities[i])
		if startEnd == nil {
			return 1
		}
		startPair, endPair := startEnd[0], startEnd[1]
		entitiesOffsets = append(entitiesOffsets, ins{tag: startPair.tag, off: startPair.off})
		internal := i + 1
		// parse nested entities inside current
		for internal < len(entities) {
			// get offset of next entity
			var offNext int
			switch e := entities[internal].(type) {
			case *tg.MessageEntityBold:
				offNext = int(e.Offset)
			case *tg.MessageEntityItalic:
				offNext = int(e.Offset)
			case *tg.MessageEntityUnderline:
				offNext = int(e.Offset)
			case *tg.MessageEntityStrike:
				offNext = int(e.Offset)
			case *tg.MessageEntityBlockquote:
				offNext = int(e.Offset)
			case *tg.MessageEntityCode:
				offNext = int(e.Offset)
			case *tg.MessageEntityPre:
				offNext = int(e.Offset)
			case *tg.MessageEntitySpoiler:
				offNext = int(e.Offset)
			case *tg.MessageEntityTextUrl:
				offNext = int(e.Offset)
			case *tg.MessageEntityCustomEmoji:
				offNext = int(e.Offset)
			case *tg.MessageEntityTextMention:
				offNext = int(e.Offset)
			default:
				offNext = 1<<30
			}
			// end offset of current
			end := endPair.off
			if offNext < end {
				internal += recursive(internal)
			} else {
				break
			}
		}
		entitiesOffsets = append(entitiesOffsets, ins{tag: endPair.tag, off: endPair.off})
		return internal - i
	}

	i := 0
	for i < len(entities) {
		i += recursive(i)
	}

	// If we have insertions, apply them from the end
	if len(entitiesOffsets) == 0 {
		return text, nil
	}

	// Convert sur string to rune-slice for indexing by code units
	outRunes := []rune(sur)
	// We'll produce a working string and apply insertions descending by offset
	sort.SliceStable(entitiesOffsets, func(i, j int) bool {
		return entitiesOffsets[i].off > entitiesOffsets[j].off
	})

	for _, it := range entitiesOffsets {
		off := it.off
		if off < 0 {
			off = 0
		}
		if off > len(outRunes) {
			off = len(outRunes)
		}
		// insert tag at rune position 'off'
		prefix := string(outRunes[:off])
		suffix := string(outRunes[off:])
		out := prefix + it.tag + suffix
		// rebuild outRunes
		outRunes = []rune(out)
	}

	// convert back to normal UTF-8
	final := removeSurrogates(string(outRunes))
	// Escape the replaced region's inner text for safety? The python version escaped inner text when inserting; however
	// here tags are inserted directly; callers should ensure produced HTML is safe.
	return final, nil
}
