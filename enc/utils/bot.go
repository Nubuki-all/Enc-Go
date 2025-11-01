package utils

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"enc/internal/bot"
	"enc/internal/bot/state"
	"enc/internal/bot/types"
)

// BotIsPaused returns true if any pause flag is set
func BotIsPaused() bool {
	return bot.Paused.Any()
}

// EnCanceller returns the encode cancellation map
func EnCanceller() map[string]any {
	return bot.ECancel
}

func IndexToPauseType(i int) (state.PauseType, bool) {
	pt := state.PauseType(i)
	switch pt {
	case state.DefaultPause, state.DLPause, state.RSSPause:
		return pt, true
	}
	return 0, false
}

// PauseByType sets or clears a specific pause type
func PauseByType(pt state.PauseType, on bool) {
	bot.Paused.Set(pt, on)
}

func Pause(idx int, on bool) error {
	pt, ok := IndexToPauseType(idx)
	if !ok {
		return fmt.Errorf("invalid pause index %d", idx)
	}
	PauseByType(pt, on)
	return nil
}

// GetPauseStatusByType whether a specific pause type is set
func GetPauseStatusByType(pt state.PauseType) bool {
	return bot.Paused.Get(pt)
}

func GetPauseStatus(idx int) (bool, error) {
	pt, ok := IndexToPauseType(idx)
	if !ok {
		return false, fmt.Errorf("invalid pause index %d", idx)
	}
	return GetPauseStatusByType(pt), nil
}

// RmPauseByType clears the given pause type after duration if still set
func RmPauseByType(pt state.PauseType, duration time.Duration) {
	if !bot.Paused.Get(pt) {
		return
	}
	go func() {
		time.Sleep(duration)
		// only clear if still paused
		if bot.Paused.Get(pt) {
			bot.Paused.Set(pt, false)
		}
	}()
}

func RmPause(idx int, duration time.Duration) error {
	pt, ok := IndexToPauseType(idx)
	if !ok {
		return fmt.Errorf("invalid pause index %d", idx)
	}
	RmPauseByType(pt, duration)
	return nil
}

// GetBatchQueue returns the current batch queue
func GetBatchQueue() map[string]any {
	return bot.BatchQueue
}

// GetQueue returns the main queue
func GetQueue() *types.Queue {
	return bot.Queue
}

// GetVar returns internal variable by name
func GetVar(name string) any {
	switch nameLower := strings.ToLower(name); nameLower {
	case "groupenc":
		return bot.GroupEnc
	case "version2":
		return bot.ReleaseVersion
	case "paused":
		return bot.Paused
	default:
		return nil
	}
}

// GetVersion returns the global release version
func GetVersion() string {
	return bot.ReleaseVersion
}

// SplitQuotedFields splits s into whitespace-separated tokens while
// preserving quoted substrings (single ' or double "). Quotes are
// removed. Backslash escaping is supported inside quotes.
// Unclosed quotes are accepted (token continues to end).
func SplitQuotedFields(s string) []string {
	var out []string
	var b strings.Builder
	inQuote := false
	var quoteChar rune
	esc := false

	for _, r := range s {
		if esc {
			b.WriteRune(r)
			esc = false
			continue
		}

		if inQuote {
			switch r {
			case '\\':
				esc = true
			default:
				if r == quoteChar {
					// end quote, don't write the quote char
					inQuote = false
					quoteChar = 0
				} else {
					b.WriteRune(r)
				}
			}
			continue
		}

		// not in quote
		if r == '"' || r == '\'' {
			inQuote = true
			quoteChar = r
			continue
		}

		if unicode.IsSpace(r) {
			if b.Len() > 0 {
				out = append(out, b.String())
				b.Reset()
			}
			continue
		}

		b.WriteRune(r)
	}

	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

// SplitQuotedFieldsErr is like SplitQuotedFields but returns an error
// if a quoted section was not closed.
func SplitQuotedFieldsErr(s string) ([]string, error) {
	var out []string
	var b strings.Builder
	inQuote := false
	var quoteChar rune
	esc := false

	for _, r := range s {
		if esc {
			b.WriteRune(r)
			esc = false
			continue
		}

		if inQuote {
			switch r {
			case '\\':
				esc = true
			default:
				if r == quoteChar {
					inQuote = false
					quoteChar = 0
				} else {
					b.WriteRune(r)
				}
			}
			continue
		}

		// not in quote
		if r == '"' || r == '\'' {
			inQuote = true
			quoteChar = r
			continue
		}

		if unicode.IsSpace(r) {
			if b.Len() > 0 {
				out = append(out, b.String())
				b.Reset()
			}
			continue
		}

		b.WriteRune(r)
	}

	if inQuote {
		return nil, fmt.Errorf("unclosed quote")
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out, nil
}

func RemoveRFilterByIndex(s []*types.RenameFilterData, index int) []*types.RenameFilterData {
	ret := make([]*types.RenameFilterData, 0, len(s)-1)
	ret = append(ret, s[:index]...)
	return append(ret, s[index+1:]...)
}

// IsPositiveNumber returns true if the string is a valid number greater than zero.
func IsPositiveNumber(s string) bool {
	f, err := strconv.ParseInt(s, 10, 64)
	return err == nil && f > 0
}

// IsNumber returns true if the string is a valid number.
func IsNumber(s string) bool {
	_, err := strconv.ParseInt(s, 10, 64)
	return err == nil
}

// IsNegativeNumber returns true if the string is a valid number lesser than zero.
func IsNegativeNumber(s string) bool {
	f, err := strconv.ParseInt(s, 10, 64)
	return err == nil && f < 0
}
