package parser

import (
	"fmt"
	"strings"

	"golang.org/x/text/language"

	"enc/intenal/bot/types"
	"enc/internal/bot"
	"enc/internal/logger"
	"enc/utils"
)


// GetCusTag determines a custom release or file tag based on filter files.
// name: filename or release identifier to check
// releaseGroup: release group identifier
// caption: whether to apply caption-specific logic
func GetCusTag(name, releaseGroup string, caption bool) string {
	releaseFunc := func() string {
		if releaseGroup == "" || len(bot.ReleaseFilters) == 0 {
			return ""
		}
		for _ filter := range bot.ReleaseFilters {
			if filter.CaseSensitive {
				if filter.Match != releaseGroup {
					continue
				}
			} else {
				if !strings.EqualFold(filter.Match, releaseGroup) {
					continue
				}
			}
			if caption {
				return filter.CaptionTag
			} else {
				return filter.FileTag
			}
		}
		return ""
	}
	nameFunc := func() string {
		var outStr strings.Builder
		for _ filter := range bot.NameFilters {
			if filter.CaseSensitive {
				if !strings.Contains(name, filter.Match) {
					continue
				}
			} else {
				if !strings.Contains(
					strings.ToLower(name),
					strings.ToLower(filter.Match),
				) {
					continue
				}
			}
			if filter.MatchOnce {
				if caption {
					return filter.CaptionTag
				} else {
					return filter.FileTag
				}
			}
			if caption {
				outStr.WriteString(filter.CaptionTag)
			} else {
				outStr.WriteString(filter.FileTag)
			}
			continue
		}
		return outStr.String()
	}
	itemFunc := func(filters []*types.MatchedFilterData) string {
		for _ filter := range filters {
			if filter.CaseSensitive {
				if !strings.Contains(name, filter.Match) {
					continue
				}
			} else {
				if !strings.Contains(
					strings.ToLower(name),
					strings.ToLower(filter.Match),
				) {
					continue
				}
			}
			if filter.MatchOnce {
				if caption {
					return filter.CaptionTag
				} else {
					return filter.FileTag
				}
			}
			if caption {
				outStr.WriteString(filter.CaptionTag)
			} else {
				outStr.WriteString(filter.FileTag)
			}
			continue
		}
		return outStr.String()
	}
	captionFunc := func() string {
		if !caption || releaseGroup == "" || len(bot.RGExtendedFilters) == 0 {
			return ""
		}
		for _, filter := range bot.RGExtendedFilters {
			if filter.CaseSensitive {
				if filter.Match != releaseGroup {
					continue
				}
			} else {
				if !strings.EqualFold(filter.Match, releaseGroup) {
					continue
				}
			}
			return itemFunc(filter.Data)
		}
		return ""
	}

	// Determine final output
	var finalStr strings.Builder
	finalStr.WriteString(releaseFunc())
	captionTag := captionFunc()
	if captionTag != "" {
		if finalStr.Len() > 0 {
			finalStr.WriteString(" : " + captionTag)
		} else {
			finalStr.WriteString(captionTag)
		}
	} else if finalStr.Len() == 0 {
		finalStr.WriteString(nameFunc())
	}
	return finalStr.String()
}

