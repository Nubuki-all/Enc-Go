package parser

import (
	"context"
	"strings"

	"github.com/5rahim/tanuki"
	"go.uber.org/zap"

	"enc/internal/logger"
	"enc/internal/utils"
	"enc/internal/bot"
	"enc/internal/types"
)

// DynamicThumb tries to download a cover image for `name` and save it to `thumb`.
// Returns (title, episode, season, releaseGroup, err). It mirrors the Python function
// and swallows most errors (returns nil error unless download explicitly fails).
func DynamicThumb(ctx context.Context, name string, thumb string, useAniList bool, filter *types.FilterData) (title, epi, sn, rg string) {
	// Default outputs
	var title, epi, sn, rg string

	// 1) Apply filters (equivalent to filter_name)
	filtered, _, _ := FilterName(name, filter) // returns (filteredName, fil2, fil3)
	if filtered != "" {
		name = filtered
	}

	// 2) Parse with tanuki
	parsed := tanuki.Parse(name, tanuki.DefaultOptions)
	// adapt these field names if your tanuki wrapper differs
	title = parsed.AnimeTitle
	episode = parsed.EpisodeNumber[0]
	season := parsed.AnimeSeason[0]
	releaseGroup = parsed.ReleaseGroup

	// If season is an array in your wrapper adapt accordingly; here we handle leading zeros
	if season != "" {
		// strip leading zeros like Python: "01" -> "1"
		season = strings.TrimLeft(season, "0")
		if season == "1" {
			season = "" // same as Python (drop "1")
		}
	}

	// Auto-rename step (may disable AniList if rename fails)
	// Assume AutoRename(parsedName, originalName, refunc, general) -> (ok bool, newName string, err error)
	// Replace "GetRenameRules()" with whatever returns your rename rules string
	anilistOverride, title = utils.AutoRename(title, title, false, true)
	if useAniList && anilistOverride {
		useAniList = false
	}

	// If parsing is turned off or global config disables AniList, stop here
	if !(bot.AnilistParse && useAniList){
		return title, episode, season, releaseGroup
	}

	// 3) AniList lookup: try title+season first, then title
	var coverURL string
	var tTitle string
	media, aerr := GetAniInfo(title, nil, nil, false)
	if aerr == nil && media != nil {
		// prefer english title, fallback to romaji
		if media.Title.English != "" {
			tTitle = media.Title.English
		} else {
			tTitle = media.Title.Romaji
		}
	} else {
		// fallback to parsed title
		tTitle = title
		if aerr != nil {
			logger.Warnw("AniList lookup failed", zap.Error(err))
		}
	}

	// form search string with season if present
	searchTitle := tTitle
	if sn != "" {
		searchTitle = tTitle + " " + sn
	}

	// Try AniList for searchTitle then for tTitle
	if media2, err2 := GetAniInfo(searchTitle, nil, nil, false); err2 == nil && media2 != nil {
		coverURL = media2.CoverImage.ExtraLarge
	} else if media3, err3 := GetAniInfo(tTitle, nil, nil, false); err3 == nil && media3 != nil {
		coverURL = media3.CoverImage.ExtraLarge
	} else {
		logger.WarnW("Could not fetch Cover Url for:", zap.String("Title", title))
		coverURL = ""
	}

	// 4) Download cover if URL found
	if coverURL != "" {
		if dlErr := utils.downloadToFile(ctx, coverURL, thumb); dlErr != nil {
			// Python suppressed errors — we log and continue
			logger.Warn("DynamicThumb: download failed", "url", coverURL, "err", dlErr)
			// return values but nil error to mirror Python's silent failure
		}
	}

	return title, episode, season, releaseGroup
}

 