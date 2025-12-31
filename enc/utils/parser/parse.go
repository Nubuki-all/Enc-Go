// Package parser and generates new parse names
package parser

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/5rahim/tanuki"
	"go.uber.org/zap"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"enc/config"
	"enc/internal/bot"
	"enc/internal/bot/types"
	"enc/internal/logger"
	"enc/utils"
)

type ParseResult struct {
	FileName      string
	MediaInfoName string
}

func slice2String(slice []string) string {
	var sb strings.Builder
	for _, x := range slice {
		sb.WriteString(fmt.Sprintf("[%s]", x))
	}
	return sb.String()
}

// ParseOptions holds optional parameters for ParseName.
// Any zero-value is treated the same as the previous default for that argument.
type ParseOptions struct {
	Direct         string
	UseAniList     bool
	FilterOverride *types.FilterData
	Version        string
	Path           string
	ConOverride    string
	Quality        string
	EncString      string
	Ext            string
}

func ParseName(
	rawName string,
	opts *ParseOptions,
) (ParseResult, error) {
	// default options if nil
	if opts == nil {
		opts = &ParseOptions{}
	}

	// unpack options to local variables (keeps the original names for minimal changes)
	direct := opts.Direct
	useAniList := opts.UseAniList
	filterOverride := opts.FilterOverride
	version := opts.Version
	path := opts.Path
	conOverride := opts.ConOverride
	encString := opts.EncString
	quality := opts.Quality

	// 1) Direct override
	if direct != "" {
		return ParseResult{direct, direct}, nil
	}

	// 2) First-pass parse for “orTitle”
	baseParsed := tanuki.Parse(rawName, tanuki.DefaultOptions)

	// 3) Apply your filters (returns filteredName, fil2, fil3)
	filteredName, fil2, _ := FilterName(rawName, filterOverride)

	// 4) Second-pass parse for the real fields
	p := tanuki.Parse(filteredName, tanuki.DefaultOptions)

	// 5) Extract and normalize fields
	title := p.AnimeTitle
	orTitle := baseParsed.AnimeTitle
	epi := ""
	if len(p.EpisodeNumber) > 0 {
		epi = p.EpisodeNumber[0]
	}
	season := ""
	if len(p.AnimeSeason) > 0 {
		season = p.AnimeSeason[0]
	}
	relGroup := p.ReleaseGroup
	relInfo := p.ReleaseInformation
	year := p.AnimeYear
	epTitle := p.EpisodeTitle
	sources := p.Source

	// Normalize season (“01”→“1”, drop “1”)
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

	epTitle = SanitizeEpisodeTitle(epTitle)

	// Wrap release info in brackets
	releaseInfo := slice2String(relInfo)
	source := slice2String(sources)

	if title == "" {
		return ParseResult{}, fmt.Errorf("parsing failed for %q", rawName)
	}
	if year != "" {
		title = fmt.Sprintf("%s %s", title, year)
	}

	// 7) Codecs
	codec := quality
	if codec == "" && encString != "" {
		codec = utils.GetCodec(encString)
	} else {
		codec = utils.ProbeQualityCodec(path)
	}

	// 8) AniList lookup, if enabled
	var (
		con  string
		te   string
		aTag string
	)
	useAL := useAniList
	if bot.AnilistParse && useAL {
		media, err := GetAniInfo(title, nil, nil, false)
		if err != nil {
			// disable further AniList overrides
			logger.WarnW("AniList lookup failed", zap.Error(err))
			useAL = false
		} else {
			// English title fallback
			if eng := media.Title.English; eng != "" {
				title = eng
			}
			// romaji fallback
			if title == "" {
				title = media.Title.Romaji
			}
			con = media.CountryOfOrigin
			te = strconv.Itoa(media.Episodes)
			// zero-pad if epi had leading zero
			if strings.HasPrefix(epi, "0") {
				te = "0" + te
			}
			con = ConvertISO(con)
		}
	}

	// 9) Audio/subtitle tag
	if conOverride != "" {
		aTag = conOverride
	} else if fil2 != "" && !strings.EqualFold(fil2, "auto") {
		// if fil2 is non-empty and not "auto", override; if fil2 is empty we preserve existing capInfo
		if !strings.EqualFold(fil2, "disable") {
			aTag = fil2
		}
	} else {
		aTag, _ = GetFileTag(path, false, false)
		if aTag == "" {
			aTag = GetCusTag(filteredName, relGroup, false)
		}
		if aTag == "" && useAL {
			// use country code if nothing else
			aTag = con
		}
		if aTag == "" {
			aTag, _ = GetFileTag(path, false, true)
			if aTag == "?" {
				aTag = ""
			}
		}
	}

	// 10) Title truncation
	shortTitle := title
	if len(shortTitle) > 33 {
		shortTitle = shortTitle[:32] + "…"
		shortTitle = strings.SplitN(shortTitle, ":", 2)[0]
	}

	_, newTitle := utils.AutoRename(shortTitle, orTitle, false, false)
	if newTitle != shortTitle {
		title = newTitle
	}
	shortTitle = newTitle

	// 11) Custom-rename
	if bot.CustomRename != "" {
		out, _ := CustomRename(shortTitle, season, epi, aTag, codec)
		return ParseResult{out, out}, nil
	}

	// 12) Build primary filename
	var primary strings.Builder
	primary.WriteString(config.Rconf.ReleaseName)
	primary.WriteString(" ")
	primary.WriteString(shortTitle)
	if season != "" {
		primary.WriteString(" S")
		primary.WriteString(season)
	}
	if epi != "" {
		primary.WriteString(" - ")
		primary.WriteString(epi)
	}
	if version != "" {
		primary.WriteString("v")
		primary.WriteString(version)
	}
	if releaseInfo == "[END]" || (te != "" && te == epi && season == "") {
		primary.WriteString(" [END]")
	}
	if aTag != "" && aTag != "disable" {
		primary.WriteString(" [")
		primary.WriteString(aTag)
		primary.WriteString("]")
	}

	// 13) Build secondary filename
	sec := primary.String()
	sec = strings.Replace(sec, shortTitle, title, 1)
	if config.Rconf.ReleaseNameB != "" {
		sec = strings.Replace(sec, config.Rconf.ReleaseName, config.Rconf.ReleaseNameB, 1)
	}
	if epTitle != "" && aTag != "" {
		sec = strings.Replace(
			sec,
			fmt.Sprintf("[%s]", aTag),
			fmt.Sprintf("- %s [%s]", epTitle, aTag),
			1,
		)
	}
	if codec != "" {
		sec += " " + codec
	}
	if source != "" {
		sec += source
	}

	var ext string
	if opts.Ext == "" {
		ext = filepath.Ext(path)
	} else {
		ext = opts.Ext
	}
	if ext == "" {
		ext = ".mkv"
	}
	primary.WriteString(ext)

	return ParseResult{
		FileName:      primary.String(),
		MediaInfoName: sec,
	}, nil
}

