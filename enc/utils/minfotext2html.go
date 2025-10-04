package utils

import (
	"bytes"
	"fmt"
	"html"
	"sort"
	"strings"
)

// ParseMediaInfoTextToHTML parses MediaInfo's default text output and returns HTML
// formatted according to your rules. Returns an empty string if input is empty.
func ParseMediaInfoTextToHTML(input string) string {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}

	lines := strings.Split(input, "\n")

	// Map section => slice of tracks (each track is map[key]value)
	typeTracks := map[string][]map[string]string{}
	order := []string{} // keep section order of first appearance

	// helper to ensure we have a current track map for the current section
	ensureCurrent := func(section string) {
		if _, ok := typeTracks[section]; !ok {
			typeTracks[section] = nil
			order = append(order, section)
		}
		if len(typeTracks[section]) == 0 {
			typeTracks[section] = append(typeTracks[section], map[string]string{})
		}
	}

	currentSection := ""
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			// blank line - treat as separator, but keep the currentSection
			continue
		}

		// If the line does not contain ":" assume it's a section header
		if !strings.Contains(line, ":") {
			// new section header
			currentSection = line
			ensureCurrent(currentSection)
			continue
		}

		// otherwise treat as key:value (split on first ":" only)
		parts := strings.SplitN(line, ":", 2)
		key := strings.TrimSpace(parts[0])
		val := ""
		if len(parts) > 1 {
			val = strings.TrimSpace(parts[1])
		}

		// if we haven't seen a section yet, put into "General"
		if currentSection == "" {
			currentSection = "General"
			ensureCurrent(currentSection)
		} else {
			ensureCurrent(currentSection)
		}

		// put into the last track map for the current section
		lastIdx := len(typeTracks[currentSection]) - 1
		if lastIdx < 0 {
			typeTracks[currentSection] = append(typeTracks[currentSection], map[string]string{})
			lastIdx = 0
		}

		// if key already present, append with separator
		if prev, ok := typeTracks[currentSection][lastIdx][key]; ok && prev != "" {
			typeTracks[currentSection][lastIdx][key] = prev + "; " + val
		} else {
			typeTracks[currentSection][lastIdx][key] = val
		}
	}

	// Now render HTML according to rules
	var buf bytes.Buffer
	/*important := map[string]bool{
		"General": true,
		"Menu":    true,
		"Video":   true,
		"Audio":   true,
		"Text":    true,
	}*/

	for _, section := range order {
		tracks := typeTracks[section]

		// Section heading (bold)
		buf.WriteString("<div><strong>")
		buf.WriteString(html.EscapeString(section))
		buf.WriteString("</strong></div>\n")

		// If it's Video/Audio/Text, print each track as {Type} or {Type} #n
		if section == "Video" || section == "Audio" || section == "Text" {
			for i, t := range tracks {
				label := section
				if len(tracks) > 1 {
					label = fmt.Sprintf("%s #%d", section, i+1)
				}
				buf.WriteString("<div>{")
				buf.WriteString(html.EscapeString(label))
				buf.WriteString("}</div>\n")
				writeKeyValues(&buf, t)
			}
			continue
		}

		// For other sections: if multiple tracks, still label them
		if len(tracks) == 1 {
			writeKeyValues(&buf, tracks[0])
		} else {
			for i, t := range tracks {
				label := section
				if len(tracks) > 1 {
					label = fmt.Sprintf("%s #%d", section, i+1)
					buf.WriteString("<div>{")
					buf.WriteString(html.EscapeString(label))
					buf.WriteString("}</div>\n")
				}
				writeKeyValues(&buf, t)
			}
		}
	}

	return buf.String()
}

// writeKeyValues writes keys in sorted order as: <em>Key</em>: Value
func writeKeyValues(buf *bytes.Buffer, m map[string]string) {
	if len(m) == 0 {
		return
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := m[k]
		if strings.TrimSpace(v) == "" {
			continue
		}
		buf.WriteString("<div><em>")
		buf.WriteString(html.EscapeString(k))
		buf.WriteString("</em>: ")
		buf.WriteString(html.EscapeString(v))
		buf.WriteString("</div>\n")
	}
}
