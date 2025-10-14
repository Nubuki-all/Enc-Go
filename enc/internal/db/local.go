package db

import (
	"encoding/json"
	"errors"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"moul.io/zapgorm2"

	"enc/config"
	"enc/internal/bot"
	"enc/internal/bot/types"
	"enc/internal/logger"
)

// ————————————————————————————————
// GORM models for SQL fallback
// ————————————————————————————————

// queueRecord stores the ordered queue as JSON
type queueRecord struct {
	BotID     string         `gorm:"primaryKey"`
	Queue     datatypes.JSON `gorm:"type:JSON"`
	QueueKeys datatypes.JSON `gorm:"type:JSON"`
}

/*type queueRecord struct {
	BotID string             `gorm:"primaryKey"`
	Queue []types.QueueEntry `gorm:"type:TEXT"`
}*/

// settingRecord stores simple string settings
type settingRecord struct {
	BotID string `gorm:"primaryKey"`
	Key   string `gorm:"primaryKey"`
	Value string
}

// arrRecord stores slices of strings as JSON
type arrRecord struct {
	BotID string     `gorm:"primaryKey"`
	Key   string     `gorm:"primaryKey"`
	Value Int64Slice `gorm:"type:json"`
}

// mapRecord stores arbitrary maps (e.g. RSSDict) as JSON
type mapRecord struct {
	BotID string `gorm:"primaryKey"`
	Key   string `gorm:"primaryKey"`
	Value string // JSON-serialized map[string]interface{}
}

// filterRecord holds all filter settings in one JSON column.
type filterRecord struct {
	BotID          string                        `gorm:"column:bot_id;primaryKey"`
	Data           *types.FilterData             `gorm:"column:data;type:JSON"` // JSON column
	NameFilters    []*types.MatchedFilterData    `gorm:"column:name_filters;type:JSON"`
	ReleaseFilters []*types.MatchedFilterData    `gorm:"column:release_filters;type:JSON"`
	RGExtended     []*types.RGExtendedFilterData `gorm:"column:rg_extended;type:JSON"`
	RenameFilters  []*types.RenameFilterData     `gorm:"column:rename_filters;type:JSON"`
}

// TableName ensures GORM uses your `filters` table.
func (filterRecord) TableName() string {
	return "filters"
}

var localdb *gorm.DB

// InitLocalDB sets up the SQLite file & tables
func InitLocalDB() error {
	path := config.Conf.LocalDBPath
	if path == "" {
		path = ".encbot.db"
	}
	var err error
	glogger := zapgorm2.New(logger.Logger)
	glogger.IgnoreRecordNotFoundError = true
	localdb, err = gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: glogger,
	})
	if err != nil {
		return fmt.Errorf("local sqlite open failed: %w", err)
	}
	// Use WAL mode for better concurrency
	sqlDB, _ := localdb.DB()
	_, err = sqlDB.Exec("PRAGMA journal_mode=WAL;")
	if err != nil {
		logger.Error(err)
	}

	// Auto-migrate schemas
	err = localdb.AutoMigrate(&queueRecord{}, &settingRecord{}, &arrRecord{}, &mapRecord{}, &filterRecord{})
	if err != nil {
		return fmt.Errorf("sqlite migrate: %w", err)
	}
	return nil
}

// LoadLocalDB is called during startup to hydrate bot state
func LoadLocalDB() {
	logger.InfoW("Initializing SQLite fallback...")
	if err := InitLocalDB(); err != nil {
		logger.WarnW("LocalDB init error", zap.Error(err))
		return
	}

	// Load queue
	var qr queueRecord
	if err := localdb.First(&qr, "bot_id = ?", config.Rconf.BotID).Error; nil == err {
		var queueSlice []types.QueueEntry
		var queueKeysMap map[string]struct{}

		if len(qr.Queue) > 0 {
			if err := json.Unmarshal(qr.Queue, &queueSlice); err != nil {
				logger.WarnW("Failed to unmarshal queue", zap.Error(err))
				return
			}
		}
	
		// Check if the JSON keys data is empty before unmarshaling.
		if len(qr.QueueKeys) > 0 {
			if err := json.Unmarshal(qr.QueueKeys, &queueKeysMap); err != nil {
				logger.WarnW("Failed to unmarshal queue keys", zap.Error(err))
				return
			}
		}

		sLen, mLen := len(queueSlice), len(queueKeysMap)
		if sLen > 0 && mLen > 0 {
			if sLen != mLen {
				logger.Warn("Length of keys is different from Length of entries")
				return
			}
			bot.Queue.LoadFromMapSlice(queueKeysMap, queueSlice)
			logger.InfoW("Loaded queue from SQLite", zap.Int("count", len(queueSlice)))
		}
	}

	// Load settings
	var settings []settingRecord
	if err := localdb.Find(&settings, "bot_id = ?", config.Rconf.BotID).Error; err == nil {
		for _, r := range settings {
			switch r.Key {
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
	if err := localdb.Find(&arrs, "bot_id = ?", config.Rconf.BotID).Error; err == nil {
		for _, a := range arrs {
			if a.Key == "t_users" {
				bot.TempUsers = a.Value
			}
		}
	}

	// Load maps (e.g. RSSDict)
	loadRSSDict := func() {
		var rec mapRecord
		err := localdb.First(&rec, "bot_id = ? AND key = ?", config.Rconf.BotID, "rss").Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// nothing saved yet
				return
			}
			logger.Error(err)
			return
		}

		var m map[string]*types.FeedData
		if err := json.Unmarshal([]byte(rec.Value), &m); err != nil {
			logger.WarnW("LoadRSSLocal: could not unmarshal rss json", zap.Error(err))
			return
		}
		if m == nil {
			return
		}
		bot.RSSDictMutex.Lock()
		bot.RSSDict = m
		bot.RSSDictMutex.Unlock()
	}
	loadRSSDict()

	var rec filterRecord
	err := localdb.First(&rec, "bot_id = ?", config.Rconf.BotID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return
		}
		logger.ErrorW(err.Error())
		return
	}
	bot.Filters = rec.Data
	bot.RGExtendedFilters = rec.RGExtended
	bot.NameFilters = rec.NameFilters
	bot.ReleaseFilters = rec.ReleaseFilters
	bot.RenameFilters = rec.RenameFilters
	logger.InfoW("SQLite state loaded")
}

