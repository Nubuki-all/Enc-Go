package utils

import (
	"bytes"
	"html"
	"strings"
	//"fmt"
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

	ensureSection := func(section string) {
		if _, ok := typeTracks[section]; !ok {
			typeTracks[section] = nil
			order = append(order, section)
		}
		// If no track exists yet for this section, create the first one
		if len(typeTracks[section]) == 0 {
			typeTracks[section] = append(typeTracks[section], Track{Pairs: []KV{}})
		}
	}

	currentSection := ""
	for _, raw := range lines {
		line := raw
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			// blank line = separator
			continue
		}

		// If there's no ":" at all treat as a section header
		if !strings.Contains(line, ":") {
			currentSection = strings.TrimSpace(line)
			ensureSection(currentSection)
			// start a new track under this header (so repeated header => new track)
			typeTracks[currentSection] = append(typeTracks[currentSection], Track{Pairs: []KV{}})
			continue
		}

		// Choose splitting strategy:
		// - For Menu: split on LAST ":" (so timecodes at start become the key)
		// - For other sections: split on FIRST ":" (so keys like "Duration" remain)
		var key, val string
		if strings.EqualFold(strings.TrimSpace(currentSection), "Menu") {
			// last colon
			lastColon := strings.LastIndex(line, ":")
			if lastColon == -1 {
				// fallback - treat whole line as key
				key = strings.TrimSpace(line)
				val = ""
			} else {
				key = strings.TrimSpace(line[:lastColon])
				val = strings.TrimSpace(line[lastColon+1:])
			}
		} else {
			// first colon
			firstColon := strings.Index(line, ":")
			if firstColon == -1 {
				// fallback - treat whole line as key
				key = strings.TrimSpace(line)
				val = ""
			} else {
				key = strings.TrimSpace(line[:firstColon])
				val = strings.TrimSpace(line[firstColon+1:])
			}
		}

		// If we haven't seen a section yet, treat as General
		if currentSection == "" {
			currentSection = "General"
			ensureSection(currentSection)
		} else {
			ensureSection(currentSection)
		}

		// append KV to the last track of current section
		lastIdx := len(typeTracks[currentSection]) - 1
		if lastIdx < 0 {
			typeTracks[currentSection] = append(typeTracks[currentSection], Track{Pairs: []KV{}})
			lastIdx = len(typeTracks[currentSection]) - 1
		}
		typeTracks[currentSection][lastIdx].Pairs = append(typeTracks[currentSection][lastIdx].Pairs, KV{Key: key, Val: val})
	}

	// Render HTML
	var buf bytes.Buffer
	important := map[string]bool{
		"General": true,
		"Menu":    true,
		"Video":   true,
		"Audio":   true,
		"Text":    true,
	}

	for _, section := range order {
		tracks := typeTracks[section]

		// Section heading larger: use <h2> for important sections
		if important[section] {
			buf.WriteString("<h3 style=\"margin:8px 0\">")
			buf.WriteString(html.EscapeString(section))
			buf.WriteString("</h3>\n")
		} else {
			buf.WriteString("<h4 style=\"margin:6px 0\">")
			buf.WriteString(html.EscapeString(section))
			buf.WriteString("</h34>\n")
		}
		buf.WriteString("\n<hr>\n")

		if section == "Video" || section == "Audio" || section == "Text" {
			for _, t := range tracks {
				/*label := section
				if len(tracks) > 1 {
					label = fmt.Sprintf("%s #%d", section, i+1)
				}
				buf.WriteString("<div>{")
				buf.WriteString(html.EscapeString(label))
				buf.WriteString("}</div>\n")*/

				// write key/values in original order; add spacing after each entry
				for _, kv := range t.Pairs {
					if strings.TrimSpace(kv.Key) == "" && strings.TrimSpace(kv.Val) == "" {
						continue
					}
					buf.WriteString("<div style=\"margin-bottom:8px\"><em>")
					buf.WriteString(html.EscapeString(kv.Key))
					// user requested "key : value" spacing
					buf.WriteString("</em> : ")
					buf.WriteString(html.EscapeString(kv.Val))
					buf.WriteString("</div>\n")
					buf.WriteString("<hr>\n")
				}
			}
			continue
		}

		// For Menu: do NOT print {Menu} labels; just render key : value lines
		if strings.EqualFold(strings.TrimSpace(section), "Menu") {
			for _, t := range tracks {
				for _, kv := range t.Pairs {
					if strings.TrimSpace(kv.Key) == "" && strings.TrimSpace(kv.Val) == "" {
						continue
					}
					buf.WriteString("<div style=\"margin-bottom:8px\"><em>")
					buf.WriteString(html.EscapeString(kv.Key))
					buf.WriteString("</em> : ")
					buf.WriteString(html.EscapeString(kv.Val))
					buf.WriteString("</div>\n")
					buf.WriteString("<hr>\n")
				}
			}
			continue
		}

		// For other sections: if multiple tracks, label them (but skip labeling Menu)
		for _, kv := range tracks[0].Pairs {
			if strings.TrimSpace(kv.Key) == "" && strings.TrimSpace(kv.Val) == "" {
				continue
			}
			buf.WriteString("<div style=\"margin-bottom:8px\"><em>")
			buf.WriteString(html.EscapeString(kv.Key))
			buf.WriteString("</em> : ")
			buf.WriteString(html.EscapeString(kv.Val))
			buf.WriteString("</div>\n")
			buf.WriteString("<hr>\n")
		}	
	}

	return buf.String()
}

