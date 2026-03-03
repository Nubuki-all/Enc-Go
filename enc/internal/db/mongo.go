// Package db contains database related functions
package db

import (
	"context"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"

	"enc/config"
	"enc/internal/bot"
	"enc/internal/bot/types"
	"enc/internal/logger"
)

var (
	// Client is the shared MongoDB client
	Client *mongo.Client

	// Collections
	QueueCol  *mongo.Collection
	CodeCol   *mongo.Collection
	FilterCol *mongo.Collection
	RSSCol    *mongo.Collection
	UsersCol  *mongo.Collection

	// useMongo toggles between MongoDB and local fallback
	useMongo bool
)

func UseMongo() bool {
	return useMongo
}

// InitMongo establishes the MongoDB connection (if configured) and
// initializes all collection handles. If DATABASE_URL is unset or
// the server cannot be reached, it falls back to non-Mongo behavior.
func InitMongo(ctx context.Context) error {
	// If no URL provided, skip Mongo entirely
	if config.Conf.DatabaseURL == "" {
		logger.InfoW("DATABASE_URL unset — using local DB fallback")
		useMongo = false
		return nil
	}

	// Try to connect to MongoDB with a short timeout
	opts := options.Client().
		ApplyURI(config.Conf.DatabaseURL).
		SetServerSelectionTimeout(30 * time.Second)

	client, err := mongo.Connect(ctx, opts)
	if err != nil {
		logger.WarnW("MongoDB connection failed — falling back to local DB", zap.Error(err))
		useMongo = false
		return nil
	}

	// Ping the server to verify connectivity
	pingCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		logger.WarnW("MongoDB ping failed — falling back to local DB", zap.Error(err))
		useMongo = false
		return nil
	}

	// On success, store the client and collection handles
	Client = client
	database := client.Database(config.Conf.DBName)
	QueueCol = database.Collection("queue")
	CodeCol = database.Collection("code")
	FilterCol = database.Collection("filter")
	RSSCol = database.Collection("rss")
	UsersCol = database.Collection("users")

	useMongo = true
	logger.InfoW("MongoDB initialized successfully")
	return nil
}

// LoadQueue loads the ordered QueueEntry slice into bot.Queue
func LoadQueue() {
	if !useMongo || QueueCol == nil {
		return
	}
	doc := QueueCol.FindOne(context.Background(), bson.M{"_id": config.Rconf.BotID})
	var result struct {
		Queue     []types.QueueEntry  `bson:"queue"`
		QueueKeys map[string]struct{} `bson:"queueKeys"`
	}
	if err := doc.Decode(&result); err != nil {
		if err != mongo.ErrNoDocuments {
			logger.WarnW("LoadQueue decode error", zap.Error(err))
		}
		return
	}
	mLen := len(result.QueueKeys)
	sLen := len(result.Queue)
	if mLen != sLen || mLen == 0 {
		return
	}
	bot.Queue.LoadFromMapSlice(result.QueueKeys, result.Queue)
	logger.InfoW("Queue loaded from MongoDB", zap.Int("count", len(result.Queue)))
}

// LoadMap reads a sub-document under key into the target map
func LoadMap(coll *mongo.Collection, key string, target *map[string]any) {
	if !useMongo {
		return
	}
	doc := coll.FindOne(context.Background(), bson.M{"_id": config.Rconf.BotID})
	var result bson.M
	if err := doc.Decode(&result); err != nil {
		if err != mongo.ErrNoDocuments {
			logger.WarnW("loadMap decode error", zap.Error(err), zap.String("key", key))
		}
		return
	}

	if sub, ok := result[key].(bson.M); ok {
		for k, v := range sub {
			(*target)[k] = v
		}
	}
}

func LoadRSSDict() {
	if !useMongo {
		return
	}

	filter := bson.M{"_id": config.Rconf.BotID}
	// Only fetch the rss subdocument
	opts := options.FindOne().SetProjection(bson.M{"rss": 1})

	// decode directly into a struct with the exact expected type
	var doc struct {
		RSS map[string]*types.FeedData `bson:"rss"`
	}

	if err := RSSCol.FindOne(context.Background(), filter, opts).Decode(&doc); err != nil {
		if err == mongo.ErrNoDocuments {
			return
		}
		logger.Errorf("FindOne decode: %w", err)
	}
	// swap in under lock
	bot.RSSDictMutex.Lock()
	bot.RSSDict = doc.RSS
	bot.RSSDictMutex.Unlock()
}

// LoadSlice reads an array of strings under key into the target slice
func LoadSlice(coll *mongo.Collection, key string, target *[]int64) {
	if !useMongo {
		return
	}
	doc := coll.FindOne(context.Background(), bson.M{"_id": config.Rconf.BotID})
	var result bson.M
	if err := doc.Decode(&result); err != nil {
		return
	}

	if arr, ok := result[key].(bson.A); ok {
		for _, v := range arr {
			if s, ok := v.(int64); ok {
				if slices.Contains(*target, s) {
					continue
				}
				*target = append(*target, s)
			}
		}
	}
}

