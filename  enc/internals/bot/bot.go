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
    CustomRename    string
    TempUsers       []string
)

var (
    FFmpegArgs   string
    FFmpeg2Args  string
    FFmpeg3Args  string
    FFmpeg4Args  string
    MuxArgs      string
)

var Aria2Client *arigo.Client
var Queue = queue.NewQueue()
