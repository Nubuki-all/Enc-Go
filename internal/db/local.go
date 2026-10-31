package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"go.uber.org/zap"
	//_ "modernc.org/sqlite"

	"enc/config"
	"enc/internal/bot"
	"enc/internal/bot/types"
	"enc/internal/logger"
)

const schemaSQL = `
CREATE TABLE IF NOT EXISTS queue_records (
	bot_id TEXT PRIMARY KEY,
	queue TEXT,
	queue_keys TEXT
);

CREATE TABLE IF NOT EXISTS setting_records (
	bot_id TEXT NOT NULL,
	key TEXT NOT NULL,
	value TEXT,
	PRIMARY KEY (bot_id, key)
);

CREATE TABLE IF NOT EXISTS arr_records (
	bot_id TEXT NOT NULL,
	key TEXT NOT NULL,
	value TEXT,
	PRIMARY KEY (bot_id, key)
);

CREATE TABLE IF NOT EXISTS map_records (
	bot_id TEXT NOT NULL,
	key TEXT NOT NULL,
	value TEXT,
	PRIMARY KEY (bot_id, key)
);

CREATE TABLE IF NOT EXISTS filters (
	bot_id TEXT PRIMARY KEY,
	data TEXT,
	name_filters TEXT,
	release_filters TEXT,
	rg_extended TEXT,
	rename_filters TEXT
);
`

var localdb *sql.DB

// InitLocalDB sets up the SQLite file & tables
func InitLocalDB() error {
	path := config.Conf.LocalDBPath
	if path == "" {
		path = ".encbot.db"
	}
	var err error
	localdb, err = sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("local sqlite open failed: %w", err)
	}
	if _, err := localdb.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		logger.Error(err)
	}
	if _, err := localdb.Exec(schemaSQL); err != nil {
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

	loadQueue()
	loadSettings()
	loadArrays()
	loadRSSDict()
	loadFilters()

	logger.InfoW("SQLite state loaded")
}

