package config

import (
	"fmt"
	"log"
	"strings"

	"github.com/spf13/viper"
)

// Config holds application configuration loaded from .env
type Config struct {
	ADLTimeout            int
	AlwaysDeployLatest    bool
	AllowAction           bool
	AppID                 int
	APIHash               string
	Aria2Port             int
	BotToken              string
	CacheDL               bool
	CapDeco               string
	CLink                 string
	CmdSuffix             string
	CompatMode            bool
	CustomRename          string
	DatabaseURL           string
	DBName                string
	Dev                   int
	DLStuff               string
	DumpChannel           int
	DumpLeech             bool
	Dyno                  string
	Encoder               string
	ExtCap                bool
	FBanner               bool
	FChannel              int
	FChannelStat          int
	FCodec                string
	FFmpeg                string
	FFmpeg2               string
	FFmpeg3               string
	FFmpeg4               string
	FinishedProgressStr   string
	FlCap                 bool
	FSThreshold           int
	FSticker              string
	LocalDBPath           string
	LockOnStartup         bool
	LogChannel            int64
	LogsInChannel         bool
	MICap                 bool
	MuxArgs               string
	NoBanner              bool
	NoTempPM              bool
	Ovr                   string
	Owner                 string
	PauseOnDLInfo         bool
	QDLTimeout            int
	QBitPort              int
	QBitPort2             int
	QBitTimeout           int
	Releaser              string
	ReportFailed          bool
	ReportFailedDL        bool
	ReportFailedEnc       bool
	RSSChat               string
	RSSDelay              int
	RSSDirect             bool
	TelegraphAPI          string
	TelegraphAuthor       string
	TempUser              string
	TGDownloadClient      string
	TGUploadClient        string
	Thumb                 string
	UnfinishedProgressStr string
	UploadAsVideo         bool
	UseAniList            bool
	UseCaption            bool
	UVS                   bool
	Workers               int
}

// RuntimeConfig holds runtime state
type RuntimeConfig struct {
	Aria2            interface{}
	BatchIncoming    []interface{}
	BatchQueue       map[string]interface{}
	BotID            string
	Cached           bool
	CachedDL         bool
	CustomRename     interface{}
	DisplayAddDLInfo bool
	DockerDeployed   bool
	ECancel          map[string]interface{}
	GroupEnc         bool
	GroupEncList     []interface{}
	MaxMessageLength int
	OnlyOwnerPM      bool
	PauseStatus      int
	Paused           []interface{}
	PreviewBatch     map[string]interface{}
	PreviewList      []interface{}
	Queue            map[string]interface{}
	QueueStatus      []interface{}
	RQueue           []interface{}
	ReleaseName      string
	ReleaseNameB     string
	ReportFailedDL   bool
	ReportFailedEnc  bool
	RSSDict          map[string]interface{}
	RSSRanOnce       bool
	SAS              bool
	SQS              bool
	Started          bool
	TempOnlyInGroup  bool
	TgpAuthor        string
	TgpAuthorUrl     string
	TempUsers        []interface{}
	UCancel          []interface{}
	Version2         []interface{}
}

var (
	// Conf is the loaded application config
	Conf *Config
	// RuntimeConf holds the runtime state
	Rconf *RuntimeConfig
)

var requiredEnvVars = []string{
	"API_ID",
	"API_HASH",
	"BOT_TOKEN",
}

func formatReleaseName(name string) string {
	name = strings.TrimSpace(name)
	if name != "" {
		return "[" + name + "]"
	}
	return ""
}

