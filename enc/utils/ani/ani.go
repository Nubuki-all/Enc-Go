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
	ID                 int      `json:"id"`
	IDMal              int      `json:"idMal"`
	Title              Title    `json:"title"`
	Format             string   `json:"format"`
	Status             string   `json:"status"`
	Description        string   `json:"description"`
	Season             string   `json:"season"`
	Episodes           int      `json:"episodes"`
	Duration           int      `json:"duration"`
	CountryOfOrigin    string   `json:"countryOfOrigin"`
	Source             string   `json:"source"`
	CoverImage         Image    `json:"coverImage"`
	BannerImage        string   `json:"bannerImage"`
	Genres             []string `json:"genres"`
	AverageScore       int      `json:"averageScore"`
	NextAiringEpisode  NextEp   `json:"nextAiringEpisode"`
	IsAdult            bool     `json:"isAdult"`
	SiteURL            string   `json:"siteUrl"`
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
	AiringAt       int `json:"airingAt"`
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
	Errors []struct { Message string `json:"message"` } `json:"errors"`
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
		Data struct { Media Media `json:"Media"` } `json:"data"`
		Errors []struct{ Message string `json:"message"` } `json:"errors"`
	}
	if err := json.Unmarshal(body, &gqlResp); err != nil {
		return nil, err
	}
	if len(gqlResp.Errors) > 0 {
		return nil, fmt.Errorf("AniList error: %s", gqlResp.Errors[0].Message)
	}
	return &gqlResp.Data.Media, nil
}

// GetFlag returns country flag emoji for a given country code (stub)
func GetFlag(countryCode string) string {
	// TODO: implement flag lookup, e.g. via a map or external package
	return ""
}

// ConvertISO converts ISO2 to ISO3 codes (stub)
func ConvertISO(code2 string) string {
	// TODO: use a mapping or external library
	return ""
}

// CustomRename applies a custom pattern for renaming
func CustomRename(pattern, title, season, episode, audio, codec string) (string, string) {
	// TODO: implement formatting logic
	return "", ""
}

// ParseName extracts anime info from file name (stub)
func ParseName(filename string) (map[string]string, error) {
	// TODO: integrate with anitopy equivalent or simple regex parsing
	return nil, nil
}

// Additional functions like FilterName, GenerateCaption, DynamicThumb, etc.
// can be added here following similar stubs and TODOs for complex logic.