func loadQueue() {
	var queueJSON, queueKeysJSON sql.NullString
	err := localdb.QueryRow(
		`SELECT queue, queue_keys FROM queue_records WHERE bot_id = ?`,
		config.Rconf.BotID,
	).Scan(&queueJSON, &queueKeysJSON)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			logger.Error(err)
		}
		return
	}

	var queueSlice []types.QueueEntry
	var queueKeysMap map[string]struct{}

	if queueJSON.Valid && queueJSON.String != "" {
		if err := json.Unmarshal([]byte(queueJSON.String), &queueSlice); err != nil {
			logger.WarnW("Failed to unmarshal queue", zap.Error(err))
			return
		}
	}
	if queueKeysJSON.Valid && queueKeysJSON.String != "" {
		if err := json.Unmarshal([]byte(queueKeysJSON.String), &queueKeysMap); err != nil {
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

func loadSettings() {
	rows, err := localdb.Query(`SELECT key, value FROM setting_records WHERE bot_id = ?`, config.Rconf.BotID)
	if err != nil {
		return
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			logger.Error(err)
			continue
		}
		switch key {
		case "cus_rename":
			bot.CustomRename = value
		case "ffmpeg":
			bot.FFmpegArgs = value
		case "ffmpeg2":
			bot.FFmpeg2Args = value
		case "ffmpeg3":
			bot.FFmpeg3Args = value
		case "ffmpeg4":
			bot.FFmpeg4Args = value
		case "mux_args":
			bot.MuxArgs = value
		}
	}
}

func loadArrays() {
	rows, err := localdb.Query(`SELECT key, value FROM arr_records WHERE bot_id = ?`, config.Rconf.BotID)
	if err != nil {
		return
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var key string
		var arr Int64Slice
		if err := rows.Scan(&key, &arr); err != nil {
			logger.Error(err)
			continue
		}
		if key == "t_users" {
			// Merge like the Mongo loader, keeping users added from TEMP_USERS.
			for _, id := range arr {
				if !slices.Contains(bot.TempUsers, id) {
					bot.TempUsers = append(bot.TempUsers, id)
				}
			}
		}
	}
}

func loadRSSDict() {
	var value string
	err := localdb.QueryRow(
		`SELECT value FROM map_records WHERE bot_id = ? AND key = ?`,
		config.Rconf.BotID, "rss",
	).Scan(&value)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			logger.Error(err)
		}
		return
	}

	var m map[string]*types.FeedData
	if err := json.Unmarshal([]byte(value), &m); err != nil {
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

func loadFilters() {
	var dataJSON, nameJSON, releaseJSON, rgJSON, renameJSON sql.NullString
	err := localdb.QueryRow(
		`SELECT data, name_filters, release_filters, rg_extended, rename_filters
		 FROM filters WHERE bot_id = ?`,
		config.Rconf.BotID,
	).Scan(&dataJSON, &nameJSON, &releaseJSON, &rgJSON, &renameJSON)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			logger.ErrorW(err.Error())
		}
		return
	}

	if dataJSON.Valid && dataJSON.String != "" {
		var d types.FilterData
		if err := json.Unmarshal([]byte(dataJSON.String), &d); err != nil {
			logger.WarnW("Failed to unmarshal filter data", zap.Error(err))
		} else {
			bot.Filters = &d
		}
	}
	if nameJSON.Valid && nameJSON.String != "" {
		if err := json.Unmarshal([]byte(nameJSON.String), &bot.NameFilters); err != nil {
			logger.WarnW("Failed to unmarshal name filters", zap.Error(err))
		}
	}
	if releaseJSON.Valid && releaseJSON.String != "" {
		if err := json.Unmarshal([]byte(releaseJSON.String), &bot.ReleaseFilters); err != nil {
			logger.WarnW("Failed to unmarshal release filters", zap.Error(err))
		}
	}
	if rgJSON.Valid && rgJSON.String != "" {
		if err := json.Unmarshal([]byte(rgJSON.String), &bot.RGExtendedFilters); err != nil {
			logger.WarnW("Failed to unmarshal rg extended filters", zap.Error(err))
		}
	}
	if renameJSON.Valid && renameJSON.String != "" {
		if err := json.Unmarshal([]byte(renameJSON.String), &bot.RenameFilters); err != nil {
			logger.WarnW("Failed to unmarshal rename filters", zap.Error(err))
		}
	}
}

