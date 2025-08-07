package bot

import (
	"enc/internal/bot/state"
	"enc/internal/bot/types"
	"github.com/siku2/arigo"
)

var (
	AnilistParse    bool
	CaptionParse    bool
	DockerDeployed  bool
	GroupEnc        bool
	TempOnlyInGroup bool
	ReportFailedDl  bool
	ReportFailedEnc bool
	SAS             bool
	SQS             bool
	Started         bool
	CustomRename    string
	TempUsers       []string
	BatchIng        []string
	BatchQueue      map[string]any
	ECancel         map[string]any
	RSSDict         map[string]any
	UCancel         []any
	PreviewBatch    any
	PreviewList     any
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
	Filters     = &types.FilterData{}
	Queue       = types.NewQueue()
	Paused      = &state.PauseState{}
	RGCaptionFilters = []*types.RGCaptionFilterData{}
	NameFilters = []*types.MatchedFilterData{}
	ReleaseFilters = []*types.MatchedFilterData{}
	RenameFilters =  []*types.RenameFilterData{}
)
