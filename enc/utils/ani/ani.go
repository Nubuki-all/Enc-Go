package ani

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	
	"golang.org/x/text/language"
	"go.uber.org/zap"
	
	"enc/internal/bot"
	"enc/intenal/bot/types"
	"enc/internal/logger"
	"enc/utils"
)

const (
	GraphQLEndpoint = "https://graphql.anilist.co"
)

// GraphQL query templates
var AnimeQuery = `query ($id: Int, $idMal:Int, $search: String, $type: MediaType, $asHtml: Boolean) {
  Media (id: $id, idMal: $idMal, search: $search, type: $type) {
    id
    idMal
    title { romaji english native }
    format status description(asHtml: $asHtml)
    startDate { year month day }
    season episodes duration countryOfOrigin source(trailer:{site,thumbnail})
    coverImage{extraLarge} bannerImage genres averageScore
    nextAiringEpisode { airingAt timeUntilAiring episode }
    isAdult
    characters(role:MAIN,page:1,perPage:10) {
      nodes { id name{full native} image{large} description(asHtml:$asHtml) siteUrl }
    }
    studios(isMain:true){ nodes{ name siteUrl }}
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
	body, _ := ioutil.ReadAll(resp.Body)
	var gqlResp struct {
		Data struct {
			Media Media `json:"Media"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &gqlResp); err != nil {
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
	
	nameFilter, releaseFilter, captionFilter := utils.TextFilter() 

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
	region := tag.Region()
	if region.IsUnknown() {
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
	var err         error
	// force "ara" to return arabia's flag
	if lanCode == "ara" {
		countryCode = "SA"
	} else {
		countryCode, err = langTagToRegion(lanCode)
	}
	if err != nil{
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
	} else if bot.Filters != nil{
		fil = bot.Filters
	} else {
		fil := types.FilterData{}
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


// ParseName extracts anime info from file name (stub)
func ParseName(filename string) (map[string]string, error) {
	// TODO: integrate with anitopy equivalent or simple regex parsing
	return nil, nil
}

// Additional functions like FilterName, GenerateCaption, DynamicThumb, etc.
// can be added here following similar stubs and TODOs for complex logic.