// SaveQueueLocal writes the current queue to SQLite.
func SaveQueueLocal() {
	if localdb == nil {
		return
	}
	m, s, unlock := bot.Queue.LockForSave()
	defer unlock()

	if s == nil {
		s = []types.QueueEntry{}
	}
	if m == nil {
		m = map[string]struct{}{}
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

	_, err = localdb.Exec(`
		INSERT INTO queue_records (bot_id, queue, queue_keys)
		VALUES (?, ?, ?)
		ON CONFLICT(bot_id) DO UPDATE SET
			queue = excluded.queue,
			queue_keys = excluded.queue_keys
	`, config.Rconf.BotID, string(queueJSON), string(queueKeysJSON))
	if err != nil {
		logger.ErrorW(err.Error())
	}
}

// SaveSettingsLocal persists settings and ffmpeg templates
func SaveSettingsLocal() {
	if localdb == nil {
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
		_, err := localdb.Exec(`
			INSERT INTO setting_records (bot_id, key, value)
			VALUES (?, ?, ?)
			ON CONFLICT(bot_id, key) DO UPDATE SET value = excluded.value
		`, config.Rconf.BotID, k, v)
		if err != nil {
			logger.ErrorW(err.Error())
		}
	}
}

// SaveTempUsersLocal persists temp users
func SaveTempUsersLocal() {
	if localdb == nil {
		return
	}
	_, err := localdb.Exec(`
		INSERT INTO arr_records (bot_id, key, value)
		VALUES (?, ?, ?)
		ON CONFLICT(bot_id, key) DO UPDATE SET value = excluded.value
	`, config.Rconf.BotID, "t_users", bot.TempUsers)
	if err != nil {
		logger.ErrorW(err.Error())
	}
}

// SaveRSSLocal persists the RSSDict map
func SaveRSSLocal() {
	if localdb == nil {
		return
	}
	bot.RSSDictMutex.Lock()
	data, err := json.Marshal(bot.RSSDict)
	bot.RSSDictMutex.Unlock()
	if err != nil {
		logger.WarnW("could not marshal RSSDict", zap.Error(err))
		return
	}
	_, err = localdb.Exec(`
		INSERT INTO map_records (bot_id, key, value)
		VALUES (?, ?, ?)
		ON CONFLICT(bot_id, key) DO UPDATE SET value = excluded.value
	`, config.Rconf.BotID, "rss", string(data))
	if err != nil {
		logger.ErrorW(err.Error())
	}
}

// SaveFilterDataLocal, SaveRGExtendedFiltersLocal, SaveNameFiltersLocal,
// SaveReleaseFiltersLocal, and SaveRenameFiltersLocal each upsert only their
// own column of the filters row.
func SaveFilterDataLocal() {
	if localdb == nil {
		return
	}
	data, err := json.Marshal(bot.Filters)
	if err != nil {
		logger.ErrorW(err.Error())
		return
	}
	_, err = localdb.Exec(`
		INSERT INTO filters (bot_id, data)
		VALUES (?, ?)
		ON CONFLICT(bot_id) DO UPDATE SET data = excluded.data
	`, config.Rconf.BotID, string(data))
	if err != nil {
		logger.ErrorW(err.Error())
	}
}

func SaveRGExtendedFiltersLocal() {
	if localdb == nil {
		return
	}
	data, err := json.Marshal(bot.RGExtendedFilters)
	if err != nil {
		logger.ErrorW(err.Error())
		return
	}
	_, err = localdb.Exec(`
		INSERT INTO filters (bot_id, rg_extended)
		VALUES (?, ?)
		ON CONFLICT(bot_id) DO UPDATE SET rg_extended = excluded.rg_extended
	`, config.Rconf.BotID, string(data))
	if err != nil {
		logger.ErrorW(err.Error())
	}
}

func SaveNameFiltersLocal() {
	if localdb == nil {
		return
	}
	data, err := json.Marshal(bot.NameFilters)
	if err != nil {
		logger.ErrorW(err.Error())
		return
	}
	_, err = localdb.Exec(`
		INSERT INTO filters (bot_id, name_filters)
		VALUES (?, ?)
		ON CONFLICT(bot_id) DO UPDATE SET name_filters = excluded.name_filters
	`, config.Rconf.BotID, string(data))
	if err != nil {
		logger.ErrorW(err.Error())
	}
}

func SaveReleaseFiltersLocal() {
	if localdb == nil {
		return
	}
	data, err := json.Marshal(bot.ReleaseFilters)
	if err != nil {
		logger.ErrorW(err.Error())
		return
	}
	_, err = localdb.Exec(`
		INSERT INTO filters (bot_id, release_filters)
		VALUES (?, ?)
		ON CONFLICT(bot_id) DO UPDATE SET release_filters = excluded.release_filters
	`, config.Rconf.BotID, string(data))
	if err != nil {
		logger.ErrorW(err.Error())
	}
}

func SaveRenameFiltersLocal() {
	if localdb == nil {
		return
	}
	data, err := json.Marshal(bot.RenameFilters)
	if err != nil {
		logger.ErrorW(err.Error())
		return
	}
	_, err = localdb.Exec(`
		INSERT INTO filters (bot_id, rename_filters)
		VALUES (?, ?)
		ON CONFLICT(bot_id) DO UPDATE SET rename_filters = excluded.rename_filters
	`, config.Rconf.BotID, string(data))
	if err != nil {
		logger.ErrorW(err.Error())
	}
}
