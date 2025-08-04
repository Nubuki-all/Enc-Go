package db

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"

	"enc/config"
	"enc/internal/bot"
	"enc/internal/bot/queue"
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
		zap.L().Info("DATABASE_URL unset — using local DB fallback")
		useMongo = false
		return nil
	}

	// Try to connect to MongoDB with a short timeout
	opts := options.Client().
		ApplyURI(config.Conf.DatabaseURL).
		SetServerSelectionTimeout(5 * time.Second)

	client, err := mongo.Connect(ctx, opts)
	if err != nil {
		zap.L().Warn("MongoDB connection failed — falling back to local DB", zap.Error(err))
		useMongo = false
		return nil
	}

	// Ping the server to verify connectivity
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		zap.L().Warn("MongoDB ping failed — falling back to local DB", zap.Error(err))
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
	zap.L().Info("MongoDB initialized successfully")
	return nil
}

// LoadQueue loads the ordered QueueEntry slice into bot.Queue
func LoadQueue() {
	if !useMongo || QueueCol == nil {
		return
	}
	doc := QueueCol.FindOne(context.Background(), bson.M{"_id": config.Rconf.BotID})
	var result struct {
		Queue []queue.QueueEntry `bson:"queue"`
	}
	if err := doc.Decode(&result); err != nil {
		if err != mongo.ErrNoDocuments {
			zap.L().Warn("LoadQueue decode error", zap.Error(err))
		}
		return
	}
	bot.Queue.LoadFromSlice(result.Queue)
	zap.L().Info("Queue loaded from MongoDB", zap.Int("count", len(result.Queue)))
}

// loadMap reads a sub-document under key into the target map
func LoadMap(coll *mongo.Collection, key string, target *map[string]interface{}) {
	if !useMongo {
		return
	}
	doc := coll.FindOne(context.Background(), bson.M{"_id": config.Rconf.BotID})
	var result bson.M
	if err := doc.Decode(&result); err != nil {
		if err != mongo.ErrNoDocuments {
			zap.L().Warn("loadMap decode error", zap.Error(err), zap.String("key", key))
		}
		return
	}

	if sub, ok := result[key].(bson.M); ok {
		for k, v := range sub {
			(*target)[k] = v
		}
	}
}

// loadSlice reads an array of strings under key into the target slice
func LoadSlice(coll *mongo.Collection, key string, target *[]string) {
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
			if s, ok := v.(string); ok {
				*target = append(*target, s)
			}
		}
	}
}

// loadString reads a string under key into the target string pointer
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

func SaveAll(ctx context.Context) {
	SaveQueue(ctx)
	SaveBatchQueue(ctx)
	SaveTempUsers(ctx)
	SaveFFmpegArgs(ctx)
	SaveMuxArgs(ctx)
	SaveFilterSettings(ctx)
	SaveRSSDict(ctx)
}

func SaveQueue(ctx context.Context) {
	if !useMongo || QueueCol == nil {
		SaveQueueLocal()
		//if err := SaveQueueLocal(); err != nil {
		// 	zap.L().Warn("SaveQueueLocal failed", zap.Error(err))
		//}
		return
	}
	filter := bson.M{"_id": config.Rconf.BotID}
	update := bson.M{"$set": bson.M{"queue": bot.Queue.AsSlice()}}
	opts := options.Update().SetUpsert(true)
	if _, err := QueueCol.UpdateOne(ctx, filter, update, opts); err != nil {
		zap.L().Warn("SaveQueue failed", zap.Error(err))
	}
}

func SaveBatchQueue(ctx context.Context) {
	if !useMongo || QueueCol == nil {
		if err := SaveBatchQueueLocal(); err != nil {
			zap.L().Warn("SaveBatchQueueLocal failed", zap.Error(err))
		}
		return
	}
	filter := bson.M{"_id": config.Rconf.BotID}
	update := bson.M{"$set": bson.M{"batches": bot.BatchQueue}}
	opts := options.Update().SetUpsert(true)
	if _, err := QueueCol.UpdateOne(ctx, filter, update, opts); err != nil {
		zap.L().Warn("SaveBatchQueue failed", zap.Error(err))
	}
}

func SaveTempUsers(ctx context.Context) {
	if !useMongo || UsersCol == nil {
		SaveTempUsersLocal()
		// if err := SaveTempUsersLocal(); err != nil {
		//	zap.L().Warn("SaveTempUsersLocal failed", zap.Error(err))
		//}
		return
	}
	filter := bson.M{"_id": config.Rconf.BotID}
	update := bson.M{"$set": bson.M{"t_users": bot.TempUsers}}
	opts := options.Update().SetUpsert(true)
	if _, err := UsersCol.UpdateOne(ctx, filter, update, opts); err != nil {
		zap.L().Warn("SaveTempUsers failed", zap.Error(err))
	}
}

func SaveFFmpegArgs(ctx context.Context) {
	if !useMongo || CodeCol == nil {
		SaveSettingsLocal()
		//if err := SaveSettingsLocal(); err != nil {
		//	zap.L().Warn("SaveFFmpegArgsLocal failed", zap.Error(err))
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
		zap.L().Warn("SaveFFmpegArgs failed", zap.Error(err))
	}
}

func SaveMuxArgs(ctx context.Context) {
	if !useMongo || CodeCol == nil {
		SaveSettingsLocal()
		//if err := SaveMuxArgsLocal(); err != nil {
		//	zap.L().Warn("SaveMuxArgsLocal failed", zap.Error(err))
		//}
		return
	}
	filter := bson.M{"_id": config.Rconf.BotID}
	update := bson.M{"$set": bson.M{"mux_args": bot.MuxArgs}}
	opts := options.Update().SetUpsert(true)
	if _, err := CodeCol.UpdateOne(ctx, filter, update, opts); err != nil {
		zap.L().Warn("SaveMuxArgs failed", zap.Error(err))
	}
}

func SaveRSSDict(ctx context.Context) {
	if !useMongo || RSSCol == nil {
		SaveRSSLocal()
		//if err := SaveRSSLocal(); err != nil {
		//	zap.L().Warn("SaveRSSLocal failed", zap.Error(err))
		//}
		return
	}
	filter := bson.M{"_id": config.Rconf.BotID}
	update := bson.M{"$set": bson.M{"rss": bot.RSSDict}}
	opts := options.Update().SetUpsert(true)
	if _, err := RSSCol.UpdateOne(ctx, filter, update, opts); err != nil {
		zap.L().Warn("SaveRSSDict failed", zap.Error(err))
	}
}

// SaveFilterSettings persists bot.Autoname and bot.CustomRename.
func SaveFilterSettings(ctx context.Context) {
	if !useMongo || FilterCol == nil {
		SaveSettingsLocal()
		//if err := SaveSettingsLocal(); err != nil {
		//	zap.L().Warn("SaveSettingsLocal (filters) failed", zap.Error(err))
		//}
		return
	}
	filter := bson.M{"_id": config.Rconf.BotID}
	update := bson.M{"$set": bson.M{
		"autoname":   bot.Autoname,
		"cus_rename": bot.CustomRename,
	}}
	opts := options.Update().SetUpsert(true)
	if _, err := FilterCol.UpdateOne(ctx, filter, update, opts); err != nil {
		zap.L().Warn("SaveFilterSettings failed", zap.Error(err))
	}
}
