package bot

import (
	"enc/internal/bot/queue"
	"github.com/siku2/arigo"
)

var (
	AnilistParse    bool
	CaptionParse    bool
	DockerDeployed  bool
	TempOnlyInGroup bool
	Paused          bool
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

var (
	Aria2Client *arigo.Client
	Queue       = queue.NewQueue()
)
