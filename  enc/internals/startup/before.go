package startup

import (
    "io"
    "net/http"
    "os"
    "go.uber.org/zap"
    "enc/config"
    "enc/internal/bot"
    "enc/internal/db"
    "enc/internal/bot/entimer"
)

func Initialize() {
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
        zap.L().Info("Docker: Yes")
    }
}


func DownloadThumbnail() {
    thumbURL := config.Conf.Thumb
    if thumbURL == "" {
        return
    }

    resp, err := http.Get(thumbURL)
    if err != nil {
        zap.L().Warn("Failed to download thumbnail", zap.Error(err))
        return
    }
    defer resp.Body.Close()

    out, err := os.Create(".thumb.jpg")
    if err != nil {
        zap.L().Warn("Failed to create thumbnail file", zap.Error(err))
        return
    }
    defer out.Close()

    _, err = io.Copy(out, resp.Body)
    if err != nil {
        zap.L().Warn("Failed to write thumbnail file", zap.Error(err))
    } else {
        zap.L().Info("Downloaded thumbnail to .thumb.jpg")
    }
}

func SetupCustomRename() {
	if v := config.Conf.CustomRename; v != "" {
		bot.CustomRename = v
		zap.L().Info("Custom rename enabled", zap.String("template", v))
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
			zap.L().Warn("Failed to download link", zap.String("url", link), zap.Error(err))
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
			zap.L().Warn("Cannot create file", zap.String("file", fn), zap.Error(err))
			continue
		}
		_, err = io.Copy(out, resp.Body)
		out.Close()
		if err != nil {
			zap.L().Warn("Failed writing file", zap.String("file", fn), zap.Error(err))
		} else {
			zap.L().Info("Downloaded link", zap.String("url", link), zap.String("file", fn))
		}
	}
}


func SetupFlags() {
	if config.Conf.NoTempPM {
		bot.TempOnlyInGroup = true
		zap.L().Info("Only allow PM in groups")
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
			zap.L().Warn("Could not create dir", zap.String("dir", d), zap.Error(err))
		}
	}
}

func AddTempUsers() {
	raw := config.Conf.TempUser
	if raw == "" {
		return
	}
	owners := strings.Fields(config.Conf.Owner)
	for _, t := range strings.Fields(raw) {
		if contains(owners, t) || contains(bot.TempUsers, t) {
			continue
		}
		bot.TempUsers = append(bot.TempUsers, t)
		zap.L().Info("Added temp user", zap.String("user", t))
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
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    if err := db.InitMongo(ctx); err != nil {
        zap.L().Warn("database initialization failed, falling back to local", zap.Error(err))
    }

    // 2) If we didn’t connect to Mongo, use local loader and return
    if !db.UseMongo() {
        zap.L().Info("Using local DB fallback")
        db.LoadLocalDB()
        return
    }

    // 3) Otherwise, load persisted state into bot.* variables
    zap.L().Info("Loading state from MongoDB")
    db.loadMap(db.QueueCol, "batches", &bot.BatchQueue)
    db.LoadQueue()
    db.loadMap(db.RSSCol,   "rss",     &bot.RSSDict)

    db.loadSlice(db.UsersCol, "t_users", &bot.TempUsers)

    db.loadString(db.FilterCol, "autoname",   &bot.Autoname)
    db.loadString(db.FilterCol, "cus_rename", &bot.CustomRename)

    db.loadString(db.CodeCol, "ffmpeg",   &bot.FFmpegArgs)
    db.loadString(db.CodeCol, "ffmpeg2",  &bot.FFmpeg2Args)
    db.loadString(db.CodeCol, "ffmpeg3",  &bot.FFmpeg3Args)
    db.loadString(db.CodeCol, "ffmpeg4",  &bot.FFmpeg4Args)
    db.loadString(db.CodeCol, "mux_args", &bot.MuxArgs)

    zap.L().Info("startup: state loaded from database")
}