// SanitizeEpisodeTitle clears the title if it contains any exact "not allowed" keywords.
func SanitizeEpisodeTitle(episodeTitle string) string {
	if episodeTitle == "" {
		return ""
	}

	// Combined list: Original keywords + new keywords
	notAllowed := []string{
		"END", "MULTi", "WEB", "WEB-DL", "DDP5.1",
		"DDP2.0", "AAC2", "AMZN", "DUAL", "NF", "BILI",
	}

	for _, na := range notAllowed {
		// Strict case-sensitive check
		if strings.Contains(episodeTitle, na) {
			return ""
		}
	}

	return episodeTitle
}

type CustCapOptions struct {
	Path          string
	UseAniList    bool
	CustType      string
	Ccd           string
	Ver           string
	Encoder       string
	Filter        *types.FilterData
	CodecOverride string
	Direct        string
	EncString     string
}

// CustCap builds a caption for a given source file (mirror of Python custcap).
// It returns the caption string (or a fallback if an error happens).
func CustCap(
	name string,
	opts *CustCapOptions,
) (string, error) {
	// default options if nil
	if opts == nil {
		opts = &CustCapOptions{}
	}

	// unpack options to local variables for minimal diff with original code
	path := opts.Path
	useAniList := opts.UseAniList
	custType := opts.CustType
	ccd := opts.Ccd
	ver := opts.Ver
	encoder := opts.Encoder
	filter := opts.Filter
	codecOverride := opts.CodecOverride
	direct := opts.Direct
	encString := opts.EncString

	// Direct override
	if direct != "" {
		return fmt.Sprintf("`%s`", direct), nil
	}

	// If FL_CAP is true, return just the filename wrapped in code ticks
	if config.Conf.FlCap {
		return fmt.Sprintf("`%s`", filepath.Base(path)), nil
	}

	// Main logic
	var caption string
	defer func() {
		// In Python, exceptions are swallowed and a default caption is returned.
		// We return caption built so far (or fallback) and nil error unless we explicit return errors.
		_ = caption
	}()

	// Apply filters (equivalent to filter_name)
	fName, _, fil3 := FilterName(name, filter)

	// Parse with tanuki
	baseParsed := tanuki.Parse(name, tanuki.DefaultOptions)
	parsed := tanuki.Parse(fName, tanuki.DefaultOptions)

	orTitle := baseParsed.AnimeTitle
	title := parsed.AnimeTitle

	episode := ""
	if len(parsed.EpisodeNumber) > 0 {
		episode = parsed.EpisodeNumber[0]
	}
	season := ""
	if len(parsed.AnimeSeason) > 0 {
		season = parsed.AnimeSeason[0]
	}
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

	releaseGroup := parsed.ReleaseGroup
	relInfo := parsed.ReleaseInformation
	episodeTitle := parsed.EpisodeTitle
	// yr := parsed.AnimeYear
	sources := parsed.Source

	releaseInfo := slice2String(relInfo)
	source := slice2String(sources)

	// sanitize episode title against not_allowed
	episodeTitle = SanitizeEpisodeTitle(episodeTitle)

	if title == "" {
		return "", fmt.Errorf("parsing failed for %q", name)
	}

	// caption decorator default
	if ccd == "" {
		ccd = config.Conf.CapDeco
	}

	rIsEnd := (releaseInfo == "[END]")

	// codec
	var codec string
	if codecOverride != "" {
		codec = codecOverride
	} else if encString == "" {
		codec = utils.ProbeQualityCodec(path)
	} else {
		codec = utils.GetCodec(encString)
	}

	// determine cap_info: get custom tag for caption (uses get_cus_tag)
	var capInfo string
	// cust_type takes precedence
	if custType != "" {
		capInfo = custType
	} else if fil3 != "" && !strings.EqualFold(fil3, "auto") {
		// if fil3 is non-empty and not "auto", override; if fil3 is empty we preserve existing capInfo
		if !strings.EqualFold(fil3, "disable") {
			capInfo = fil3
		}
	} else {
		capInfo = GetCusTag(fName, releaseGroup, true)
		if capInfo == "" {
			capInfo, _ = GetFileTag(path, true, false)
		}
	}

	// AniList lookup (get episodes count / english title) — optional
	var totalEpisodes string
	if !bot.AnilistParse || !useAniList {
		// Parsing turned off, skip AniList
		logger.Debug("AniList parsing turned off, ignoring AniList Lookup")
	} else {
		// safe AniList lookup
		media, err := GetAniInfo(title, nil, nil, false)
		if err == nil && media != nil {
			if media.Title.English != "" {
				title = media.Title.English
			} else if media.Title.Romaji != "" {
				title = media.Title.Romaji
			}
			if season != "" {
				if media2, err2 := GetAniInfo(fmt.Sprintf("%s %s", title, season), nil, nil, false); err2 == nil && media2 != nil {
					totalEpisodes = fmt.Sprintf("%d", media2.Episodes)
				}
			}
			if totalEpisodes == "" {
				totalEpisodes = fmt.Sprintf("%d", media.Episodes)
			}
			// keep leading-zero logic if original episode started with 0
			if episode != "" && strings.HasPrefix(episode, "0") {
				totalEpisodes = "0" + totalEpisodes
			}
		} else {
			// no AniList info; continue
			logger.WarnW("GetAniInfo failed or returned nil", zap.Error(err))
		}
	}

	// Format title (capitalize words)
	undCaser := cases.Title(language.Und, cases.NoLower)
	title = undCaser.String(title)

	// Auto-rename (assumes AutoRename returns (ok bool, out string, err error))
	_, title = utils.AutoRename(title, orTitle, true, false) // caption=true, general=false

	// compute CRC32 and post MediaInfo URL if file exists
	var crc32s string
	var miURL string
	if utils.FileExists(path) {
		if cs, err := utils.CRC32(path); err == nil {
			crc32s = cs
		} else {
			logger.WarnW("CRC32 failed", zap.Error(err))
		}
		if mi, err := utils.PostMediaInfo(path); err == nil {
			miURL = mi
		} else {
			logger.WarnW("PostMediaInfo failed", zap.Error(err))
		}
	}
	if capInfo == "" && miURL != "" {
		capInfo = "[None]"
	}
	// Build caption
	formatCtx := map[string]string{
		"title":   title,
		"episode": episode,
		"season":  season,
		"ver":     ver,
		"encoder": encoder,
		"codec":   codec,
		"crc32s":  crc32s,
		"ccd":     ccd,
	}
	formatCap := func(s string, ctx map[string]string) string {
		if s == "" {
			return s
		}
		repl := strings.NewReplacer(
			"{title}", ctx["title"],
			"{episode}", ctx["episode"],
			"{season}", ctx["season"],
			"{ver}", ctx["ver"],
			"{encoder}", ctx["encoder"],
			"{codec}", ctx["codec"],
			"{crc32s}", ctx["crc32s"],
			"{ccd}", ctx["ccd"],
		)
		return repl.Replace(s)
	}

	captionBuilder := strings.Builder{}
	captionBuilder.WriteString(fmt.Sprintf("**%s Title:** `%s`\n", ccd, title))

	if episode != "" {
		captionBuilder.WriteString(fmt.Sprintf("**%s Episode:** `%s`", ccd, episode))
		if ver != "" {
			captionBuilder.WriteString(fmt.Sprintf(" (v%s)", ver))
		}
		captionBuilder.WriteString("\n")
	}
	if season != "" {
		captionBuilder.WriteString(fmt.Sprintf("**%s Season:** `%s`\n", ccd, season))
	}

	// cap_info handling: if cap_info and miURL then create markdown link, else inline formatting
	if capInfo != "" {
		capInfoFormatted := formatCap(capInfo, formatCtx)
		if miURL != "" {
			captionBuilder.WriteString(fmt.Sprintf("**%s Type:** [%s](%s)", ccd, strings.TrimSpace(capInfoFormatted), miURL))
		} else {
			captionBuilder.WriteString(fmt.Sprintf("**%s Type:** `%s`", ccd, strings.TrimSpace(capInfoFormatted)))
		}
	}

	// release information
	if !rIsEnd && releaseInfo != "" {
		captionBuilder.WriteString(fmt.Sprintf(" `%s`", releaseInfo))
	}
	// end marker
	if episode == totalEpisodes || rIsEnd {
		captionBuilder.WriteString(" **[END]**\n")
	} else {
		captionBuilder.WriteString("\n")
	}

	if episodeTitle != "" {
		captionBuilder.WriteString(fmt.Sprintf("**%s Episode Title:** `%s`\n", ccd, episodeTitle))
	}

	if codec != "" {
		captionBuilder.WriteString(fmt.Sprintf("**🌟:** `%s`", codec))
		if source != "" {
			captionBuilder.WriteString(fmt.Sprintf(" `%s`", source))
		}
		captionBuilder.WriteString("\n")
	}
	if encoder != "" {
		encr := strings.TrimPrefix(encoder, "@")
		captionBuilder.WriteString(fmt.Sprintf("**%s Encoder:** `%s`\n", ccd, encr))
	}

	captionBuilder.WriteString(fmt.Sprintf("**%s CRC32:** `[%s]`\n", ccd, crc32s))
	captionBuilder.WriteString(fmt.Sprintf("**🔗 %s**", config.Conf.CLink))

	caption = captionBuilder.String()

	return caption, nil
}

