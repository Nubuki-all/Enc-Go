package db

import (
	"context"
	"encoding/json"
	"fmt"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	_ "github.com/mattn/go-sqlite3"
	"gorm.io/gorm/logger"
	"go.uber.org/zap"

	"enc/internal/bot"
	"enc/internal/config"
)

// queueRecord stores the ordered queue as JSON
type queueRecord struct {
	BotID string           `gorm:"primaryKey"`
	Queue []bot.QueueEntry `gorm:"type:TEXT"`
}

// settingRecord stores simple string settings
type settingRecord struct {
	BotID string `gorm:"primaryKey"`
	Key   string `gorm:"primaryKey"`
	Value string
}

// arrRecord stores slices of strings as JSON
type arrRecord struct {
	BotID string   `gorm:"primaryKey"`
	Key   string   `gorm:"primaryKey"`
	Value []string `gorm:"type:TEXT"`
}

// mapRecord stores arbitrary maps (e.g. RSSDict) as JSON
type mapRecord struct {
	BotID string `gorm:"primaryKey"`
	Key   string `gorm:"primaryKey"`
	Value string // JSON-serialized map[string]interface{}
}

var localdb *gorm.DB

// InitLocalDB sets up the SQLite file & tables
func InitLocalDB() error {
	if config.Conf.DatabaseURL != "" {
		return nil
	}
	path := config.Conf.LocalDBPath
	if path == "" {
		path = ".encbot.db"
	}
	var err error
	localdb, err = gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("local sqlite open failed: %w", err)
	}
	// Use WAL mode for better concurrency
	sqlDB, _ := localdb.DB()
	sqlDB.Exec("PRAGMA journal_mode=WAL;")

	// Auto-migrate schemas
	err = localdb.AutoMigrate(&queueRecord{}, &settingRecord{}, &arrRecord{}, &mapRecord{})
	if err != nil {
		return fmt.Errorf("sqlite migrate: %w", err)
	}
	return nil
}

// LoadLocalDB is called during startup to hydrate bot state
func LoadLocalDB() {
	if config.Conf.DatabaseURL != "" {
		return
	}
	zap.L().Info("Initializing SQLite fallback...")
	if err := InitLocalDB(); err != nil {
		zap.L().Warn("LocalDB init error", zap.Error(err))
		return
	}

	// Load queue
	var qr queueRecord
	if err := localdb.First(&qr, "bot_id = ?", config.Conf.BotID).Error; err == nil {
		if len(qr.Queue) > 0 {
			bot.Queue.LoadFromSlice(qr.Queue)
			zap.L().Info("Loaded queue from SQLite", zap.Int("count", len(qr.Queue)))
		}
	}

	// Load settings
	var settings []settingRecord
	if err := localdb.Find(&settings, "bot_id = ?", config.Conf.BotID).Error; err == nil {
		for _, r := range settings {
			switch r.Key {
			case "autoname":
				bot.Autoname = r.Value
			case "cus_rename":
				bot.CustomRename = r.Value
			case "ffmpeg":
				bot.FFmpegArgs = r.Value
			case "ffmpeg2":
				bot.FFmpeg2Args = r.Value
			case "ffmpeg3":
				bot.FFmpeg3Args = r.Value
			case "ffmpeg4":
				bot.FFmpeg4Args = r.Value
			case "mux_args":
				bot.MuxArgs = r.Value
			}
		}
	}

	// Load arrays (e.g. temp users)
	var arrs []arrRecord
	if err := localdb.Find(&arrs, "bot_id = ?", config.Conf.BotID).Error; err == nil {
		for _, a := range arrs {
			if a.Key == "t_users" {
				bot.TempUsers = a.Value
			}
		}
	}

	// Load maps (e.g. RSSDict)
	var maps []mapRecord
	if err := localdb.Find(&maps, "bot_id = ?", config.Conf.BotID).Error; err == nil {
		for _, m := range maps {
			if m.Key == "rss" {
				var mp map[string]interface{}
				if err := json.Unmarshal([]byte(m.Value), &mp); err == nil {
					bot.RSSDict = mp
				}
			}
		}
	}

	zap.L().Info("SQLite state loaded")
}

// SaveQueueLocal writes current queue to SQLite
func SaveQueueLocal() {
	if config.Conf.DatabaseURL != "" || localdb == nil {
		return
	}
	obj := queueRecord{BotID: config.Conf.BotID, Queue: bot.Queue.AsSlice()}
	localdb.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "bot_id"}},
		UpdateAll: true,
	}).Create(&obj)
}

// SaveSettingsLocal persists settings and ffmpeg templates
func SaveSettingsLocal() {
	if config.Conf.DatabaseURL != "" || localdb == nil {
		return
	}
	toSave := map[string]string{
		"autoname":   bot.Autoname,
		"cus_rename": bot.CustomRename,
		"ffmpeg":     bot.FFmpegArgs,
		"ffmpeg2":    bot.FFmpeg2Args,
		"ffmpeg3":    bot.FFmpeg3Args,
		"ffmpeg4":    bot.FFmpeg4Args,
		"mux_args":   bot.MuxArgs,
	}
	for k, v := range toSave {
		rec := settingRecord{BotID: config.Conf.BotID, Key: k, Value: v}
		localdb.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "bot_id"}, {Name: "key"}},
			UpdateAll: true,
		}).Create(&rec)
	}
}

// SaveTempUsersLocal persists temp users
func SaveTempUsersLocal() {
	if config.Conf.DatabaseURL != "" || localdb == nil {
		return
	}
	rec := arrRecord{BotID: config.Conf.BotID, Key: "t_users", Value: bot.TempUsers}
	localdb.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "bot_id"}, {Name: "key"}},
		UpdateAll: true,
	}).Create(&rec)
}

// SaveRSSLocal persists the RSSDict map
func SaveRSSLocal() {
	if config.Conf.DatabaseURL != "" || localdb == nil {
		return
	}
	data, err := json.Marshal(bot.RSSDict)
	if err != nil {
		zap.L().Warn("could not marshal RSSDict", zap.Error(err))
		return
	}
	rec := mapRecord{BotID: config.Conf.BotID, Key: "rss", Value: string(data)}
	localdb.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "bot_id"}, {Name: "key"}},
		UpdateAll: true,
	}).Create(&rec)
}

// SaveBatchQueueLocal persists bot.BatchQueue to SQLite
func SaveBatchQueueLocal() error {
    if config.Conf.DatabaseURL != "" || db == nil {
        return nil
    }
    // store BatchQueue as JSON in the same queueRecord table under key "batches"
    // For simplicity, reuse settingRecord schema to store map[string]interface{} as JSON:
    data, err := json.Marshal(bot.BatchQueue)
    if err != nil {
        return err
    }
    rec := settingRecord{
        BotID: config.Conf.BotID,
        Key:   "batches",
        Value: string(data),
    }
    return db.Clauses(clause.OnConflict{
        Columns:   []clause.Column{{Name: "bot_id"}, {Name: "key"}},
        UpdateAll: true,
    }).Create(&rec).Error
}