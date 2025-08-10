package parser

import (
	"bytes"
	"fmt"
	"html"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/dustin/go-humanize"
	"go.uber.org/zap"

	"enc/utils"
)

// Template used for the final markdown message. Using text/template avoids
// any backtick quoting issues and is type-safe.
const animeTemplate = "" +
	"[{{ .CFlag }}]**{{ .Romaji }}**\n\n" +
	"**ID | MAL ID:** `{{ .IDM }}` | `{{ .IDMal }}`\n" +
	"➤ **SOURCE:** `{{ .Source }}`\n" +
	"➤ **TYPE:** `{{ .Formats }}`\n" +
	"➤ **GENRES:** `{{ .Genre }}`\n" +
	"➤ **SEASON:** `{{ .Season }}`\n" +
	"➤ **EPISODES:** `{{ .Episodes }}`\n" +
	"➤ **STATUS:** `{{ .Status }}`\n" +
	"➤ **NEXT AIRING:** `{{ .AirOn }}`\n" +
	"➤ **SCORE:** `{{ .Score }}` 🌟\n" +
	"➤ **ADULT RATED:** `{{ .Adult }}`\n" +
	"🎬 {{ .TrailerLink }}\n" +
	"📖 [Synopsis & More]({{ .SynopsisLink }})"

// data passed into the template
type animeTemplateData struct {
	CFlag        string
	Romaji       string
	IDM          int
	IDMal        int
	Source       string
	Formats      string
	Genre        string
	Season       interface{}
	Episodes     interface{}
	Status       string
	AirOn        string
	Score        interface{}
	Adult        bool
	TrailerLink  string
	SynopsisLink string
}

// MakeItRW: convert UNIX timestamp to readable string. If asCountdown is true,
// return a humanized relative time; otherwise a friendly absolute date.
func MakeItRW(unixTS int64, asCountdown bool) string {
	t := time.Unix(unixTS, 0)
	if asCountdown {
		return humanize.Time(t) // e.g. "in 3 days"
	}
	return t.Format("Jan 2, 2006")
}

