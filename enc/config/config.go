package config

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds application configuration loaded from .env
type Config struct {
	ADLTimeout            int
	AlwaysDeployLatest    bool
	AllowAction           bool
	AltDownloadDir        string
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
	Dev                   int64
	DLStuff               string
	DownloadDir           string
	DumpChannel           int
	DumpDir               string
	DumpLeech             bool
	Dyno                  string
	EncodeDir             string
	Encoder               string
	ExtCap                bool
	FBanner               bool
	FChannel              int64
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
	JsonLog               bool
	LocalDBPath           string
	LockOnStartup         bool
	LogChannel            int64
	LogsInChannel         bool
	LogFileName           string
	MediaInfoDir          string
	MICap                 bool
	MuxArgs               string
	MuxDir                string
	NoBanner              bool
	NoPM                  bool
	Ovr                   string
	Owner                 string
	PauseOnDLInfo         bool
	ProgresUpdateInterval int
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
	TempDir               string
	TempUser              string
	TgUlTimeout           int
	Thumb                 string
	UnfinishedProgressStr string
	UploadAsDoc           bool
	UseAniList            bool
	UseCaption            bool
	UVS                   bool
	Workers               int
}

// RuntimeConfig holds runtime state
type RuntimeConfig struct {
	ADLTimeout            time.Duration
	BotID                 string
	Error                 error
	Owners                []int64
	ProgresUpdateInterval time.Duration
	QBitTimeout           time.Duration
	QDLTimeout            time.Duration
	ReleaseName           string
	ReleaseNameB          string
	Started               bool
	TgpAuthor             string
	TgpAuthorUrl          string
	TgUlTimeout           time.Duration
}

var (
	// Conf is the loaded application config
	Conf *Config
	// RuntimeConf holds the runtime state
	Rconf *RuntimeConfig
)

