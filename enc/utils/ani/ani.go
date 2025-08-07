package ani

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"strconv"
	"time"

	"go.uber.org/zap"
	"golang.org/x/text/language"
	"github.com/5rahim/tanuki"

	"enc/config"
	"enc/intenal/bot/types"
	"enc/internal/bot"
	"enc/internal/logger"
	"enc/utils"
)

const (
	GraphQLEndpoint = "https://graphql.anilist.co"
)

// GraphQL query templates
var AnimeQuery = `
query ($id: Int, $idMal: Int, $search: String, $type: MediaType, $asHtml: Boolean) {
  Media(id: $id, idMal: $idMal, search: $search, type: $type) {
    id
    idMal
    title {
      romaji
      english
      native
    }
    format
    status
    description(asHtml: $asHtml)
    startDate {
      year
      month
      day
    }
    season
    episodes
    duration
    countryOfOrigin
    source
    trailer {
      id
      site
      thumbnail
    }
    coverImage {
      extraLarge
    }
    bannerImage
    genres
    averageScore
    nextAiringEpisode {
      airingAt
      timeUntilAiring
      episode
    }
    isAdult
    characters(role: MAIN, page: 1, perPage: 10) {
      nodes {
        id
        name {
          full
          native
        }
        image {
          large
        }
        description(asHtml: $asHtml)
        siteUrl
      }
    }
    studios(isMain: true) {
      nodes {
        name
        siteUrl
      }
    }
    siteUrl
  }
}`


// Media holds AniList media info
type Media struct {
	ID                int      `json:"id"`
	IDMal             int      `json:"idMal"`
	Title             Title    `json:"title"`
	Format            string   `json:"format"`
	Status            string   `json:"status"`
	Description       string   `json:"description"`
	Season            string   `json:"season"`
	Episodes          int      `json:"episodes"`
	Duration          int      `json:"duration"`
	CountryOfOrigin   string   `json:"countryOfOrigin"`
	Source            string   `json:"source"`
	CoverImage        Image    `json:"coverImage"`
	BannerImage       string   `json:"bannerImage"`
	Genres            []string `json:"genres"`
	AverageScore      int      `json:"averageScore"`
	NextAiringEpisode NextEp   `json:"nextAiringEpisode"`
	IsAdult           bool     `json:"isAdult"`
	SiteURL           string   `json:"siteUrl"`
}

type Title struct {
	Romaji  string `json:"romaji"`
	English string `json:"english"`
	Native  string `json:"native"`
}

type Image struct {
	ExtraLarge string `json:"extraLarge"`
}

type NextEp struct {
	AiringAt        int `json:"airingAt"`
	TimeUntilAiring int `json:"timeUntilAiring"`
	Episode         int `json:"episode"`
}

// GraphQLRequest defines the request payload
type GraphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

// GraphQLResponse wraps the AniList response
type GraphQLResponse struct {
	Data   map[string]Media `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// GetAniInfo fetches media info from AniList
func GetAniInfo(search string, id, idMal *int, asHtml bool) (*Media, error) {
	vars := make(map[string]interface{})
	vars["search"] = search
	vars["type"] = "ANIME"
	vars["asHtml"] = asHtml
	if id != nil {
		vars["id"] = *id
	}
	if idMal != nil {
		vars["idMal"] = *idMal
	}
	reqBody := GraphQLRequest{Query: AnimeQuery, Variables: vars}
	b, _ := json.Marshal(reqBody)
	resp, err := http.Post(GraphQLEndpoint, "application/json", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	//body, _ := ioutil.ReadAll(resp.Body)
	var gqlResp struct {
		Data struct {
			Media Media `json:"Media"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	err = json.NewDecoder(resp.Body).Decode(&gqlResp)
    if err != nil {
        return nil, err
    }
	if len(gqlResp.Errors) > 0 {
		return nil, fmt.Errorf("AniList error: %s", gqlResp.Errors[0].Message)
	}
	return &gqlResp.Data.Media, nil
}

