package bot

import (
	"enc/internal/bot/queue"
	"enc/internal/bot/state"
	"github.com/siku2/arigo"
)

var (
	AnilistParse    bool
	CaptionParse    bool
	DockerDeployed  bool
	TempOnlyInGroup bool
	ReportFailedDl  bool
	ReportFailedEnc bool
	SAS             bool
	SQS             bool
	Started         bool
	CustomRename    string
	TempUsers       []string
	BatchQueue      map[string]any
	RSSDict         map[string]any
)

var (
	Autoname    string
	FFmpegArgs  string
	FFmpeg2Args string
	FFmpeg3Args string
	FFmpeg4Args string
	MuxArgs     string
)

var ReleaseVersion string

var (
	Aria2Client *arigo.Client
	Queue       = queue.NewQueue()
	Paused      = &state.PauseState{}
)
