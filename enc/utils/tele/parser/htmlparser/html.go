package htmlparser

import (
	"bytes"
	"fmt"
	stdhtml "html"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/gotd/td/tg"
	htmlpkg "golang.org/x/net/html"
)

// ParserResult holds parse results.
type ParserResult struct {
	Message  string
	Entities []tg.MessageEntityClass
}

// addSurrogatesForOffsets converts a UTF-8 string into a surrogate-expanded
// representation where each UTF-16 code unit corresponds to one rune.
// This simplifies offset/length arithmetic because Telegram uses UTF-16 code units.
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
	outRunes := make([]rune, len(u16))
	for i, v := range u16 {
		outRunes[i] = rune(v)
	}
	return string(outRunes)
}

// removeSurrogates decodes surrogate-expanded string back to the original UTF-8.
func removeSurrogates(s string) string {
	r := []rune(s)
	u16 := make([]uint16, len(r))
	for i, rr := range r {
		u16[i] = uint16(rr)
	}
	decoded := utf16.Decode(u16)
	return string(decoded)
}

// utf16Units returns how many UTF-16 code units are needed for s.
func utf16Units(s string) int {
	cnt := 0
	for _, r := range s {
		if r > 0xFFFF {
			cnt += 2
		} else {
			cnt += 1
		}
	}
	return cnt
}

// runeIndexFromUTF16Offset returns the rune index in s that corresponds to the given
// UTF-16 code-unit offset. If offset is beyond end, returns len([]rune(s)).
func runeIndexFromUTF16Offset(s string, offset int) int {
	runes := []rune(s)
	count := 0
	for i, r := range runes {
		cu := 1
		if r > 0xFFFF {
			cu = 2
		}
		if count+cu > offset {
			return i
		}
		count += cu
	}
	return len(runes)
}

// Parse converts HTML-like input into plain UTF-8 message and gotd/td entities.
// Accepts normal UTF-8 input; surrogate expansion is handled internally.
// Returned Entities have Offsets/Lengths expressed in UTF-16 code units (as ints).
func Parse(input string) (*ParserResult, error) {
	// Use UTF-8 input for tokenizer (do NOT expand surrogates here)
	z := htmlpkg.NewTokenizer(strings.NewReader(input))

	var buf bytes.Buffer
	tagEntities := map[string][]tg.MessageEntityClass{}
	var finished []tg.MessageEntityClass

	// posUTF16 counts UTF-16 code units consumed so far
	posUTF16 := 0

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
		case htmlpkg.ErrorToken:
			if z.Err() == io.EOF {
				goto DONE
			}
			return nil, z.Err()
		case htmlpkg.TextToken:
			data := string(z.Text())
			data = stdhtml.UnescapeString(data)
			// update entity lengths using UTF-16 code unit counts
			addLen := utf16Units(data)
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
					case *tg.MessageEntityTextURL:
						v.Length += addLen
					case *tg.MessageEntityCustomEmoji:
						v.Length += addLen
					default:
						_ = v
					}
				}
			}
			buf.WriteString(data)
			posUTF16 += addLen

		case htmlpkg.StartTagToken:
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
			offset := posUTF16
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
				pushEntity(tag, &tg.MessageEntityPre{Offset: offset, Length: 0, Language: lang})
			case "spoiler":
				pushEntity(tag, &tg.MessageEntitySpoiler{Offset: offset, Length: 0})
			case "a":
				href := ""
				if v, ok := attrMap["href"]; ok {
					href = v
				}
				pushEntity(tag, &tg.MessageEntityTextURL{Offset: offset, Length: 0, URL: href})
			case "emoji":
				idStr := attrMap["id"]
				if idStr == "" {
					continue
				}
				idNum, err := strconv.ParseInt(idStr, 10, 64)
				if err != nil {
					continue
				}
				pushEntity(tag, &tg.MessageEntityCustomEmoji{Offset: offset, Length: 0, DocumentID: idNum})
			default:
				// ignore
			}

		case htmlpkg.EndTagToken:
			name, _ := z.TagName()
			tag := strings.ToLower(string(name))
			popEntity(tag)

		case htmlpkg.SelfClosingTagToken:
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
			offset := posUTF16
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
				finished = append(finished, &tg.MessageEntityCustomEmoji{Offset: offset, Length: 0, DocumentID: idNum})
			default:
			}
		default:
		}
	}

DONE:

	// collect non-zero-length entities
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
		case *tg.MessageEntityTextURL:
			if en.Length > 0 {
				ents = append(ents, en)
			}
		case *tg.MessageEntityCustomEmoji:
			if en.Length > 0 {
				ents = append(ents, en)
			}
		default:
			_ = en
		}
	}

	// sort by offset ascending
	sort.Slice(ents, func(i, j int) bool {
		var oi, oj int
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
		case *tg.MessageEntityTextURL:
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
		case *tg.MessageEntityTextURL:
			oj = v.Offset
		case *tg.MessageEntityCustomEmoji:
			oj = v.Offset
		default:
			oj = 0
		}
		return oi < oj
	})

	res := &ParserResult{
		Message:  buf.String(),
		Entities: ents,
	}
	return res, nil
}