// LoadString reads a string under key into the target string pointer
func LoadString(coll *mongo.Collection, key string, target *string) {
	if !useMongo {
		return
	}
	doc := coll.FindOne(context.Background(), bson.M{"_id": config.Rconf.BotID})
	var result bson.M
	if err := doc.Decode(&result); err != nil {
		return
	}

	if s, ok := result[key].(string); ok {
		*target = s
	}
}

// LoadFilterData reads FilterData from Mongo in the single‐document style
// that your SaveFilterData currently writes.
func LoadFilterData(coll *mongo.Collection) {
	if !useMongo {
		return
	}

	// Wrapper type to match the stored shape
	var doc struct {
		FilterData *types.FilterData `bson:"filterData"`
	}

	err := coll.
		FindOne(context.Background(), bson.M{"_id": config.Rconf.BotID}).
		Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return
		}
		logger.ErrorW(err.Error())
		return
	}
	bot.Filters = doc.FilterData
}

// LoadRGExtendedFilters reads back the “rg_extended” slice.
func LoadRGExtendedFilters(coll *mongo.Collection) {
	if !useMongo {
		return
	}
	var doc struct {
		RGExtended []*types.RGExtendedFilterData `bson:"rg_extended"`
	}
	err := coll.FindOne(context.Background(), bson.M{"_id": config.Rconf.BotID}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return
	} else if err != nil {
		logger.ErrorW(err.Error())
		return
	}
	bot.RGExtendedFilters = doc.RGExtended
}

// LoadMatchedFilters reads back the “matched_filters” slice.
func LoadMatchedFilters(coll *mongo.Collection) {
	if !useMongo {
		return
	}
	var doc struct {
		Names    []*types.MatchedFilterData `bson:"name_filters"`
		Releases []*types.MatchedFilterData `bson:"release_filters"`
	}
	err := coll.FindOne(context.Background(), bson.M{"_id": config.Rconf.BotID}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return
	} else if err != nil {
		logger.ErrorW(err.Error())
		return
	}
	bot.NameFilters = doc.Names
	bot.ReleaseFilters = doc.Releases
}

// LoadRenameFilters reads back the “rename_filters” slice.
func LoadRenameFilters(coll *mongo.Collection) {
	if !useMongo {
		return
	}
	var doc struct {
		Rename []*types.RenameFilterData `bson:"rename_filters"`
	}
	err := coll.FindOne(context.Background(), bson.M{"_id": config.Rconf.BotID}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return
	} else if err != nil {
		logger.ErrorW(err.Error())
		return
	}
	bot.RenameFilters = doc.Rename
}

func SaveAll(ctx context.Context) {
	SaveQueue(ctx)
	SaveTempUsers(ctx)
	SaveFFmpegArgs(ctx)
	SaveMuxArgs(ctx)
	SaveFilterSettings(ctx)
	SaveRSSDict(ctx)
	SaveFilterData(ctx)
	SaveRGExtendedFilters(ctx)
	SaveMatchedFilters(ctx)
	SaveRenameFilters(ctx)
}

func SaveQueue(ctx context.Context) {
	if !useMongo || QueueCol == nil {
		SaveQueueLocal()
		return
	}
	m, s, unlock := bot.Queue.LockForSave()
	defer unlock()
	filter := bson.M{"_id": config.Rconf.BotID}
	update := bson.M{
		"$set": bson.M{
			"queue":     s,
			"queueKeys": m,
		},
	}
	opts := options.Update().SetUpsert(true)
	if _, err := QueueCol.UpdateOne(ctx, filter, update, opts); err != nil {
		logger.WarnW("SaveQueue failed", zap.Error(err))
	}
}

func SaveTempUsers(ctx context.Context) {
	if !useMongo || UsersCol == nil {
		SaveTempUsersLocal()
		// if err := SaveTempUsersLocal(); err != nil {
		//	logger.WarnW("SaveTempUsersLocal failed", zap.Error(err))
		//}
		return
	}
	filter := bson.M{"_id": config.Rconf.BotID}
	update := bson.M{"$set": bson.M{"t_users": bot.TempUsers}}
	opts := options.Update().SetUpsert(true)
	if _, err := UsersCol.UpdateOne(ctx, filter, update, opts); err != nil {
		logger.WarnW("SaveTempUsers failed", zap.Error(err))
	}
}

func SaveFFmpegArgs(ctx context.Context) {
	if !useMongo || CodeCol == nil {
		SaveSettingsLocal()
		//if err := SaveSettingsLocal(); err != nil {
		//	logger.WarnW("SaveFFmpegArgsLocal failed", zap.Error(err))
		//}
		return
	}
	filter := bson.M{"_id": config.Rconf.BotID}
	update := bson.M{
		"$set": bson.M{
			"ffmpeg":  bot.FFmpegArgs,
			"ffmpeg2": bot.FFmpeg2Args,
			"ffmpeg3": bot.FFmpeg3Args,
			"ffmpeg4": bot.FFmpeg4Args,
		},
	}
	opts := options.Update().SetUpsert(true)
	if _, err := CodeCol.UpdateOne(ctx, filter, update, opts); err != nil {
		logger.WarnW("SaveFFmpegArgs failed", zap.Error(err))
	}
}

