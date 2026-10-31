package utils

import (
	"bytes"
	"html"
	"path/filepath"
	"strings"
)

// KV preserves insertion order for a key/value pair.
type KV struct {
	Key string
	Val string
}

// Track holds an ordered list of KV pairs for a single track.
type Track struct {
	Pairs []KV
}

// ParseMediaInfoTextToHTML parses MediaInfo's default text output and returns HTML
// formatted according to your rules. Returns an empty string if input is empty.
func ParseMediaInfoTextToHTML(input string) string {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}

	lines := strings.Split(input, "\n")

	// Map section => slice of tracks (each track preserves insertion order)
	typeTracks := map[string][]Track{}
	order := []string{} // keep section order of first appearance

	currentSection := ""

	// Pointers to optimize append operations.
	var currentTrack *Track

	for _, raw := range lines {
		line := raw
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			// A blank line can serve as a separator between tracks, but only
			// if we have finished parsing the previous track's data.
			// This logic is implicitly handled by starting a new track only on
			// section headers, which correctly separates tracks in MediaInfo output.
			continue
		}

		// If there's no ":" at all, treat it as a section header.
		if !strings.Contains(line, ":") {
			currentSection = strings.TrimSpace(line)
			// Check if this is the first time we've seen this section.
			if _, ok := typeTracks[currentSection]; !ok {
				order = append(order, currentSection)
			}

			// Always start a new track for a new section header.
			newTrack := Track{Pairs: []KV{}}
			typeTracks[currentSection] = append(typeTracks[currentSection], newTrack)
			currentTrack = &typeTracks[currentSection][len(typeTracks[currentSection])-1]
			continue
		}

		// If we haven't seen a section yet, assign it to a "General" section.
		if currentSection == "" {
			currentSection = "General"
			// Ensure "General" is in the order.
			if _, ok := typeTracks[currentSection]; !ok {
				order = append(order, currentSection)
			}
			// Create the first track for the "General" section.
			newTrack := Track{Pairs: []KV{}}
			typeTracks[currentSection] = append(typeTracks[currentSection], newTrack)
			currentTrack = &typeTracks[currentSection][0]
		}

		// Choose splitting strategy:
		var key, val string
		if strings.EqualFold(strings.TrimSpace(currentSection), "Menu") {
			// Split on LAST ":" for Menu sections.
			lastColon := strings.LastIndex(line, ":")
			if lastColon == -1 {
				key, val = strings.TrimSpace(line), ""
			} else {
				key, val = strings.TrimSpace(line[:lastColon]), strings.TrimSpace(line[lastColon+1:])
			}
		} else {
			// Split on FIRST ":" for all other sections.
			firstColon := strings.Index(line, ":")
			if firstColon == -1 {
				key, val = strings.TrimSpace(line), ""
			} else {
				key, val = strings.TrimSpace(line[:firstColon]), strings.TrimSpace(line[firstColon+1:])
			}
		}
		if strings.EqualFold(currentSection, "General") && strings.EqualFold(key, "Complete name") {
			val = filepath.Base(val)
		}

		// Append KV to the current track.
		// If currentTrack is nil (which should not happen with the logic above, but is a good safeguard),
		// it means we encountered a key-value pair without a preceding section header.
		// In a valid MediaInfo text, the first section is "General".
		if currentTrack != nil {
			currentTrack.Pairs = append(currentTrack.Pairs, KV{Key: key, Val: val})
		}
	}

	// Render HTML
	var buf bytes.Buffer
	for _, section := range order {
		tracks := typeTracks[section]

		buf.WriteString("<h4>")
		buf.WriteString(html.EscapeString(section))
		buf.WriteString("</h4>")

		for _, t := range tracks {
			for _, kv := range t.Pairs {
				if strings.TrimSpace(kv.Key) == "" && strings.TrimSpace(kv.Val) == "" {
					continue
				}
				buf.WriteString("<p><i>")
				buf.WriteString(html.EscapeString(kv.Key))
				buf.WriteString(" :</i> ")
				buf.WriteString(html.EscapeString(kv.Val))
				buf.WriteString("</p>")
			}
		}
		if buf.Len() > maxHTMLSize {
			output := buf.String()
			// keep ~65430 bytes plus truncation notice (matches original).
			cut := 65430
			cut = min(cut, len(output))
			return output[:cut] + "<strong>...<strong><br><br><strong>(TRUNCATED DUE TO CONTENT EXCEEDING MAX LENGTH)<strong>"
		}
	}

	return buf.String()
}
