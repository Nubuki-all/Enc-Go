package parser

import (
	"fmt"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/gotd/td/tg"
)

// NormalizeEntities validates and repairs entity offsets/lengths to UTF-16 code units.
// It will try heuristics (bytes -> utf16, runes -> utf16) and finally clamp lengths
// so we don't send ENTITY_BOUNDS_INVALID to Telegram.
func NormalizeEntities(msg string, ents []tg.MessageEntityClass) (string, []tg.MessageEntityClass) {
	if len(ents) == 0 {
		return ents
	}

	u16len := utf16Len(msg)
	out := make([]tg.MessageEntityClass, 0, len(ents))

	for i, e := range ents {
		off, ln := getOffLen(e)
		// basic sanity
		if off < 0 || ln <= 0 {
			// drop obviously invalid
			continue
		}
		if off+ln <= u16len {
			out = append(out, e)
			continue
		}

		// Try heuristic: treat (off,ln) as byte positions in msg
		if int(off+ln) <= len(msg) {
			newOff := bytePosToUTF16Units(msg, int(off))
			newEnd := bytePosToUTF16Units(msg, int(off+ln))
			newLen := newEnd - newOff
			if newOff >= 0 && newLen > 0 && newOff+newLen <= u16len {
				if ne := cloneWithNewOffLen(e, newOff, newLen); ne != nil {
					out = append(out, ne)
					continue
				}
			}
		}

		// Try heuristic: treat (off,ln) as rune indices
		if int(off+ln) <= utf8.RuneCountInString(msg) {
			newOff := runeIndexToUTF16Units(msg, int(off))
			newEnd := runeIndexToUTF16Units(msg, int(off+ln))
			newLen := newEnd - newOff
			if newOff >= 0 && newLen > 0 && newOff+newLen <= u16len {
				if ne := cloneWithNewOffLen(e, newOff, newLen); ne != nil {
					out = append(out, ne)
					continue
				}
			}
		}

		// Last resort: clamp length so entity fits (if offset within bounds).
		if off < u16len {
			newLen := u16len - off
			if newLen > 0 {
				if ne := cloneWithNewOffLen(e, off, newLen); ne != nil {
					out = append(out, ne)
					continue
				}
			}
		}

		// Could not salvage — drop and log for debugging.
		fmt.Printf("[NormalizeEntities] dropping entity #%d (type=%T) off=%d len=%d msgU16=%d\n", i, e, off, ln, u16len)
	}

	return msg, out
}

// ==== Helpers ====

func getOffLen(e tg.MessageEntityClass) (int, int) {
	switch v := e.(type) {
	case *tg.MessageEntityBold:
		return int(v.Offset), int(v.Length)
	case *tg.MessageEntityItalic:
		return int(v.Offset), int(v.Length)
	case *tg.MessageEntityUnderline:
		return int(v.Offset), int(v.Length)
	case *tg.MessageEntityStrike:
		return int(v.Offset), int(v.Length)
	case *tg.MessageEntityCode:
		return int(v.Offset), int(v.Length)
	case *tg.MessageEntityPre:
		return int(v.Offset), int(v.Length)
	case *tg.MessageEntityTextURL:
		return int(v.Offset), int(v.Length)
	case *tg.MessageEntityMentionName:
		return int(v.Offset), int(v.Length)
	case *tg.MessageEntityBlockquote:
		return int(v.Offset), int(v.Length)
	case *tg.MessageEntitySpoiler:
		return int(v.Offset), int(v.Length)
	case *tg.MessageEntityCustomEmoji:
		return int(v.Offset), int(v.Length)
	default:
		return -1, -1
	}
}

func cloneWithNewOffLen(e tg.MessageEntityClass, off, ln int) tg.MessageEntityClass {
	switch v := e.(type) {
	case *tg.MessageEntityBold:
		return &tg.MessageEntityBold{Offset: off, Length: ln}
	case *tg.MessageEntityItalic:
		return &tg.MessageEntityItalic{Offset: off, Length: ln}
	case *tg.MessageEntityUnderline:
		return &tg.MessageEntityUnderline{Offset: off, Length: ln}
	case *tg.MessageEntityStrike:
		return &tg.MessageEntityStrike{Offset: off, Length: ln}
	case *tg.MessageEntityCode:
		return &tg.MessageEntityCode{Offset: off, Length: ln}
	case *tg.MessageEntityPre:
		lang = v.Language
		return &tg.MessageEntityPre{Offset: off, Length: ln, Language: lang}
	case *tg.MessageEntityTextURL:
		if v.URL != "" {
			return &tg.MessageEntityTextURL{Offset: off, Length: ln, URL: v.URL}
		}
		return &tg.MessageEntityTextURL{Offset: off, Length: ln}
	case *tg.MessageEntityMentionName:
		return &tg.MessageEntityMentionName{Offset: off, Length: ln, UserID: v.UserID}
	case *tg.MessageEntityBlockquote:
		return &tg.MessageEntityBlockquote{Offset: off, Length: ln}
	case *tg.MessageEntitySpoiler:
		return &tg.MessageEntitySpoiler{Offset: off, Length: ln}
	case *tg.MessageEntityCustomEmoji:
		if v.DocumentID != 0 {
			return &tg.MessageEntityCustomEmoji{Offset: off, Length: ln, DocumentID: v.DocumentID}
		}
		return &tg.MessageEntityCustomEmoji{Offset: off, Length: ln}
	default:
		return nil
	}
}

// utf16Len returns number of UTF-16 code units in s (Telegram's unit).
func utf16Len(s string) int {
	rs := []rune(s)
	return len(utf16.Encode(rs))
}

// bytePosToUTF16Units converts a byte-position into count of UTF-16 units up to that byte pos.
func bytePosToUTF16Units(s string, bytePos int) int {
	if bytePos <= 0 {
		return 0
	}
	if bytePos >= len(s) {
		return utf16Len(s)
	}
	prefix := s[:bytePos]
	return utf16Len(prefix)
}

// runeIndexToUTF16Units converts rune index (0-based) to UTF-16 units for prefix of length runeIndex.
func runeIndexToUTF16Units(s string, runeIndex int) int {
	if runeIndex <= 0 {
		return 0
	}
	r := []rune(s)
	if runeIndex >= len(r) {
		return utf16Len(s)
	}
	return utf16Len(string(r[:runeIndex]))
}