// AnimeArch performs the AniList lookup and returns (coverImageURL, message, error).
// `useMal` corresponds to Python arg.m (if true, treat numeric query as MAL id).
func AnimeArch(query string, useMal bool) (string, string, error) {
	// prepare vars for GetAniInfo
	var idPtr *int
	var idMalPtr *int
	if n, err := strconv.Atoi(query); err == nil {
		if useMal {
			idMalPtr = &n
		} else {
			idPtr = &n
		}
	}

	// fetch from AniList (asHtml = true so descriptions are HTML)
	media, err := GetAniInfo(query, idPtr, idMalPtr, true)
	if err != nil {
		return "", "", fmt.Errorf("GetAniInfo error: %w", err)
	}
	if media == nil {
		return "", "", fmt.Errorf("AniList returned no media")
	}

	// extract fields (names follow your Media struct; adapt if needed)
	idm := media.ID
	idmal := media.IDMal
	romaji := media.Title.Romaji
	english := media.Title.English
	native := media.Title.Native
	formats := media.Format
	status := media.Status
	synopsis := media.Description // HTML (asHtml=true)
	season := media.Season
	episodes := media.Episodes
	country := media.CountryOfOrigin
	cFlag := countryCodeToFlag(country)
	source := media.Source
	coverImg := ""
	if media.CoverImage.ExtraLarge != "" {
		coverImg = media.CoverImage.ExtraLarge
	}
	bannerImg := media.BannerImage
	genres := media.Genres
	genre := ""
	if len(genres) > 0 {
		if len(genres) == 1 {
			genre = genres[0]
		} else {
			genre = strings.Join(genres, ", ")
		}
	}
	score := media.AverageScore
	var airOn string
	if media.NextAiringEpisode != nil {
		airOn = MakeItRW(int64(media.NextAiringEpisode.AiringAt), false)
	}

	// trailer link (only handle youtube per original logic)
	trailerLink := "N/A"
	if media.Trailer != nil && strings.EqualFold(media.Trailer.Site, "youtube") && media.Trailer.ID != "" {
		trailerLink = fmt.Sprintf("[Trailer](https://youtu.be/%s)", media.Trailer.ID)
	}

	// build characters HTML block
	var charsBuf bytes.Buffer
	for _, character := range media.Characters.Nodes {
		imgPart := ""
		if character.Image.Large != "" {
			imgPart = fmt.Sprintf(`<a href="%s"><img src="%s"/></a><br>`, html.EscapeString(character.SiteURL), html.EscapeString(character.Image.Large))
		}
		charsBuf.WriteString("<br>")
		charsBuf.WriteString(imgPart)
		charsBuf.WriteString("<br>")
		charsBuf.WriteString(fmt.Sprintf("<h3>%s</h3>", html.EscapeString(character.Name.Full)))
		charsBuf.WriteString(fmt.Sprintf("<em>%s %s</em><br>", cFlag, html.EscapeString(character.Name.Native)))
		charsBuf.WriteString(fmt.Sprintf("<b>Character ID</b>: %d<br>", character.ID))
		// description is HTML already (asHtml=true)
		desc := character.Description
		if desc == "" {
			desc = "N/A"
		}
		charsBuf.WriteString(fmt.Sprintf("<h4>About Character and Role:</h4>%s", desc))
		charsBuf.WriteString("<br><br>")
	}

	// studios listing
	var studiosBuf strings.Builder
	for _, s := range media.Studios.Nodes {
		if s.SiteURL != "" {
			studiosBuf.WriteString(fmt.Sprintf(`<a href="%s">• %s</a> `, html.EscapeString(s.SiteURL), html.EscapeString(s.Name)))
		} else {
			studiosBuf.WriteString(fmt.Sprintf("• %s ", html.EscapeString(s.Name)))
		}
	}
	studios := studiosBuf.String()

	// build HTML page to post
	var pageBuf bytes.Buffer
	titleImg := coverImg
	if titleImg == "" {
		titleImg = bannerImg
	}
	if titleImg != "" {
		pageBuf.WriteString(fmt.Sprintf(`<img src="%s" title="%s"/>`, html.EscapeString(titleImg), html.EscapeString(romaji)))
	}
	pageBuf.WriteString(fmt.Sprintf(`<h1>[%s] %s</h1>`, html.EscapeString(cFlag), html.EscapeString(native)))
	pageBuf.WriteString("<h3>Synopsis:</h3>")
	pageBuf.WriteString(synopsis)
	pageBuf.WriteString("<br>")
	if charsBuf.Len() > 0 {
		pageBuf.WriteString("<h2>Main Characters:</h2>")
		pageBuf.WriteString(charsBuf.String())
		pageBuf.WriteString("<br><br>")
	}
	pageBuf.WriteString("<h3>More Info:</h3>")
	if media.StartDate.Year != 0 {
		pageBuf.WriteString(fmt.Sprintf("<b>Started On:</b> %02d/%02d/%04d", media.StartDate.Day, media.StartDate.Month, media.StartDate.Year))
	}
	pageBuf.WriteString(fmt.Sprintf("<br><b>Studios:</b> %s<br>", studios))
	if idmal != 0 {
		pageBuf.WriteString(fmt.Sprintf(`<a href="https://myanimelist.net/anime/%d">View on MAL</a>`, idmal))
	}
	if media.SiteURL != "" {
		pageBuf.WriteString(fmt.Sprintf(`<a href="%s"> View on anilist.co</a>`, html.EscapeString(media.SiteURL)))
	}
	if bannerImg != "" {
		pageBuf.WriteString(fmt.Sprintf(`<img src="%s"/>`, html.EscapeString(bannerImg)))
	}

	// upload the HTML page (PostToTGPH assumed to exist and return (url string, err error))
	titleH := english
	if titleH == "" {
		titleH = romaji
	}
	synopsisLink := ""
	if titleH != "" {
		if p, perr := utils.PostToTelegraph(titleH, pageBuf.String()); perr == nil {
			synopsisLink = p.Url
		} else {
			zap.L().Warn(perr.Error())
		}
	}

	// Prepare data for template and render it
	tmplData := animeTemplateData{
		CFlag:        cFlag,
		Romaji:       romaji,
		IDM:          idm,
		IDMal:        idmal,
		Source:       source,
		Formats:      formats,
		Genre:        genre,
		Season:       season,
		Episodes:     episodes,
		Status:       status,
		AirOn:        airOn,
		Score:        score,
		Adult:        media.IsAdult,
		TrailerLink:  trailerLink,
		SynopsisLink: synopsisLink,
	}

	tmpl, err := template.New("anime").Parse(animeTemplate)
	if err != nil {
		return coverImg, "", fmt.Errorf("template parse error: %w", err)
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, tmplData); err != nil {
		return coverImg, "", fmt.Errorf("template exec error: %w", err)
	}

	return coverImg, out.String(), nil
}