var requiredEnvVars = []string{
	"APP_ID",
	"API_HASH",
	"BOT_TOKEN",
	"OWNER",
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
	viper.AddConfigPath(".")
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
	viper.SetDefault("ALT_DOWNLOAD_DIR", ".downloads2")
	viper.SetDefault("ARIA2_PORT", 6800)
	viper.SetDefault("CAP_DECO", "◉")
	viper.SetDefault("C_LINK", "@ANi_MiNE")
	viper.SetDefault("CMD_SUFFIX", "")
	viper.SetDefault("COMPATIBILITY_MODE", true)
	viper.SetDefault("CUSTOM_RENAME", "")
	viper.SetDefault("DBNAME", "ENC-GO")
	viper.SetDefault("DEV", 0)
	viper.SetDefault("DL_STUFF", "")
	viper.SetDefault("DOWNLOAD_DIR", ".downloads")
	viper.SetDefault("DUMP_DIR", ".dump")
	viper.SetDefault("DUMP_LEECH", true)
	viper.SetDefault("ENCODE_DIR", ".encode")
	viper.SetDefault("EXTENDED_CAPTIONS", true)
	viper.SetDefault("FFMPEG", `ffmpeg -i """{input}""" -preset ultrafast -c:v libx265 -crf 27 -map 0:v -c:a aac -map 0:a -c:s copy -map 0:s? """{output}"""`)
	// viper.SetDefault("FINISHED_PROGRESS_STR", "🧡")
	viper.SetDefault("FLOOD_SLEEP_THRESHOLD", 600)
	viper.SetDefault("LOG_FILE_NAME", "logs.txt")
	viper.SetDefault("MI_IN_CAPTION", true)
	viper.SetDefault("MEDIAINFO_DIR", ".minfo")
	viper.SetDefault("MUX_DIR", ".mux")
	viper.SetDefault("PAUSE_ON_DL_INFO", true)
	viper.SetDefault("PROGRESS_UPDATE_INTERVAL", 10)
	viper.SetDefault("QBIT_PORT", 8090)
	viper.SetDefault("QBIT_PORT2", 9090)
	viper.SetDefault("QBIT_TIMEOUT", 20)
	viper.SetDefault("RELEASER", "A-M|ANi-MiNE")
	viper.SetDefault("REPORT_FAILED", true)
	viper.SetDefault("RSS_DELAY", 60)
	viper.SetDefault("RSS_DIRECT", true)
	viper.SetDefault("TELEGRAPH_API", "https://api.telegra.ph")
	viper.SetDefault("TEMP_DIR", ".temp")
	viper.SetDefault("UN_FINISHED_PROGRESS_STR", "🤍")
	viper.SetDefault("UPLOAD_AS_DOCUMENT", true)
	viper.SetDefault("USE_ANILIST", true)
	viper.SetDefault("USE_CAPTION", true)
	viper.SetDefault("WORKERS", 2)

	Conf = &Config{
		ADLTimeout:            viper.GetInt("ARIA2_DL_TIMEOUT"),
		AlwaysDeployLatest:    viper.GetBool("ALWAYS_DEPLOY_LATEST"),
		AllowAction:           viper.GetBool("ALLOW_ACTION"),
		AltDownloadDir:        viper.GetString("ALT_DOWNLOAD_DIR"),
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
		Dev:                   viper.GetInt64("DEV"),
		DLStuff:               viper.GetString("DL_STUFF"),
		DownloadDir:           viper.GetString("DOWNLOAD_DIR"),
		DumpChannel:           viper.GetInt("DUMP_CHANNEL"),
		DumpDir:               viper.GetString("DUMP_DIR"),
		DumpLeech:             viper.GetBool("DUMP_LEECH"),
		Dyno:                  viper.GetString("DYNO"),
		EncodeDir:             viper.GetString("ENCODE_DIR"),
		Encoder:               viper.GetString("ENCODER"),
		ExtCap:                viper.GetBool("EXTENDED_CAPTIONS"),
		FBanner:               viper.GetBool("FBANNER"),
		FChannel:              viper.GetInt64("FCHANNEL"),
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
		JsonLog:               viper.GetBool("JSON_LOGFILE"),
		LocalDBPath:           viper.GetString("LOCAL_DB_PATH"),
		LockOnStartup:         viper.GetBool("LOCK_ON_STARTUP"),
		LogChannel:            viper.GetInt64("LOG_CHANNEL"),
		LogsInChannel:         viper.GetBool("LOGS_IN_CHANNEL"),
		LogFileName:           viper.GetString("LOG_FILE_NAME"),
		MediaInfoDir:          viper.GetString("MEDIAINFO_DIR"),
		MICap:                 viper.GetBool("MI_IN_CAPTION"),
		MuxArgs:               viper.GetString("MUX_ARGS"),
		MuxDir:                viper.GetString("MUX_DIR"),
		NoBanner:              viper.GetBool("NO_BANNER"),
		NoPM:                  viper.GetBool("NO_TEMP_PM"),
		Ovr:                   viper.GetString("OVR"),
		Owner:                 viper.GetString("OWNER"),
		PauseOnDLInfo:         viper.GetBool("PODI"),
		ProgresUpdateInterval: viper.GetInt("PROGRESS_UPDATE_INTERVAL"),
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
		TempDir:               viper.GetString("TEMP_DIR"),
		TempUser:              viper.GetString("TEMP_USERS"),
		TgUlTimeout:           viper.GetInt("TG_UL_TIMEOUT"),
		Thumb:                 viper.GetString("THUMBNAIL"),
		UnfinishedProgressStr: viper.GetString("UN_FINISHED_PROGRESS_STR"),
		UploadAsDoc:           viper.GetBool("UPLOAD_AS_DOCUMENT"),
		UseAniList:            viper.GetBool("USE_ANILIST"),
		UseCaption:            viper.GetBool("USE_CAPTION"),
		UVS:                   viper.GetBool("UPLOAD_VIDEO_AS_SPOILER"),
	}

	Rconf = &RuntimeConfig{}
	if !strings.HasSuffix(Conf.TelegraphAPI, "/") {
		Conf.TelegraphAPI += "/"
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

	if Conf.ProgresUpdateInterval < 2 {
		Conf.ProgresUpdateInterval = 2
	}
	Rconf.ADLTimeout = time.Duration(Conf.ADLTimeout)
	Rconf.ProgresUpdateInterval = time.Duration(Conf.ProgresUpdateInterval)
	Rconf.QBitTimeout = time.Duration(Conf.QBitTimeout)
	Rconf.QDLTimeout = time.Duration(Conf.QDLTimeout)
	Rconf.TgUlTimeout = time.Duration(Conf.TgUlTimeout)
	return nil
}

func init() {
	if err := LoadConfig(); err != nil {
		Conf = &Config{
			LogFileName: "logs.txt",
		}
		Rconf = &RuntimeConfig{
			Error: err,
		}
	}
}
