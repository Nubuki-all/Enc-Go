package parser

import (
	"context"
	"strconv"
	"strings"

	"github.com/5rahim/tanuki"
	"go.uber.org/zap"

	"enc/internal/bot"
	"enc/internal/bot/types"
	"enc/internal/logger"
	"enc/utils"
)

type DynamicThumbResult struct {
	Downloaded   bool
	Title        string
	Episode      string
	Season       string
	ReleaseGroup string
}

// DynamicThumb tries to download a cover image for `name` and save it to `thumb`.
// Returns (result.Title, episode, result.Season, releaseGroup, err). It mirrors the Python function
// and swallows most errors (returns nil error unless download explicitly fails).
func DynamicThumb(ctx context.Context, name string, thumb string, useAniList bool, filter *types.FilterData) DynamicThumbResult {
	// Default outputs
	var anilistOverride bool
	var result DynamicThumbResult
	// 1) Apply filters (equivalent to filter_name)
	filtered, _, _ := FilterName(name, filter) // returns (filteredName, fil2, fil3)
	if filtered != "" {
		name = filtered
	}

	// 2) Parse with tanuki
	parsed := tanuki.Parse(name, tanuki.DefaultOptions)
	// adapt these field names if your tanuki wrapper differs
	result.Title = parsed.AnimeTitle
	if len(parsed.EpisodeNumber) > 0 {
		result.Episode = parsed.EpisodeNumber[0]
	}
	if len(parsed.AnimeSeason) > 0 {
		result.Season = parsed.AnimeSeason[0]
	}
	result.ReleaseGroup = parsed.ReleaseGroup

	if result.Season != "" {
		if strings.HasPrefix(result.Season, "0") {
			if i, errConv := strconv.Atoi(result.Season); errConv == nil {
				result.Season = strconv.Itoa(i)
			}
		}
		if result.Season == "1" {
			result.Season = ""
		}
	}

	// Auto-rename step (may disable AniList if rename fails)
	// Assume AutoRename(parsedName, originalName, refunc, general) -> (ok bool, newName string, err error)
	// Replace "GetRenameRules()" with whatever returns your rename rules string
	anilistOverride, result.Title = utils.AutoRename(result.Title, result.Title, false, true)
	if useAniList && anilistOverride {
		useAniList = false
	}

	// If parsing is turned off or global config disables AniList, stop here
	if !bot.AnilistParse && !useAniList {
		println(true)
		return result
	}

	// 3) AniList lookup: try result.Title+result.Season first, then result.Title
	var coverURL string
	var tTitle string
	media, aerr := GetAniInfo(result.Title, nil, nil, false)
	if aerr == nil && media != nil {
		// prefer english result.Title, fallback to romaji
		if media.Title.English != "" {
			tTitle = media.Title.English
		} else {
			tTitle = media.Title.Romaji
		}
	} else {
		// fallback to parsed result.Title
		tTitle = result.Title
		if aerr != nil {
			logger.WarnW("AniList lookup failed", zap.Error(aerr))
		}
	}

	// form search string with result.Season if present
	searchTitle := tTitle
	if result.Season != "" {
		searchTitle = tTitle + " " + result.Season
	}

	// Try AniList for searchTitle then for tTitle
	if media2, err2 := GetAniInfo(searchTitle, nil, nil, false); err2 == nil && media2 != nil {
		coverURL = media2.CoverImage.ExtraLarge
	} else if media3, err3 := GetAniInfo(tTitle, nil, nil, false); err3 == nil && media3 != nil {
		coverURL = media3.CoverImage.ExtraLarge
	} else {
		logger.WarnW("Could not fetch Cover Url for:", zap.String("Title", result.Title))
		coverURL = ""
	}
	println(coverURL)

	// 4) Download cover if URL found
	if coverURL != "" {
		if dlErr := utils.DownloadToFile(ctx, coverURL, thumb); dlErr != nil {
			// Python suppressed errors — we log and continue
			logger.Warn("DynamicThumb: download failed", "url", coverURL, "err", dlErr)
			// return values but nil error to mirror Python's silent failure
		} else {
			result.Downloaded = true
		}
	}

	return result
}