// GetCusTag determines a custom release or file tag based on filter files.
// fn: filename or release identifier to check
// rg: release group identifier
// caption: whether to apply caption-specific logic
func GetCusTag(fn, rg string, caption bool) (string, error) {
	nameFilter, releaseFilter, captionFilter, err := utils.TextFilter()
	if err != nil {
		return "", err
	}

	var releaseTag string
	// Process release_group filter
	for _, line := range strings.Split(releaseFilter, "\n") {
		parts := strings.Split(line, "|")
		if len(parts) < 3 {
			continue
		}
		check := strings.TrimSpace(parts[0])
		out := strings.TrimSpace(parts[1])
		outCaption := strings.TrimSpace(parts[2])
		if !strings.EqualFold(check, rg) {
			continue
		}
		if caption && len(parts) > 2 {
			out = outCaption
		}
		if strings.EqualFold(out, "disable") {
			break
		}
		releaseTag = out
		break
	}

	// Process file filter
	var fileTagBuilder strings.Builder
	for _, line := range strings.Split(nameFilter, "\n") {
		parts := strings.Split(line, "|")
		if len(parts) < 3 {
			continue
		}
		check := strings.TrimSpace(parts[0])
		out := strings.TrimSpace(parts[1])
		outCaption := strings.TrimSpace(parts[2])
		// decide tag based on caption flag
		if caption {
			out = outCaption
		}
		matched := false
		if strings.HasPrefix(check, "^") {
			// prefix match: substring in filename
			key := strings.TrimPrefix(check, "^")
			matched = strings.Contains(fn, key)
		} else {
			matched = strings.Contains(strings.ToLower(fn), strings.ToLower(check))
		}
		if !matched {
			continue
		}
		if strings.EqualFold(out, "disable") {
			continue
		}
		fileTagBuilder.WriteString(out)
		// stop or continue based on rules
		if strings.HasPrefix(check, "^") {
			break
		}
		if !caption {
			break
		}
		fileTagBuilder.WriteString(" ")
	}
	fileTag := strings.TrimSpace(fileTagBuilder.String())

	// Process caption filter
	var captionTag string
	if caption {
		for _, line := range strings.Split(captionFilter, "\n") {
			parts := strings.Split(line, "||")
			if len(parts) < 2 {
				continue
			}
			rgCheck := parts[0]
			items := parts[1:]
			if rgCheck != rg {
				continue
			}
			for _, itm := range items {
				p := strings.Split(itm, "|")
				if len(p) < 2 {
					continue
				}
				check := p[0]
				out := p[1]
				if strings.Contains(fn, check) {
					captionTag = out
					break
				}
			}
			if captionTag != "" {
				break
			}
		}
	}

	// Determine final output
	if releaseTag != "" || captionTag != "" {
		final := releaseTag
		if captionTag != "" {
			if final != "" {
				final = captionTag + " : " + final
			} else {
				final = captionTag
			}
		}
		return final, nil
	}
	// default to fileTag when no release/caption tags
	return fileTag, nil
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

func filterName(name string, filter *types.FilterData) (string, string, string) {
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
	if s := fil.Fil2; s != "" {
		fil2 = s
	}
	if s := fil.Fil3; s != "" {
		fil3 = s
	}
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

type ParseResult struct {
    FileName   string
    MediaInfoName string
}

// Media and GetAniInfo assumed defined as you shared.

// ParseName applies tanuki + AniList + your filters + codecs to build filenames.
func ParseName(
    rawName string,
    direct string,
    useAniList bool,
    filterOverride *types.FilterData,
    version string,
    folder string,
    encString string,
) (ParseResult, error) {
    // 1) Direct override
    if direct != "" {
        return ParseResult{direct, direct}, nil
    }

    // 2) First-pass parse for “orTitle”
    baseParsed := tanuki.Parse(rawName, tanuki.DefaultOptions)

    // 3) Apply your filters (returns filteredName, fil2, fil3)
    filteredName, fil2, fil3 := filterName(rawName, filterOverride)

    // 4) Second-pass parse for the real fields
    p := tanuki.Parse(filteredName, tanuki.DefaultOptions)

    // 5) Extract and normalize fields
    title := p.AnimeTitle
    orTitle := baseParsed.AnimeTitle
    epi := p.EpisodeNumber[0]
    season := p.AnimeSeason[0]
    relGroup := p.ReleaseGroup
    relInfo := p.ReleaseInformation
    year := p.AnimeYear
    epTitle := p.EpisodeTitle
    sources := p.Source

    // Normalize season (“01”→“1”, drop “1”)
    if season != "" {
        season = strings.TrimLeft(season, "0")
        if season == "" {
            season = "1"
        }
        if season == "1" {
            season = ""
        }
    }

    // Wrap release info in brackets
    
    slice2String := func(slice []string) string {
    	var sb strings.Builder
    	for _, x := range slice {
    		sb.WriteString(fmt.Sprintf("[%s]", x))
    	}
        return sb.String()
    }
    ri := slice2String(relInfo)
    source := slice2String(sources)
    if title == "" {
        return ParseResult{}, fmt.Errorf("parsing failed for %q", rawName)
    }
    if year != "" {
        title = fmt.Sprintf("%s %s", title, year)
    }

    // 6) Build input-file path
    if !strings.HasSuffix(folder, "/") {
        folder += "/"
    }
    infile := filepath.Join(folder, filteredName)

    // 7) Codecs
    codec := utils.GetCodec(encString)
    // 8) AniList lookup, if enabled
    var (
        con   string
        te    string
        useAL = useAniList
    )
    if !bot.AnilistParse && useAL {
        media, err := GetAniInfo(title, nil, nil, false)
        if err != nil {
            // disable further AniList overrides
            logger.Warn("AniList lookup failed", "error", err)
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
    aTag, _ := GetFileTag(infile, false, false)
    if aTag == "" {
        aTag, _ = GetCusTag(filteredName, relGroup, false)
    }
    if aTag == "" && useAL {
        // use country code if nothing else
        aTag = con
    }

    // 10) Title truncation
    shortTitle := title
    if len(shortTitle) > 33 {
        shortTitle = shortTitle[:32] + "…"
        shortTitle = strings.SplitN(shortTitle, ":", 2)[0]
    }

    // 11) Custom-rename
    if bot.CustomRename != "" {
        out, _ := CustomRename(shortTitle, season, epi, aTag, codec)
        return ParseResult{out, out}, nil
    }

    // 12) Build primary filename
    var primary strings.Builder
    primary.WriteString(config.Conf.ReleaseName)
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
    if ri == "[END]" || (te != "" && te == epi && season == "") {
        primary.WriteString(" [END]")
    }
    if aTag != "" {
        primary.WriteString(" [")
        primary.WriteString(aTag)
        primary.WriteString("]")
    }
    primary.WriteString(".mkv")

    // 13) Build secondary filename
    sec := primary.String()
    sec = strings.Replace(sec, shortTitle, title, 1)
    if config.Conf.ReleaseNameB != "" {
        sec = strings.Replace(sec, config.ReleaseName, config.ReleaseNameB, 1)
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

    return ParseResult{
        FileName:   primary.String(),
        MediaInfoName: sec,
    }, nil
}