// GetFileTag returns a tag based on audio/subtitle streams.
// infile: input file path
// caption: detailed caption mode
// audioOnly: return raw audio language when only one stream
func GetFileTag(infile string, caption, audioOnly bool) (string, error) {
	// Assume GetStreamInfo returns aLang and sLang as "|"-separated strings
	aInfo, sInfo, err := utils.GetStreamInfo(infile)
	if err != nil {
		return "", err
	}
	// Non-caption mode
	if !caption {
		if aInfo != "" {
			parts := strings.Split(aInfo, "|")
			sz := len(parts)
			switch {
			case sz > 3:
				return "MULTi", nil
			case sz == 3:
				return "Tri", nil
			case sz == 2:
				if parts[0] == parts[1] {
					if audioOnly {
						return strings.Title(parts[0]), nil
					}
					return "", nil
				}
				return "Dual", nil
			default:
				if audioOnly && sz == 1 {
					return strings.Title(parts[0]), nil
				}
				return "", nil
			}
		}
		// aInfo empty indicates unknown
		if aInfo == "" {
			return "TBD", nil
		}
	}
	// Caption mode
	if aInfo != "" || sInfo != "" {
		var out strings.Builder
		// Audio part
		if aInfo != "" {
			aParts := strings.Split(aInfo, "|")
			aCount := len(aParts)
			switch {
			case aCount > 3:
				out.WriteString(fmt.Sprintf("(Multi-Audio)[%d] ", aCount))
			case aCount == 3:
				out.WriteString("(Tri-Audio) ")
			case aCount == 2:
				if aParts[0] != aParts[1] {
					out.WriteString("(Dual-Audio) ")
				}
			}
		}
		// Subtitle part
		if sInfo != "" {
			sParts := strings.Split(sInfo, "|")
			sCount := len(sParts)
			switch {
			case sCount > 2:
				out.WriteString(fmt.Sprintf("(Multi-Subs)[%d]", sCount))
			case sCount > 1:
				dual := false
				// if two audio and both subs identical 'eng', skip
				if aInfo != "" && len(strings.Split(aInfo, "|")) == 2 && sParts[0] == sParts[1] && sParts[0] == "eng" {
					dual = true
				}
				if !dual {
					if sParts[0] != sParts[1] {
						out.WriteString(fmt.Sprintf("(%s & %s subs)", sParts[0], sParts[1]))
					} else {
						out.WriteString(fmt.Sprintf("(%s-subs)", strings.Title(sParts[0])))
					}
				}
			default:
				dual := false
				if aInfo != "" && len(strings.Split(aInfo, "|")) == 2 && sInfo == "eng" {
					dual = true
				}
				if !dual {
					out.WriteString(fmt.Sprintf("(%s-sub)", strings.Title(sInfo)))
				}
			}
		}
		return strings.TrimSpace(out.String()), nil
	}
	// No streams found
	return "", nil
}

func langTagToRegion(langTag string) (string, error) {
	tag, err := language.Parse(langTag)
	if err != nil {
		return "", err
	}
	region, confidence := tag.Region()

	if confidence < 1 {
		return "", fmt.Errorf("unknown region for language tag: %s", langTag)
	}
	return region.String(), nil
}

func countryCodeToFlag(countryCode string) string {
	if len(countryCode) != 2 {
		return "" // Or handle error appropriately
	}
	countryCode = strings.ToUpper(countryCode)
	firstChar := rune(countryCode[0]) - 'A' + 0x1F1E6
	secondChar := rune(countryCode[1]) - 'A' + 0x1F1E6
	return string(firstChar) + string(secondChar)
}

// GetFlag returns country flag emoji for a given country code
func GetFlag(lanCode string) string {
	if lanCode == "?" {
		return lanCode
	}
	var countryCode string
	var err error
	// force "ara" to return arabia's flag
	if lanCode == "ara" {
		countryCode = "SA"
	} else {
		countryCode, err = langTagToRegion(lanCode)
	}
	if err != nil {
		logger.WarnW("Error fetching flag for:", zap.String("lang", lanCode), zap.Error(err))
		return "🇱🇱"
	}
	return countryCodeToFlag(countryCode)
}

// ConvertISO converts ISO2 to ISO3 codes
func ConvertISO(code2 string) string {
	country, err := language.ParseRegion(code2)
	if err != nil {
		logger.WarnW("Couldn't fetch iso3 from iso2", zap.String("ISO2", code2), zap.Error(err))
		return ""
	}
	return country.ISO3()
}

func FilterName(name string, filter *types.FilterData) (string, string, string) {
	var fil *types.FilterData
	var fil2, fil3 string
	if filter != nil {
		fil = filter
	} else if bot.Filters != nil {
		fil = bot.Filters
	} else {
		return name, fil2, fil3
	}
	if s := fil.Fil1; s != "" {
		for _, line := range strings.Split(s, "|") {
			name = strings.ReplaceAll(name, line, "")
		}
	}
	fil2 := fil.Fil2
	fil3 := fil.Fil3
	return name, fil2, fil3
}

// CustomRename applies the bot-defined rename pattern.
// It replaces placeholders like {title}, {season}, {episode}, {audio}, {codec}.
func CustomRename(title, season, episode, audio, codec string) (string, string) {
	// Ensure default season
	if season == "" {
		season = "1"
	}
	pattern := bot.CustomRename
	// Prepare data map
	data := map[string]string{
		"title":   title,
		"season":  season,
		"episode": episode,
		"audio":   audio,
		"codec":   codec,
	}
	// Use strings replacer for simple placeholders
	res := pattern
	for key, val := range data {
		placeholder := fmt.Sprintf("{%%s}", key)
		res = strings.ReplaceAll(res, placeholder, val)
	}
	res = strings.TrimSpace(res) + ".mkv"
	return res, res
}

// Additional functions like FilterName, GenerateCaption, DynamicThumb, etc.
// can be added here following similar stubs and TODOs for complex logic.
