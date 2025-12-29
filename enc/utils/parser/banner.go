package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/5rahim/tanuki"
	"go.uber.org/zap"

	"enc/config"
	"enc/internal/bot"
	"enc/internal/bot/types"
	"enc/internal/logger"
	"enc/utils"
)

type FPostOptions struct {
	Path           string
	Anilist        bool
	Fcodec         string
	MediaInfoURL   string
	FilterOverride *types.FilterData
	Direct         string
	EncString      string
	Job            utils.EncodeJob
}

func FPost(
	name string,
	opts *FPostOptions,
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

	if opts == nil {
		opts = &FPostOptions{}
	}
	direct := opts.Direct
	if direct == "" {
		name, _, _ = FilterName(name, opts.FilterOverride)
	} else {
		name = direct
	}

	// parsed = anitopy.parse(name)
	parsed := tanuki.Parse(name, tanuki.DefaultOptions)

	// Extract and normalize fields
	title := parsed.AnimeTitle
	epi := ""
	if len(parsed.EpisodeNumber) > 0 {
		epi = parsed.EpisodeNumber[0]
	}
	season := ""
	if len(parsed.AnimeSeason) > 0 {
		season = parsed.AnimeSeason[0]
	}
	relGroup := parsed.ReleaseGroup
	// handle epi logic
	if epi != "" && !config.Conf.FBanner {
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
		opts.Anilist = false
	}

	// codec resolution
	var codec string
	if opts.Fcodec != "" {
		codec = opts.Fcodec
	} else {
		args := []string{}
		for _, data := range opts.Job.PendingInfo().List {
			args = append(args, data.Args)
		}
		codec = utils.GetCodecs(args)
	}

	// Try to fetch anilist info, similar to the inner try/except
	var titleR string
	var con string
	var idStr string
	var genres []string

	if (!bot.AnilistParse && !opts.Anilist) || direct != "" {
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
				idStr = strconv.Itoa(media2.ID)
			} else {
				idStr = strconv.Itoa(media.ID)
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
	if aInfo, sInfo, err := utils.GetStreamInfo(opts.Path); err == nil {
		if aInfo != "" {
			var aParts []string
			for part := range strings.SplitSeq(aInfo, "|") {
				f := GetFlag(part)
				aParts = append(aParts, f)
			}
			aLangFlags = strings.Join(aParts, ", ")
		}
		if sInfo != "" {
			var sParts []string
			for part := range strings.SplitSeq(sInfo, "|") {
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
		b.WriteString(fmt.Sprintf("<code>%s</code>", title))
	} else {
		b.WriteString(fmt.Sprintf("<b>%s</b> | <code>%s</code>", titleR, title))
	}
	b.WriteString("\n\n")
	mi := opts.MediaInfoURL
	if relGroup != "" || mi != "" {
		if relGroup == "" {
			relGroup = "None"
		}
		b.WriteString(fmt.Sprintf("<b>Source:</b> <b><a href=\"%s\"> [%s]</a></b>", mi, relGroup))
		b.WriteString("\n\n")
	}
	if genreStr != "" {
		b.WriteString(fmt.Sprintf("<b>‣ Genre</b> : %s\n", genreStr))
	}
	if epi != "" {
		b.WriteString(fmt.Sprintf("<b>‣ Episode</b> : <code>%s</code>\n", epi))
	}
	if season != "" {
		b.WriteString(fmt.Sprintf("<b>‣ Season</b> : <code>%s</code>\n", season))
	}
	b.WriteString(fmt.Sprintf("<b>‣ Quality</b> : <code>%s</code>\n", codec))
	b.WriteString(fmt.Sprintf("<b>‣ Audio(s)</b> : <code>%s</code>\n", aLangFlags))
	b.WriteString(fmt.Sprintf("<b>‣ Subtitle(s)</b> : <code>%s</code>\n", sLangFlags))

	msg = b.String()

	// success
	return picURL, msg, nil
}