// AiringAnim returns (coverImgURL, formattedMessage, error).
// If useMal is true and query is numeric it will call AniList with idMal.
// This mirrors the Python airing_anim logic.
func AiringAnim(query string, useMal bool) (coverImg string, out string, err error) {
	// Prepare id pointers if query is numeric
	var idPtr *int
	var idMalPtr *int
	if n, perr := strconv.Atoi(query); perr == nil {
		if useMal {
			idMalPtr = &n
		} else {
			idPtr = &n
		}
	}

	// Call AniList using your GetAniInfo helper. Use asHtml=true (like Python).
	media, err := GetAniInfo(query, idPtr, idMalPtr, true)
	if err != nil {
		return "", "", fmt.Errorf("GetAniInfo error: %w", err)
	}
	if media == nil {
		return "", "", fmt.Errorf("no media returned")
	}

	// Extract fields
	mid := media.ID
	romaji := media.Title.Romaji
	english := media.Title.English
	native := media.Title.Native
	status := media.Status
	episodes := media.Episodes
	country := media.CountryOfOrigin
	cFlag := countryCodeToFlag(country)
	source := media.Source
	coverImg = media.CoverImage.ExtraLarge
	genres := media.Genres
	genre := ""
	if len(genres) > 0 {
		if len(genres) == 1 {
			genre = genres[0]
		} else {
			genre = strings.Join(genres, ", ")
		}
	}
	score := media.AverageScore

	// next airing
	var airOn string
	var epNum int
	if media.NextAiringEpisode != nil {
		airOn = MakeItRW(int64(media.NextAiringEpisode.AiringAt), true) // as_countdown True
		epNum = media.NextAiringEpisode.Episode
	}

	// pick displayed title (english or romaji)
	title_ := english
	if title_ == "" {
		title_ = romaji
	}

	// Build output the same way as Python
	var b strings.Builder
	b.WriteString(fmt.Sprintf("[%s] **%s** \n   (`%s`)", cFlag, native, title_))
	b.WriteString(fmt.Sprintf("\n\n**ID:** `%d`", mid))
	b.WriteString(fmt.Sprintf("\n**Status:** `%s`\n", status))
	b.WriteString(fmt.Sprintf("**Source:** `%s`\n", source))
	b.WriteString(fmt.Sprintf("**Score:** `%d`\n", score))
	b.WriteString(fmt.Sprintf("**Genres:** `%s`\n", genre))
	if airOn != "" {
		b.WriteString(fmt.Sprintf("**Airing Episode:** `[%d/%d]`\n", epNum, episodes))
		b.WriteString(fmt.Sprintf("\n`%s`", airOn))
	}

	return coverImg, b.String(), nil
}
