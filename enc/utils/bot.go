package utils

import (
	"fmt"
	"strings"
	"time"

	"enc/internal/bot"
	"enc/internal/bot/types"
	"enc/internal/bot/state"
)

// AddTempUser appends an ID to bot.TempUsers
func AddTempUser(id string) {
	bot.TempUsers = append(bot.TempUsers, id)
}

// BotIsPaused returns true if any pause flag is set
func BotIsPaused() bool {
	return bot.Paused.Any()
}

// UCancelled returns the list of user cancellations
func UCancelled() []interface{} {
	return bot.UCancel
}

// EnCanceller returns the encode cancellation map
func EnCanceller() map[string]interface{} {
	return bot.ECancel
}

func IndexToPauseType(i int) (state.PauseType, bool) {
	pt := state.PauseType(i)
	switch pt {
	case state.DefaultPause, state.DLPause, state.RSSPause:
		return pt, true
	}
	return 0, false
}

// Pause sets or clears a specific pause type
func PauseByType(pt state.PauseType, on bool) {
	bot.Paused.Set(pt, on)
}

func Pause(idx int, on bool) error {
	pt, ok := IndexToPauseType(idx)
	if !ok {
		return fmt.Errorf("invalid pause index %d", idx)
	}
	PauseByType(pt, on)
	return nil
}

// GetPauseStatus returns whether a specific pause type is set
func GetPauseStatusByType(pt state.PauseType) bool {
	return bot.Paused.Get(pt)
}

func GetPauseStatus(idx int) (bool, error) {
	pt, ok := IndexToPauseType(idx)
	if !ok {
		return false, fmt.Errorf("invalid pause index %d", idx)
	}
	return GetPauseStatusByType(pt), nil
}

// RmPause clears the given pause type after duration if still set
func RmPauseByType(pt state.PauseType, duration time.Duration) {
	if !bot.Paused.Get(pt) {
		return
	}
	go func() {
		time.Sleep(duration)
		// only clear if still paused
		if bot.Paused.Get(pt) {
			bot.Paused.Set(pt, false)
		}
	}()
}

func RmPause(idx int, duration time.Duration) error {
	pt, ok := IndexToPauseType(idx)
	if !ok {
		return fmt.Errorf("invalid pause index %d", idx)
	}
	RmPauseByType(pt, duration)
	return nil
}

// GetBatchQueue returns the current batch queue
func GetBatchQueue() map[string]interface{} {
	return bot.BatchQueue
}

// GetPreview returns preview batch or list
func GetPreview(list bool) interface{} {
	if list {
		return bot.PreviewList
	}
	return bot.PreviewBatch
}

// GetPreviewer returns the first preview ing item
func GetPreviewer() interface{} {
	if len(bot.BatchIng) > 0 {
		return bot.BatchIng[0]
	}
	return nil
}

// GetQueue returns the main queue
func GetQueue() *types.Queue {
	return bot.Queue
}

// GetVar returns internal variable by name
func GetVar(name string) interface{} {
	switch nameLower := strings.ToLower(name); nameLower {
	case "groupenc":
		return bot.GroupEnc
	case "version2":
		return bot.ReleaseVersion
	case "paused":
		return bot.Paused
	default:
		return nil
	}
}

// GetVersion returns the global release version
func GetVersion() string {
	return bot.ReleaseVersion
}
