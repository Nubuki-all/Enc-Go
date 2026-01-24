package anitomy

import (
	"encoding/json"
	"fmt"
)

// FlexibleStringSlice can unmarshal from either a string or an array of strings
type FlexibleStringSlice []string

func (f *FlexibleStringSlice) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as string first
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		*f = []string{str}
		return nil
	}

	// Try to unmarshal as array of strings
	var slice []string
	if err := json.Unmarshal(data, &slice); err != nil {
		return fmt.Errorf("cannot unmarshal %s to string or []string", data)
	}

	*f = slice
	return nil
}

type Elements struct {
	AnimeSeason         FlexibleStringSlice `json:"season,omitempty"`
	AnimePart           FlexibleStringSlice `json:"part,omitempty"`
	AnimeSeasonPrefix   FlexibleStringSlice `json:"anime_season_prefix,omitempty"`
	AnimePartPrefix     FlexibleStringSlice `json:"anime_part_prefix,omitempty"`
	AnimeTitle          string              `json:"title,omitempty"`
	AnimeType           FlexibleStringSlice `json:"type,omitempty"`
	AnimeYear           string              `json:"year,omitempty"`
	AudioTerm           FlexibleStringSlice `json:"audio_term,omitempty"`
	DeviceCompatibility FlexibleStringSlice `json:"device,omitempty"`
	EpisodeNumber       FlexibleStringSlice `json:"episode,omitempty"`
	EpisodeNumberAlt    FlexibleStringSlice `json:"episode_number_alt,omitempty"`
	EpisodePrefix       FlexibleStringSlice `json:"episode_prefix,omitempty"`
	EpisodeTitle        string              `json:"episode_title,omitempty"`
	FileChecksum        string              `json:"file_checksum,omitempty"`
	FileExtension       string              `json:"file_extension,omitempty"`
	FileName            string              `json:"file_name,omitempty"`
	Language            FlexibleStringSlice `json:"language,omitempty"`
	Other               FlexibleStringSlice `json:"other,omitempty"`
	ReleaseGroup        string              `json:"release_group,omitempty"`
	ReleaseInformation  FlexibleStringSlice `json:"release_information,omitempty"`
	ReleaseVersion      FlexibleStringSlice `json:"release_version,omitempty"`
	Source              FlexibleStringSlice `json:"source,omitempty"`
	Subtitles           FlexibleStringSlice `json:"subtitles,omitempty"`
	VideoResolution     string              `json:"video_resolution,omitempty"`
	VideoTerm           FlexibleStringSlice `json:"video_term,omitempty"`
	VolumeNumber        FlexibleStringSlice `json:"volume,omitempty"`
	VolumePrefix        FlexibleStringSlice `json:"volume_prefix,omitempty"`
	Unknown             FlexibleStringSlice `json:"unknown,omitempty"`
	// checkAltNumber      bool
}

func (e *Elements) GetAnimeSeason() []string {
	return []string(e.AnimeSeason)
}

func (e *Elements) GetAnimePart() []string {
	return []string(e.AnimePart)
}
