package parser

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
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
	// body, _ := ioutil.ReadAll(resp.Body)
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
