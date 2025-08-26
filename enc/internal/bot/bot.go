package bot

import (
	"time"

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
	TempUsers       []int64
	BatchIng        []string
	BatchQueue      map[string]any
	ECancel         map[string]any
	RSSDict         map[string]any
	UCancel         []any
	PreviewBatch    any
	PreviewList     any
)

var (
	FFmpegArgs  string
	FFmpeg2Args string
	FFmpeg3Args string
	FFmpeg4Args string
	MuxArgs     string
	// named FFmpeg for historical reasons, even though other encoders are supported
)

var ID int64

var ReleaseVersion string

var (
	Aria2Client       *arigo.Client
	Filters           = &types.FilterData{}
	Queue             = types.NewQueue()
	Paused            = &state.PauseState{}
	RGExtendedFilters = []*types.RGExtendedFilterData{}
	NameFilters       = []*types.MatchedFilterData{}
	ReleaseFilters    = []*types.MatchedFilterData{}
	RenameFilters     = []*types.RenameFilterData{}
)

var (
	BotStartTime = time.Now()
	Version      string
)