// Unparse inserts HTML-like tags into text using entities. Accepts UTF-8 text and
// entities whose Offset/Length are in UTF-16 code units.
func Unparse(text string, entities []tg.MessageEntityClass) (string, error) {
	// surrogate expand so offsets correspond to rune positions
	sur := addSurrogatesForOffsets(text)

	type ins struct {
		tag string
		off int
	}

	// helper: build start/end tag pairs, robust across gotd versions
	parseOne := func(e tg.MessageEntityClass) (*ins, *ins) {
		switch ent := e.(type) {
		case *tg.MessageEntityBold:
			s := int(ent.Offset)
			ei := int(ent.Offset + ent.Length)
			return &ins{tag: "<b>", off: s}, &ins{tag: "</b>", off: ei}
		case *tg.MessageEntityItalic:
			s := int(ent.Offset)
			ei := int(ent.Offset + ent.Length)
			return &ins{tag: "<i>", off: s}, &ins{tag: "</i>", off: ei}
		case *tg.MessageEntityUnderline:
			s := int(ent.Offset)
			ei := int(ent.Offset + ent.Length)
			return &ins{tag: "<u>", off: s}, &ins{tag: "</u>", off: ei}
		case *tg.MessageEntityStrike:
			s := int(ent.Offset)
			ei := int(ent.Offset + ent.Length)
			return &ins{tag: "<s>", off: s}, &ins{tag: "</s>", off: ei}
		case *tg.MessageEntityCode:
			s := int(ent.Offset)
			ei := int(ent.Offset + ent.Length)
			return &ins{tag: "<code>", off: s}, &ins{tag: "</code>", off: ei}
		case *tg.MessageEntityPre:
			s := int(ent.Offset)
			ei := int(ent.Offset + ent.Length)
			lang := ent.Language
			if lang != "" {
				return &ins{tag: fmt.Sprintf(`<pre language="%s">`, stdhtml.EscapeString(lang)), off: s}, &ins{tag: "</pre>", off: ei}
			}
			return &ins{tag: "<pre>", off: s}, &ins{tag: "</pre>", off: ei}
		case *tg.MessageEntityBlockquote:
			s := int(ent.Offset)
			ei := int(ent.Offset + ent.Length)
			return &ins{tag: "<blockquote>", off: s}, &ins{tag: "</blockquote>", off: ei}
		case *tg.MessageEntitySpoiler:
			s := int(ent.Offset)
			ei := int(ent.Offset + ent.Length)
			return &ins{tag: "<spoiler>", off: s}, &ins{tag: "</spoiler>", off: ei}
		case *tg.MessageEntityTextURL:
			s := int(ent.Offset)
			ei := int(ent.Offset + ent.Length)
			return &ins{tag: fmt.Sprintf(`<a href="%s">`, stdhtml.EscapeString(ent.URL)), off: s}, &ins{tag: "</a>", off: ei}
		case *tg.MessageEntityCustomEmoji:
			s := int(ent.Offset)
			ei := int(ent.Offset + ent.Length)
			// DocumentID is used by this gotd version
			docID := int64(0)
			// use reflection fallback to find DocumentID field if necessary
			rv := reflect.ValueOf(ent).Elem()
			if f := rv.FieldByName("DocumentID"); f.IsValid() && f.Kind() == reflect.Int64 {
				docID = f.Int()
			} else if f := rv.FieldByName("DocumentId"); f.IsValid() && f.Kind() == reflect.Int64 {
				docID = f.Int()
			}
			if docID != 0 {
				return &ins{tag: fmt.Sprintf(`<emoji id="%d">`, docID), off: s}, &ins{tag: "</emoji>", off: ei}
			}
			return nil, nil
		case *tg.MessageEntityMention:
			// Try to extract a User.ID if available (some gotd versions store User inside)
			s := int(ent.Offset)
			ei := int(ent.Offset + ent.Length)
			rv := reflect.ValueOf(ent).Elem()
			if f := rv.FieldByName("User"); f.IsValid() && f.Kind() == reflect.Ptr && !f.IsNil() {
				userVal := f.Elem()
				if idf := userVal.FieldByName("ID"); idf.IsValid() && idf.Kind() == reflect.Int64 {
					uid := idf.Int()
					return &ins{tag: fmt.Sprintf(`<a href="tg://user?id=%d">`, uid), off: s}, &ins{tag: "</a>", off: ei}
				}
			}
			// fallback: cannot build mention tag
			return nil, nil
		default:
			return nil, nil
		}
	}

	// sort entities by offset asc, length desc to mimic Python behavior
	sort.SliceStable(entities, func(i, j int) bool {
		var oi, oj, li, lj int
		switch e := entities[i].(type) {
		case *tg.MessageEntityBold:
			oi, li = int(e.Offset), int(e.Length)
		case *tg.MessageEntityItalic:
			oi, li = int(e.Offset), int(e.Length)
		case *tg.MessageEntityUnderline:
			oi, li = int(e.Offset), int(e.Length)
		case *tg.MessageEntityStrike:
			oi, li = int(e.Offset), int(e.Length)
		case *tg.MessageEntityBlockquote:
			oi, li = int(e.Offset), int(e.Length)
		case *tg.MessageEntityCode:
			oi, li = int(e.Offset), int(e.Length)
		case *tg.MessageEntityPre:
			oi, li = int(e.Offset), int(e.Length)
		case *tg.MessageEntitySpoiler:
			oi, li = int(e.Offset), int(e.Length)
		case *tg.MessageEntityTextURL:
			oi, li = int(e.Offset), int(e.Length)
		case *tg.MessageEntityCustomEmoji:
			oi, li = int(e.Offset), int(e.Length)
		case *tg.MessageEntityMention:
			oi, li = int(e.Offset), int(e.Length)
		default:
			oi, li = 0, 0
		}
		switch e := entities[j].(type) {
		case *tg.MessageEntityBold:
			oj, lj = int(e.Offset), int(e.Length)
		case *tg.MessageEntityItalic:
			oj, lj = int(e.Offset), int(e.Length)
		case *tg.MessageEntityUnderline:
			oj, lj = int(e.Offset), int(e.Length)
		case *tg.MessageEntityStrike:
			oj, lj = int(e.Offset), int(e.Length)
		case *tg.MessageEntityBlockquote:
			oj, lj = int(e.Offset), int(e.Length)
		case *tg.MessageEntityCode:
			oj, lj = int(e.Offset), int(e.Length)
		case *tg.MessageEntityPre:
			oj, lj = int(e.Offset), int(e.Length)
		case *tg.MessageEntitySpoiler:
			oj, lj = int(e.Offset), int(e.Length)
		case *tg.MessageEntityTextURL:
			oj, lj = int(e.Offset), int(e.Length)
		case *tg.MessageEntityCustomEmoji:
			oj, lj = int(e.Offset), int(e.Length)
		case *tg.MessageEntityMention:
			oj, lj = int(e.Offset), int(e.Length)
		default:
			oj, lj = 0, 0
		}
		if oi == oj {
			return li > lj
		}
		return oi < oj
	})

	// Build insertion list recursively (like Pyrogram)
	type insertion struct {
		tag string
		off int
	}
	var entitiesOffsets []insertion

	var recursive func(int) int
	recursive = func(i int) int {
		if i >= len(entities) {
			return 1
		}
		startPair, endPair := parseOne(entities[i])
		if startPair == nil || endPair == nil {
			return 1
		}
		entitiesOffsets = append(entitiesOffsets, insertion{tag: startPair.tag, off: startPair.off})
		internal := i + 1
		endOff := endPair.off
		for internal < len(entities) {
			var nextOff int
			switch e := entities[internal].(type) {
			case *tg.MessageEntityBold:
				nextOff = int(e.Offset)
			case *tg.MessageEntityItalic:
				nextOff = int(e.Offset)
			case *tg.MessageEntityUnderline:
				nextOff = int(e.Offset)
			case *tg.MessageEntityStrike:
				nextOff = int(e.Offset)
			case *tg.MessageEntityBlockquote:
				nextOff = int(e.Offset)
			case *tg.MessageEntityCode:
				nextOff = int(e.Offset)
			case *tg.MessageEntityPre:
				nextOff = int(e.Offset)
			case *tg.MessageEntitySpoiler:
				nextOff = int(e.Offset)
			case *tg.MessageEntityTextURL:
				nextOff = int(e.Offset)
			case *tg.MessageEntityCustomEmoji:
				nextOff = int(e.Offset)
			case *tg.MessageEntityMention:
				nextOff = int(e.Offset)
			default:
				nextOff = 1 << 30
			}
			if nextOff < endOff {
				internal += recursive(internal)
			} else {
				break
			}
		}
		entitiesOffsets = append(entitiesOffsets, insertion{tag: endPair.tag, off: endPair.off})
		return internal - i
	}

	i := 0
	for i < len(entities) {
		i += recursive(i)
	}

	if len(entitiesOffsets) == 0 {
		return text, nil
	}

	// apply insertions descending by offset
	sort.SliceStable(entitiesOffsets, func(i, j int) bool { return entitiesOffsets[i].off > entitiesOffsets[j].off })

	outRunes := []rune(sur)
	for _, it := range entitiesOffsets {
		off := it.off
		if off < 0 {
			off = 0
		}
		if off > len(outRunes) {
			off = len(outRunes)
		}
		prefix := string(outRunes[:off])
		suffix := string(outRunes[off:])
		out := prefix + it.tag + suffix
		outRunes = []rune(out)
	}

	final := removeSurrogates(string(outRunes))
	return final, nil
}