func SaveMuxArgs(ctx context.Context) {
	if !useMongo || CodeCol == nil {
		SaveSettingsLocal()
		//if err := SaveMuxArgsLocal(); err != nil {
		//	logger.WarnW("SaveMuxArgsLocal failed", zap.Error(err))
		//}
		return
	}
	filter := bson.M{"_id": config.Rconf.BotID}
	update := bson.M{"$set": bson.M{"mux_args": bot.MuxArgs}}
	opts := options.Update().SetUpsert(true)
	if _, err := CodeCol.UpdateOne(ctx, filter, update, opts); err != nil {
		logger.WarnW("SaveMuxArgs failed", zap.Error(err))
	}
}

func SaveRSSDict(ctx context.Context) {
	if !useMongo || RSSCol == nil {
		SaveRSSLocal()
		//if err := SaveRSSLocal(); err != nil {
		//	logger.WarnW("SaveRSSLocal failed", zap.Error(err))
		//}
		return
	}
	filter := bson.M{"_id": config.Rconf.BotID}
	bot.RSSDictMutex.Lock()
	update := bson.M{"$set": bson.M{"rss": bot.RSSDict}}
	opts := options.Update().SetUpsert(true)
	if _, err := RSSCol.UpdateOne(ctx, filter, update, opts); err != nil {
		logger.WarnW("SaveRSSDict failed", zap.Error(err))
	}
	bot.RSSDictMutex.Unlock()
}

// SaveFilterSettings persists bot.CustomRename.
func SaveFilterSettings(ctx context.Context) {
	if !useMongo || FilterCol == nil {
		SaveSettingsLocal()
		//if err := SaveSettingsLocal(); err != nil {
		//	logger.WarnW("SaveSettingsLocal (filters) failed", zap.Error(err))
		//}
		return
	}
	filter := bson.M{"_id": config.Rconf.BotID}
	update := bson.M{"$set": bson.M{
		"cus_rename": bot.CustomRename,
	}}
	opts := options.Update().SetUpsert(true)
	if _, err := FilterCol.UpdateOne(ctx, filter, update, opts); err != nil {
		logger.WarnW("SaveFilterSettings failed", zap.Error(err))
	}
}

// SaveFilterData persists filter settings using MongoDB or SQL based on configuration.
func SaveFilterData(ctx context.Context) {
	if !useMongo || FilterCol == nil {
		SaveFilterDataLocal()
		return
	}

	// Upsert one document, with _id = BotID and a single "filterData" field
	filter := bson.M{"_id": config.Rconf.BotID}
	update := bson.M{"$set": bson.M{
		"filterData": bot.Filters,
	}}
	opts := options.Update().SetUpsert(true)

	if _, err := FilterCol.UpdateOne(ctx, filter, update, opts); err != nil {
		logger.WarnW("SaveFilterData (Mongo) failed", zap.Error(err))
	}
}

// SaveRGExtendedFilters upserts the slice under the “rg_extended” field.
func SaveRGExtendedFilters(ctx context.Context) {
	if !useMongo || FilterCol == nil {
		SaveRGExtendedFiltersLocal()
		return
	}
	filter := bson.M{"_id": config.Rconf.BotID}
	update := bson.M{"$set": bson.M{"rg_extended": bot.RGExtendedFilters}}
	opts := options.Update().SetUpsert(true)
	if _, err := FilterCol.UpdateOne(ctx, filter, update, opts); err != nil {
		logger.WarnW("SaveRGExtendedFilters (Mongo) failed", zap.Error(err))
	}
}

// SaveMatchedFilters upserts under “matched_filters”.
func SaveMatchedFilters(ctx context.Context) {
	if !useMongo || FilterCol == nil {
		SaveNameFiltersLocal()
		SaveReleaseFiltersLocal()
		return
	}
	filter := bson.M{"_id": config.Rconf.BotID}
	update := bson.M{"$set": bson.M{
		"name_filters":    bot.NameFilters,
		"release_filters": bot.ReleaseFilters,
	}}
	opts := options.Update().SetUpsert(true)
	if _, err := FilterCol.UpdateOne(ctx, filter, update, opts); err != nil {
		logger.WarnW("SaveMatchedFilters (Mongo) failed", zap.Error(err))
	}
}

// SaveRenameFilters upserts under “rename_filters”.
func SaveRenameFilters(ctx context.Context) {
	if !useMongo || FilterCol == nil {
		SaveRenameFiltersLocal()
		return
	}
	filter := bson.M{"_id": config.Rconf.BotID}
	update := bson.M{"$set": bson.M{"rename_filters": bot.RenameFilters}}
	opts := options.Update().SetUpsert(true)
	if _, err := FilterCol.UpdateOne(ctx, filter, update, opts); err != nil {
		logger.WarnW("SaveRenameFilters (Mongo) failed", zap.Error(err))
	}
}
