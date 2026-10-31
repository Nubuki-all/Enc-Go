package anitomy

import (
	"strings"

	"github.com/5rahim/tanuki"
)

func (e *Elements) ToTanukiElements() *tanuki.Elements {
	return &tanuki.Elements{
		AnimeSeason:         e.AnimeSeason,
		AnimePart:           e.AnimePart,
		AnimeSeasonPrefix:   e.AnimeSeasonPrefix,
		AnimePartPrefix:     e.AnimePartPrefix,
		AnimeTitle:          e.AnimeTitle,
		AnimeType:           e.AnimeType,
		AnimeYear:           e.AnimeYear,
		AudioTerm:           e.AudioTerm,
		DeviceCompatibility: e.DeviceCompatibility,
		EpisodeNumber:       e.EpisodeNumber,
		EpisodeNumberAlt:    e.EpisodeNumberAlt,
		EpisodePrefix:       e.EpisodePrefix,
		EpisodeTitle:        e.EpisodeTitle,
		FileChecksum:        e.FileChecksum,
		FileExtension:       e.FileExtension,
		FileName:            e.FileName,
		Language:            e.Language,
		Other:               e.Other,
		ReleaseGroup:        e.ReleaseGroup,
		ReleaseInformation:  e.ReleaseInformation,
		ReleaseVersion:      e.ReleaseVersion,
		Source:              e.Source,
		Subtitles:           e.Subtitles,
		VideoResolution:     strings.Join(e.VideoResolution, " "),
		VideoTerm:           e.VideoTerm,
		VolumeNumber:        e.VolumeNumber,
		VolumePrefix:        e.VolumePrefix,
		Unknown:             e.Unknown,
	}
}