// LoadConfig reads .env and environment variables using viper
func LoadConfig() error {
	viper.SetConfigFile(".env")
	viper.SetConfigType("env")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	err := viper.ReadInConfig()
	if err != nil {
		log.Println("No .env file found or failed to read .env, falling back to environment variables")
	}
	// Check for missing required env vars
	for _, key := range requiredEnvVars {
		if viper.GetString(key) == "" {
			return fmt.Errorf("missing required environment variable: %s", key)
		}
	}
	// set defaults
	viper.SetDefault("ARIA2_DL_TIMEOUT", 0)
	viper.SetDefault("ALLOW_ACTION", true)
	viper.SetDefault("ARIA2_PORT", 6800)
	viper.SetDefault("CAP_DECO", "◉")
	viper.SetDefault("C_LINK", "@ANi_MiNE")
	viper.SetDefault("CMD_SUFFIX", "")
	viper.SetDefault("COMPATIBILITY_MODE", true)
	viper.SetDefault("CUSTOM_RENAME", "")
	viper.SetDefault("DBNAME", "ENC-GO")
	viper.SetDefault("DEV", 0)
	viper.SetDefault("DL_STUFF", "")
	viper.SetDefault("DUMP_LEECH", true)
	viper.SetDefault("EXTENDED_CAPTIONS", true)
	viper.SetDefault("FFMPEG", `ffmpeg -i "%s" -preset ultrafast -c:v libx265 -crf 27 -map 0:v -c:a aac -map 0:a -c:s copy -map 0:s? "%s"`)
	viper.SetDefault("FINISHED_PROGRESS_STR", "🧡")
	viper.SetDefault("FLOOD_SLEEP_THRESHOLD", 600)
	viper.SetDefault("MI_IN_CAPTION", true)
	viper.SetDefault("PAUSE_ON_DL_INFO", true)
	viper.SetDefault("QBIT_PORT", 8090)
	viper.SetDefault("QBIT_PORT2", 9090)
	viper.SetDefault("QBIT_TIMEOUT", 20)
	viper.SetDefault("RELEASER", "A-M|ANi-MiNE")
	viper.SetDefault("REPORT_FAILED", true)
	viper.SetDefault("RSS_DELAY", 60)
	viper.SetDefault("RSS_DIRECT", true)
	viper.SetDefault("TELEGRAPH_API", "https://api.telegra.ph")
	viper.SetDefault("UN_FINISHED_PROGRESS_STR", "🤍")
	viper.SetDefault("USE_ANILIST", true)
	viper.SetDefault("USE_CAPTION", true)
	viper.SetDefault("WORKERS", 2)

	Conf = &Config{
		ADLTimeout:            viper.GetInt("ARIA2_DL_TIMEOUT"),
		AlwaysDeployLatest:    viper.GetBool("ALWAYS_DEPLOY_LATEST"),
		AllowAction:           viper.GetBool("ALLOW_ACTION"),
		AppID:                 viper.GetInt("APP_ID"),
		APIHash:               viper.GetString("API_HASH"),
		Aria2Port:             viper.GetInt("ARIA2_PORT"),
		BotToken:              viper.GetString("BOT_TOKEN"),
		CacheDL:               viper.GetBool("CACHE_DL"),
		CapDeco:               viper.GetString("CAP_DECO"),
		CLink:                 viper.GetString("C_LINK"),
		CmdSuffix:             viper.GetString("CMD_SUFFIX"),
		CompatMode:            viper.GetBool("COMPATIBILITY_MODE"),
		CustomRename:          viper.GetString("CUSTOM_RENAME"),
		DatabaseURL:           viper.GetString("DATABASE_URL"),
		DBName:                viper.GetString("DBNAME"),
		Dev:                   viper.GetInt("DEV"),
		DLStuff:               viper.GetString("DL_STUFF"),
		DumpChannel:           viper.GetInt("DUMP_CHANNEL"),
		DumpLeech:             viper.GetBool("DUMP_LEECH"),
		Dyno:                  viper.GetString("DYNO"),
		Encoder:               viper.GetString("ENCODER"),
		ExtCap:                viper.GetBool("EXTENDED_CAPTIONS"),
		FBanner:               viper.GetBool("FBANNER"),
		FChannel:              viper.GetInt("FCHANNEL"),
		FChannelStat:          viper.GetInt("FCHANNEL_STAT"),
		FCodec:                viper.GetString("FCODEC"),
		FFmpeg:                viper.GetString("FFMPEG"),
		FFmpeg2:               viper.GetString("FFMPEG2"),
		FFmpeg3:               viper.GetString("FFMPEG3"),
		FFmpeg4:               viper.GetString("FFMPEG4"),
		FinishedProgressStr:   viper.GetString("FINISHED_PROGRESS_STR"),
		FlCap:                 viper.GetBool("FILENAME_AS_CAPTION"),
		FSThreshold:           viper.GetInt("FLOOD_SLEEP_THRESHOLD"),
		FSticker:              viper.GetString("FSTICKER"),
		LocalDBPath:           viper.GetString("LOCAL_DB_PATH"),
		LockOnStartup:         viper.GetBool("LOCK_ON_STARTUP"),
		LogChannel:            viper.GetInt64("LOG_CHANNEL"),
		LogsInChannel:         viper.GetBool("LOGS_IN_CHANNEL"),
		MICap:                 viper.GetBool("MI_IN_CAPTION"),
		MuxArgs:               viper.GetString("MUX_ARGS"),
		NoBanner:              viper.GetBool("NO_BANNER"),
		NoTempPM:              viper.GetBool("NO_TEMP_PM"),
		Ovr:                   viper.GetString("OVR"),
		Owner:                 viper.GetString("OWNER"),
		PauseOnDLInfo:         viper.GetBool("PODI"),
		QDLTimeout:            viper.GetInt("QBIT_DL_TIMEOUT"),
		QBitPort:              viper.GetInt("QBIT_PORT"),
		QBitPort2:             viper.GetInt("QBIT_PORT2"),
		QBitTimeout:           viper.GetInt("QBIT_TIMEOUT"),
		Releaser:              viper.GetString("RELEASER"),
		ReportFailed:          viper.GetBool("REPORT_FAILED"),
		ReportFailedDL:        viper.GetBool("REPORT_FAILED_DL"),
		ReportFailedEnc:       viper.GetBool("REPORT_FAILED_ENC"),
		RSSChat:               viper.GetString("RSS_CHAT"),
		RSSDelay:              viper.GetInt("RSS_DELAY"),
		RSSDirect:             viper.GetBool("RSS_DIRECT"),
		TelegraphAPI:          viper.GetString("TELEGRAPH_API"),
		TelegraphAuthor:       viper.GetString("TELEGRAPH_AUTHOR"),
		TempUser:              viper.GetString("TEMP_USERS"),
		TGDownloadClient:      viper.GetString("TG_DL_CLIENT"),
		TGUploadClient:        viper.GetString("TG_UL_CLIENT"),
		Thumb:                 viper.GetString("THUMBNAIL"),
		UnfinishedProgressStr: viper.GetString("UN_FINISHED_PROGRESS_STR"),
		UploadAsVideo:         viper.GetBool("UPLOAD_AS_VIDEO"),
		UseAniList:            viper.GetBool("USE_ANILIST"),
		UseCaption:            viper.GetBool("USE_CAPTION"),
		UVS:                   viper.GetBool("UPLOAD_VIDEO_AS_SPOILER"),
		Workers:               viper.GetInt("WORKERS"),
	}

	// Initialize runtime config with defaults
	Rconf = &RuntimeConfig{
		BatchQueue:   make(map[string]interface{}),
		ECancel:      make(map[string]interface{}),
		PreviewBatch: make(map[string]interface{}),
		Queue:        make(map[string]interface{}),
		RSSDict:      make(map[string]interface{}),
	}

	author := Conf.TelegraphAuthor
	if author != "" {
		parts := strings.Split(author, "|")
		if !strings.EqualFold(parts[0], "auto") {
			Rconf.TgpAuthor = parts[0]
		}
		if len(parts) > 1 && !strings.EqualFold(parts[1], "auto") {
			Rconf.TgpAuthorUrl = parts[1]
		}
	}
	releaser := Conf.Releaser
	switch {
	case strings.Contains(releaser, "|"):
		parts := strings.Split(releaser, "|")
		Rconf.ReleaseName = formatReleaseName(parts[0])
		Rconf.ReleaseNameB = formatReleaseName(parts[1])
	case strings.EqualFold(releaser, "none"):
		Rconf.ReleaseName = ""
		Rconf.ReleaseNameB = ""
	default:
		Rconf.ReleaseName = formatReleaseName(releaser)
		Rconf.ReleaseNameB = Rconf.ReleaseName
	}
	Rconf.BotID = strings.SplitN(Conf.BotToken, ":", 2)[0]

	return nil
}

//func init() {
//	if err := LoadConfig(); err != nil {
//		log.Fatalf("Failed to load config: %v", err)
//	}
//}