// QParse mirrors `qparse` — it returns the "primary" filename generated by ParseName.
func QParse(name string, opts *ParseOptions) (string, error) {
	// ParseName(rawName, direct, useAniList, filterOverride, version, folder)
	// adjust "downloads/" if you want another default folder
	res, err := ParseName(name, opts)
	if err != nil {
		return "", err
	}
	return res.FileName, nil
}

// QParseT mirrors `qparse_t` — it returns a compact title string built from the parsed result.
// Equivalent to: parsed_title + ( " " + year if year ) + ( " " + season if season ).
func QParseT(name, ver string, fil *types.FilterData) (string, error) {
	opts := &ParseOptions{
		FilterOverride: fil,
		Version:        ver,
		UseAniList:     true,
	}
	primary, err := QParse(name, opts)
	if err != nil {
		return "", err
	}

	// Re-parse the produced filename with tanuki to extract anime_title, year, season
	parsed := tanuki.Parse(primary, tanuki.DefaultOptions)
	title := parsed.AnimeTitle

	parts := []string{title}
	if parsed.AnimeYear != "" {
		parts = append(parts, parsed.AnimeYear)
	}
	var season string
	if len(parsed.AnimeSeason) > 0 {
		season = parsed.AnimeSeason[0]
	}
	if season != "" {
		parts = append(parts, season)
	}

	return strings.Join(parts, " "), nil
}
