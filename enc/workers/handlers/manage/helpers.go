package manage

import (
	"strings"

	"enc/internal/bot"
	"enc/internal/bot/types"
)

func RenameFilterExists(name string, caseSensitive bool) bool {
	for _, filter := range bot.RenameFilters {
		if strings.EqualFold(filter.Match, name) {
			if !(caseSensitive || filter.CaseSensitive) {
				return true
			}
			if filter.Match == name {
				return true
			}
		}
	}
	return false
}

func updateMatchFilter(filters []*types.MatchedFilterData, release bool) {
	if release {
		bot.ReleaseFilters = filters
	} else {
		bot.NameFilters = filters
	}
}

func MatchFilterExists(name string, filters []*types.MatchedFilterData) bool {
	for _, filter := range filters {
		if filter.Match == name {
			return true
		}
	}
	return false
}

func ReleaseFilterExists(name string, caseSensitive bool) bool {
	for _, filter := range bot.ReleaseFilters {
		if strings.EqualFold(filter.Match, name) {
			if !(caseSensitive || filter.CaseSensitive) {
				return true
			}
			if filter.Match == name {
				return true
			}
		}
	}
	return false
}

func convertStrToMtchFiltr(datas []string) []*types.MatchedFilterData {
	out := []*types.MatchedFilterData{}
	for _, data := range datas {
		values := strings.Split(data, "||")
		valueLen := len(values)
		if valueLen < 2 {
			continue
		}
		fil := &types.MatchedFilterData{
			Match:      values[0],
			CaptionTag: values[1],
		}
		if valueLen > 2 {
			fil.CaseSensitive = values[2]
		}
		if valueLen > 3 {
			fil.MatchOnce = values[3]
		}
		out = append(out, fil)

	}
	return out
}

func ReleaseExtFilterExists(name string, caseSensitive bool) bool {
	for _, filter := range bot.RGExtendedFilters {
		if strings.EqualFold(filter.Match, name) {
			if !(caseSensitive || filter.CaseSensitive) {
				return true
			}
			if filter.Match == name {
				return true
			}
		}
	}
	return false
}
