// Package bot contains runtime variables
package bot

import (
	"sync"
	"time"

	"enc/internal/bot/state"
	"enc/internal/bot/types"
	"enc/internal/clients/mega"

	"github.com/siku2/arigo"
)

var (
	AnilistParse       bool
	AnitomyAvailable   bool
	CaptionParse       bool
	DockerDeployed     bool
	GroupEnc           bool
	TempOnlyInGroup    bool
	RSSRanOnce         bool
	ReportFailedDl     bool
	ReportFailedEnc    bool
	SAS                bool
	SQS                bool
	Started            bool
	CustomRename       string
	TempUsers          []int64
	RSSDictMutex       sync.RWMutex
	ECallbackDataMutex sync.RWMutex
	DownloaderMutex    sync.RWMutex
)

var (
	FFmpegArgs  string
	FFmpeg2Args string
	FFmpeg3Args string
	FFmpeg4Args string
	MuxArgs     string
	// named FFmpeg for historical reasons, even though other cmd tools are supported
)

var ID int64

var ReleaseVersion string

var (
	Aria2Client       *arigo.Client
	MegaClient        = mega.New()
	ECallbackData     = make(map[string]*types.CallbackInfo)
	Filters           = &types.FilterData{}
	Queue             = types.NewQueue()
	RSSDict           = make(map[string]*types.FeedData)
	Paused            = &state.PauseState{}
	RGExtendedFilters = []*types.RGExtendedFilterData{}
	NameFilters       = []*types.MatchedFilterData{}
	ReleaseFilters    = []*types.MatchedFilterData{}
	RenameFilters     = []*types.RenameFilterData{}

	// wait queues
	WaitQueueR = types.NewWaitQueue("cancel_rturn")
	WaitQueueM = types.NewWaitQueue("cancel_mturn")
)

var (
	BotStartTime = time.Now()
	Version      string
)
