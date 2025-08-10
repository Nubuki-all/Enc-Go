package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/5rahim/tanuki"
	"go.uber.org/zap"

	"enc/config"
	"enc/intenal/bot/types"
	"enc/internal/bot"
	"enc/internal/logger"
	"enc/utils"
)

func FPost(
	name string,
	out string,
	anilist bool,
	fcodec string,
	mi string,
	filterOverride *types.FilterData,
	evt bool,
	direct string,
	encString string,
) (picURL string, msg string, err error) {
	if config.Conf.NoBanner {
		return "", "", nil
	}

	defer func() {
		// Here any unexpected panic -> log and return nil-equivalent values and an error.
		if r := recover(); r != nil {
			if err, ok := r.(error); ok {
				logger.Errorf("panic: %v", err)
			}
			picURL = ""
			msg = ""
			err = fmt.Errorf("panic: %v", r)
		}
	}()

	if direct == "" {
		name, _, _ = FilterName(name, filterOverride)
	} else {
		name = direct
	}

	// parsed = anitopy.parse(name)
	parsed := tanuki.Parse(name, tanuki.DefaultOptions)

	// Extract and normalize fields
	title := parsed.AnimeTitle
	epi := parsed.EpisodeNumber[0]
	season := parsed.AnimeSeason[0]
	relGroup := parsed.ReleaseGroup
	// handle epi logic
	if epi != "" && !evt {
		if epi == "1" || epi == "01" || epi == "001" {
			epi = ""
		} else {
			return "", "", nil
		}
	}

	// season normalization
	if season != "" {
		if strings.HasPrefix(season, "0") {
			if i, errConv := strconv.Atoi(season); errConv == nil {
				season = strconv.Itoa(i)
			}
		}
		if season == "1" {
			season = ""
		}
	}

	var anilistOverride bool

	anilistOverride, title = utils.AutoRename(title, title, false, true)
	if anilistOverride {
		anilist = false
	}

	// codec resolution
	var codec string
	if fcodec != "" {
		codec = fcodec
	} else {
		codec = utils.GetCodecs(utils.EJob.PendingInfo().List)
	}

	// Try to fetch anilist info, similar to the inner try/except
	var titleR string
	var con string
	var idStr string
	var genres []string

	if !(bot.AnilistParse && anilist) || direct != "" {
		logger.DebugW("Parsing turned off")
	} else {
		media, err := GetAniInfo(title, nil, nil, false)
		var media2 *Media
		if err != nil {
			logger.WarnW("AniList lookup failed", zap.Error(err))
		} else {
			if season != "" {
				sTitle := strings.TrimSpace(title + " " + season)
				media2, _ = GetAniInfo(sTitle, nil, nil, false)
			}
			title = media.Title.English
			titleR = media.Title.Romaji
			if title == "" {
				title = titleR
			}
			if media2 != nil {
				idStr, _ = strconv.Itoa(media2.ID)
			} else {
				idStr, _ = strconv.Itoa(media.ID)
			}
			picURL = "https://img.anili.st/media/" + idStr
			con = media.CountryOfOrigin
			genres = media.Genres
		}
	}

	if titleR == "" {
		titleR = "N/A"
	}
	if con == "" {
		con = "?"
	}
	if picURL == "" {
		picURL = "https://upload.wikimedia.org/wikipedia/commons/d/d1/Image_not_available.png"
	}

	// stream info
	aLangFlags := "N/A"
	sLangFlags := "N/A"
	if aInfo, sInfo, err := utils.GetStreamInfo(out); err == nil {
		if aInfo != "" {
			var aParts []string
			for _, part := range strings.Split(aInfo, "|") {
				f := GetFlag(part)
				aParts = append(aParts, f)
			}
			aLangFlags = strings.Join(aParts, ", ")
		}
		if sInfo != "" {
			var sParts []string
			for _, part := range strings.Split(sInfo, "|") {
				f := GetFlag(part)
				sParts = append(sParts, f)
			}
			sLangFlags = strings.Join(sParts, ", ")
		}
	}

	// country flag
	cflag := con
	if con != "?" {
		cflag = countryCodeToFlag(con)
	}

	// genres formatting -> "#Genre_Name "
	genreStr := ""
	if len(genres) > 0 {
		var segs []string
		for _, s := range genres {
			// replace spaces and hyphens
			n := strings.ReplaceAll(s, " ", "_")
			n = strings.ReplaceAll(n, "-", "_")
			segs = append(segs, "#"+n)

		}
		genreStr = strings.Join(segs, " ") + " "
	}

	// Build the message using the same formatting as python f-string building
	var b strings.Builder
	b.WriteString(fmt.Sprintf("[%s]", cflag))
	if title == titleR {
		b.WriteString(fmt.Sprintf("`%s`", title))
	} else {
		b.WriteString(fmt.Sprintf("**%s** | `%s`", titleR, title))
	}
	b.WriteString("\n\n")
	if relGroup != "" || mi != "" {
		if relGroup == "" {
			relGroup = "None"
		}
		b.WriteString(fmt.Sprintf("**Source:** **[[%s]](%s)**", relGroup, mi))
		b.WriteString("\n\n")
	}
	if genreStr != "" {
		b.WriteString(fmt.Sprintf("**‣ Genre** : %s\n", genreStr))
	}
	if epi != "" {
		b.WriteString(fmt.Sprintf("**‣ Episode** : `%s`\n", epi))
	}
	if season != "" {
		b.WriteString(fmt.Sprintf("**‣ Season** : `%s`\n", season))
	}
	b.WriteString(fmt.Sprintf("**‣ Quality** : `%s`\n", codec))
	b.WriteString(fmt.Sprintf("**‣ Audio(s)** : `%s`\n", aLangFlags))
	b.WriteString(fmt.Sprintf("**‣ Subtitle(s)** : `%s`\n", sLangFlags))

	msg = b.String()

	// success
	return picURL, msg, nil
}
