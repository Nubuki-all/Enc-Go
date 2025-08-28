package startup

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"enc/config"
	"enc/internal/bot"
	"enc/internal/bot/entimer"
	"enc/internal/db"
	"enc/internal/logger"
	"go.uber.org/zap"
)

func Initialize() {
	DetectDocker()
	AddOwners()
	DownloadThumbnail()
	SetupCustomRename()
	DownloadLinks()
	SetupFlags()
	SetupFFmpegParams()
	EnsureDirectories()
	AddTempUsers()
	LoadDB()
	entimer.Timer.Start()
}

func DetectDocker() {
	if _, err := os.Stat("/tgenc"); err == nil {
		bot.DockerDeployed = true
		logger.InfoW("Docker: Yes")
	}
}

func DownloadThumbnail() {
	thumbURL := config.Conf.Thumb
	if thumbURL == "" {
		return
	}

	resp, err := http.Get(thumbURL)
	if err != nil {
		logger.WarnW("Failed to download thumbnail", zap.Error(err))
		return
	}
	defer resp.Body.Close()

	out, err := os.Create(".thumb.jpg")
	if err != nil {
		logger.WarnW("Failed to create thumbnail file", zap.Error(err))
		return
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		logger.WarnW("Failed to write thumbnail file", zap.Error(err))
	} else {
		logger.InfoW("Downloaded thumbnail to .thumb.jpg")
	}
}

func SetupCustomRename() {
	if v := config.Conf.CustomRename; v != "" {
		bot.CustomRename = v
		logger.InfoW("Custom rename enabled", zap.String("template", v))
	}
}

func DownloadLinks() {
	s := config.Conf.DLStuff
	if s == "" {
		return
	}
	for _, link := range strings.Split(s, ",") {
		link = strings.TrimSpace(link)
		if link == "" {
			continue
		}
		resp, err := http.Get(link)
		if err != nil {
			logger.WarnW("Failed to download link", zap.String("url", link), zap.Error(err))
			continue
		}
		defer resp.Body.Close()

		// derive file name from URL
		fn := filepath.Base(resp.Request.URL.Path)
		if fn == "" {
			fn = "downloaded"
		}
		out, err := os.Create(fn)
		if err != nil {
			logger.WarnW("Cannot create file", zap.String("file", fn), zap.Error(err))
			continue
		}
		_, err = io.Copy(out, resp.Body)
		out.Close()
		if err != nil {
			logger.WarnW("Failed writing file", zap.String("file", fn), zap.Error(err))
		} else {
			logger.InfoW("Downloaded link", zap.String("url", link), zap.String("file", fn))
		}
	}
}

func SetupFlags() {
	if config.Conf.NoPM {
		logger.InfoW("Will Only work for Non-owners in groups")
	}
	if config.Conf.ReportFailed {
		bot.ReportFailedDl = true
		bot.ReportFailedEnc = true
	}
	if config.Conf.ReportFailedDL {
		bot.ReportFailedDl = true
	}
	if config.Conf.ReportFailedEnc {
		bot.ReportFailedEnc = true
	}
	if config.Conf.UseAniList {
		bot.AnilistParse = true
	}
	if config.Conf.UseCaption {
		bot.CaptionParse = true
	}
}

func SetupFFmpegParams() {
	bot.FFmpegArgs = config.Conf.FFmpeg
	bot.FFmpeg2Args = config.Conf.FFmpeg2
	bot.FFmpeg3Args = config.Conf.FFmpeg3
	bot.FFmpeg4Args = config.Conf.FFmpeg4
	bot.MuxArgs = config.Conf.MuxArgs
}

func EnsureDirectories() {
	dirs := []string{
		".downloads",
		".dump",
		".encode",
		".minfo",
		".mux",
		".temp",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			logger.WarnW("Could not create dir", zap.String("dir", d), zap.Error(err))
		}
	}
}

func AddOwners() {
	var s []int64
	for _, owner := range strings.Fields(config.Conf.Owner) {
		if id, err := strconv.ParseInt(owner, 10, 64); err == nil {
			s = append(s, id)
		} else {
			logger.WarnW("Couldn't Parse owner id:- "+owner, zap.Error(err))
		}
	}
	config.Rconf.Owners = s
}

func AddTempUsers() {
	raw := config.Conf.TempUser
	if raw == "" {
		return
	}
	var id int64
	var err error
	// owners := strings.Fields(config.Conf.Owner)
	for _, t := range strings.Fields(raw) {
		if id, err = strconv.ParseInt(t, 10, 64); err != nil {
			logger.WarnW("Couldn't Parse temp_user id:- "+t, zap.Error(err))
			continue
		}
		if slices.Contains(config.Rconf.Owners, id) || slices.Contains(bot.TempUsers, id) {
			continue
		}
		bot.TempUsers = append(bot.TempUsers, id)
		logger.InfoW("Added temp user", zap.String("user", t))
	}
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func LoadDB() {
	// 1) Initialize Mongo (or switch to local fallback)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := db.InitMongo(ctx); err != nil {
		logger.WarnW("database initialization failed, falling back to local", zap.Error(err))
	}

	// 2) If we didn’t connect to Mongo, use local loader and return
	if !db.UseMongo() {
		logger.InfoW("Using local DB fallback")
		db.LoadLocalDB()
		return
	}

	// 3) Otherwise, load persisted state into bot.* variables
	logger.InfoW("Loading state from MongoDB")
	db.LoadMap(db.QueueCol, "batches", &bot.BatchQueue)
	db.LoadQueue()
	db.LoadRSSDict()
	//db.LoadMap(db.RSSCol, "rss", &bot.RSSDict)

	db.LoadSlice(db.UsersCol, "t_users", &bot.TempUsers)

	db.LoadString(db.FilterCol, "cus_rename", &bot.CustomRename)

	db.LoadString(db.CodeCol, "ffmpeg", &bot.FFmpegArgs)
	db.LoadString(db.CodeCol, "ffmpeg2", &bot.FFmpeg2Args)
	db.LoadString(db.CodeCol, "ffmpeg3", &bot.FFmpeg3Args)
	db.LoadString(db.CodeCol, "ffmpeg4", &bot.FFmpeg4Args)
	db.LoadString(db.CodeCol, "mux_args", &bot.MuxArgs)

	db.LoadFilterData(db.FilterCol)
	db.LoadRGExtendedFilters(db.FilterCol)
	db.LoadMatchedFilters(db.FilterCol)
	db.LoadRenameFilters(db.FilterCol)

	logger.InfoW("startup: State loaded from mongodb database")
}