// SaveQueueLocal writes current queue to SQLite
func SaveQueueLocal() {
	if config.Conf.DatabaseURL != "" || localdb == nil {
		return
	}
	m, s, unlock := bot.Queue.LockForSave()
	defer unlock()

	if len(s) == 0 {
		// No data to save, so return early.
		return
	}
	queueJSON, err := json.Marshal(s)
	if err != nil {
		logger.WarnW("Failed to marshal queue slice to JSON", zap.Error(err))
		return
	}

	queueKeysJSON, err := json.Marshal(m)
	if err != nil {
		logger.WarnW("Failed to marshal queue keys to JSON", zap.Error(err))
		return
	}

	obj := queueRecord{
		BotID:     config.Rconf.BotID,
		Queue:     datatypes.JSON(queueJSON),
		QueueKeys: datatypes.JSON(queueKeysJSON),
	}

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
		"cus_rename": bot.CustomRename,
		"ffmpeg":     bot.FFmpegArgs,
		"ffmpeg2":    bot.FFmpeg2Args,
		"ffmpeg3":    bot.FFmpeg3Args,
		"ffmpeg4":    bot.FFmpeg4Args,
		"mux_args":   bot.MuxArgs,
	}
	for k, v := range toSave {
		rec := settingRecord{BotID: config.Rconf.BotID, Key: k, Value: v}
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
	rec := arrRecord{BotID: config.Rconf.BotID, Key: "t_users", Value: bot.TempUsers}
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
	bot.RSSDictMutex.Lock()
	data, err := json.Marshal(bot.RSSDict)
	bot.RSSDictMutex.Unlock()
	if err != nil {
		logger.WarnW("could not marshal RSSDict", zap.Error(err))
		return
	}
	rec := mapRecord{BotID: config.Rconf.BotID, Key: "rss", Value: string(data)}
	localdb.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "bot_id"}, {Name: "key"}},
		UpdateAll: true,
	}).Create(&rec)
}

// SaveBatchQueueLocal persists bot.BatchQueue to SQLite
func SaveBatchQueueLocal() error {
	if config.Conf.DatabaseURL != "" || localdb == nil {
		return nil
	}
	// store BatchQueue as JSON in the same queueRecord table under key "batches"
	// For simplicity, reuse settingRecord schema to store map[string]interface{} as JSON:
	data, err := json.Marshal(bot.BatchQueue)
	if err != nil {
		return err
	}
	rec := settingRecord{
		BotID: config.Rconf.BotID,
		Key:   "batches",
		Value: string(data),
	}
	return localdb.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "bot_id"}, {Name: "key"}},
		UpdateAll: true,
	}).Create(&rec).Error
}

// SaveFilterDataLocal writes the entire FilterData struct into one JSON column.
// On conflict (same bot_id), it updates the row.
func SaveFilterDataLocal() {
	rec := filterRecord{
		BotID: config.Rconf.BotID,
		Data:  bot.Filters,
	}
	err := localdb.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "bot_id"}},
		UpdateAll: true,
	}).
		Create(&rec).
		Error
	if err != nil {
		logger.ErrorW(err.Error())
	}
}

func SaveRGExtendedFiltersLocal() {
	rec := filterRecord{
		BotID:      config.Rconf.BotID,
		RGExtended: bot.RGExtendedFilters,
	}
	err := localdb.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "bot_id"}},
		UpdateAll: true,
	}).
		Create(&rec).
		Error
	if err != nil {
		logger.ErrorW(err.Error())
	}
}

// SaveNameFiltersLocal writes the NameFilters slice.
func SaveNameFiltersLocal() {
	rec := filterRecord{
		BotID:       config.Rconf.BotID,
		NameFilters: bot.NameFilters,
	}
	err := localdb.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "bot_id"}},
		UpdateAll: true,
	}).
		Create(&rec).
		Error
	if err != nil {
		logger.ErrorW(err.Error())
	}
}

// SaveReleaseFiltersLocal writes the ReleaseFilters slice.
func SaveReleaseFiltersLocal() {
	rec := filterRecord{
		BotID:          config.Rconf.BotID,
		ReleaseFilters: bot.ReleaseFilters,
	}
	err := localdb.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "bot_id"}},
		UpdateAll: true,
	}).
		Create(&rec).
		Error
	if err != nil {
		logger.ErrorW(err.Error())
	}
}

// SaveRenameFiltersLocal writes the Rename array.
func SaveRenameFiltersLocal() {
	rec := filterRecord{
		BotID:         config.Rconf.BotID,
		RenameFilters: bot.RenameFilters,
	}
	err := localdb.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "bot_id"}},
		UpdateAll: true,
	}).
		Create(&rec).
		Error
	if err != nil {
		logger.ErrorW(err.Error())
	}
}
