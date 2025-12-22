// Package rssmonitor
package rssmonitor

import (
	"context"
	//"errors"
	"fmt"
	//"net/http"
	"strings"
	//"sync"
	"time"

	"github.com/mmcdole/gofeed"
	"go.uber.org/zap"

	"enc/config"
	"enc/internal/bot"
	"enc/internal/bot/types"
	"enc/internal/db"
	"enc/internal/logger"
	"enc/utils"
	"enc/utils/tele"
	"enc/workers/auto/scheduler"
)

func sendRSS(feedMsg string, chat []types.FeedChat) (*tele.Message, error) {
	var msg *tele.Message
	var err error
	for _, fc := range chat {
		chatID := fc.ChatID
		if fc.Default {
			chatID = config.Conf.RSSChat
		}
		msg, err = tele.SendMsg(chatID, feedMsg, &tele.SendOptions{ReplyTo: fc.TopicID})
		if err != nil {
			logger.Error(err)
		}
	}
	return msg, err
}

func fakeEventHandler(event *tele.Message) {
	// event.Text expected to be "command args..."
	parts := strings.Fields(event.Raw.Message.Message)
	if len(parts) == 0 {
		return
	}
	command := parts[0]
	args := strings.Join(parts[1:], " ")

	handler, found := HandlerMap[strings.TrimLeft(command, "/")]
	if !found {
		return
	}
	go func() {
		err := handler(event, args)
		if err != nil {
			logger.Error(err)
		}
	}()

	time.Sleep(3 * time.Second)
}

// matchIncludeExclude returns true if title passes include/exclude lists.
// inf/exf are [][]string — each inner slice is a list where all terms must be *present*
// in the title (all(x not in title) in original means: parse only if there's at least one item from each include list).
func matchIncludeExclude(titleLower string, inf [][]string, exf [][]string) bool {
	// If any include-list exists, the item must satisfy at least one item from each include-sublist.
	// Python's logic: for each flist in data["inf"]: if all(x not in item_title.lower() for x in flist): parse = False
	// That means: for each flist, at least one x in flist must be present in titleLower.
	for _, flist := range inf {
		ok := false
		for _, x := range flist {
			if strings.Contains(titleLower, strings.ToLower(x)) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	// Exclusions: if any exclusion list has any item present => reject
	for _, flist := range exf {
		for _, x := range flist {
			if strings.Contains(titleLower, strings.ToLower(x)) {
				return false
			}
		}
	}
	return true
}

// rssMonitor performs a single sweep over the Bot.RSSDict similar to the Python coroutine.
func rssMonitorOnce(ctx context.Context) {
	// guard: check services like _bot.sas/_bot.sqs
	if !bot.SAS && !bot.SQS {
		// wait a bit and re-check (like your Python early sleep)
		time.Sleep(10 * time.Second)
		if !bot.SAS && !bot.SQS {
			logger.WarnW("RSS scheduler paused since a download service is not responding…")
			PauseRSS()
			return
		}
	}

	// Replace Go's logger call with your logging function
	// if conf.RSS_CHAT == 0 { ... }  -> you must check that externally
	if len(bot.RSSDict) == 0 {
		// no feeds -> pause scheduler
		// scheduler.pause()
		PauseRSS()
		return
	}

	// mark paused status (you may want to hold this in bot state)
	// pause(status="rss")
	var err error
	err = utils.Pause(2, true)
	if err != nil {
		logger.Error(err)
	}

	allPaused := true

	// iterate a snapshot of keys to avoid holding lock while processing network IO
	bot.RSSDictMutex.RLock()
	keys := make([]string, 0, len(bot.RSSDict))
	for k := range bot.RSSDict {
		keys = append(keys, k)
	}
	bot.RSSDictMutex.RUnlock()

	parser := gofeed.NewParser()

	for _, title := range keys {
		bot.RSSDictMutex.RLock()
		data := bot.RSSDict[title]
		bot.RSSDictMutex.RUnlock()

		// skip if feed removed concurrently
		if data == nil {
			continue
		}
		if data.Paused {
			continue
		}

		// fetch feed HTML
		// feedHTML, err := getHTML(ctx, data.Link)
		// if err != nil {
		// 	// log error and continue
		// 	// log(fmt.Sprintf("%v - Feed Name: %s - Feed Link: %s", err, title, data.Link))
		// 	continue
		// }

		feed, err := parser.ParseURL(data.Link)
		if err != nil || len(feed.Items) == 0 {
			logger.Error(err)
			continue
		}

		// determine last link similar to Python: try Items[0].Links[1] else Item[0].Link
		lastLink := ""
		if len(feed.Items[0].Links) > 1 {
			lastLink = feed.Items[0].Links[1]
		} else {
			lastLink = feed.Items[0].Link
		}
		lastTitle := feed.Items[0].Title

		// If no change, continue
		if data.LastFeed == lastLink || data.LastTitle == lastTitle {
			continue
		}
		// If first run, allow spam
		if !bot.RSSRanOnce {
			data.AllowSpam = true
		}

		allPaused = false

		feedList := []string{}
		// collect items from newest to older until we hit last_seen
		for idx := 0; idx < len(feed.Items); idx++ {
			item := feed.Items[idx]
			itemTitle := item.Title
			itemURL := ""
			if len(item.Links) > 1 {
				itemURL = item.Links[1]
			} else {
				itemURL = item.Link
			}

			// stop if we've reached last seen
			if itemURL == data.LastFeed || itemTitle == data.LastTitle {
				break
			}

			// check includes/excludes
			lowerTitle := strings.ToLower(itemTitle)
			if !matchIncludeExclude(lowerTitle, data.Inf, data.Exf) {
				continue
			}

			// build command as Python did
			cmdParts := strings.Fields(data.Command)
			if len(cmdParts) == 0 {
				feedList = append(feedList, fmt.Sprintf("Title: %s\nLink: %s", itemTitle, itemURL))
				continue
			}
			// insert url after first element
			// equivalent to: cmd = data["command"].split(maxsplit=1); cmd.insert(1, url)
			newCmdParts := make([]string, 0, len(cmdParts)+1)
			newCmdParts = append(newCmdParts, cmdParts[0])
			newCmdParts = append(newCmdParts, itemURL)
			if len(cmdParts) > 1 {
				newCmdParts = append(newCmdParts, strings.Join(cmdParts[1:], " "))
			}
			feedMsg := strings.Join(newCmdParts, " ")
			if !strings.HasPrefix(feedMsg, "/") {
				feedMsg = "/" + feedMsg
			}
			feedList = append(feedList, feedMsg)
		}

		for i := len(feedList) - 1; i >= 0; i-- {
			feedMsg := feedList[i]
			ev, err := sendRSS(feedMsg, data.Chat)
			if err == nil && ev != nil && data.Direct {
				go fakeEventHandler(ev)
			} else if ev != nil && err == nil {
				logger.Warn("Unknown error @rssMonitor")
			}
			// throttle between sends
			time.Sleep(1 * time.Second)
		}

		// update feed data with lock
		bot.RSSDictMutex.Lock()
		if d := bot.RSSDict[title]; d != nil {
			d.AllowSpam = false
			d.LastFeed = lastLink
			d.LastTitle = lastTitle
		}
		db.SaveRSSDict(ctx)
		bot.RSSDictMutex.Unlock()

		// log feed info
		logger.InfoW(fmt.Sprintf("Feed Name: %s", title))
		logger.InfoW(fmt.Sprintf("Last item: %s", lastLink))
	}

	err = utils.RmPause(2, 0)
	if err != nil {
		logger.Error(err)
	}
	if allPaused {
		PauseRSS()
		logger.InfoW("No active rss feed. Rss Monitor has been paused!")
	} else if !bot.RSSRanOnce {
		bot.RSSRanOnce = true
	}
}

// ----- Public helper to schedule RSS -----

var DefaultScheduler *scheduler.Scheduler

// ScheduleRSS creates and starts the scheduler with given delay (seconds).
// Call ScheduleRSS(delaySeconds) from your main program.
func ScheduleRSS(delaySeconds int) {
	job := func(ctx context.Context) {
		rssMonitorOnce(ctx) // your function
	}
	DefaultScheduler = scheduler.New(time.Duration(delaySeconds)*time.Second, job)
	logger.InfoW("Scheduled RSS Monitor!", zap.Int("Run every", delaySeconds))
	DefaultScheduler.Start(20 * time.Second)
}

func PauseRSS() {
	if DefaultScheduler != nil {
		DefaultScheduler.Pause()
	}
}

func ResumeRSS() {
	if DefaultScheduler != nil {
		DefaultScheduler.Resume()
	}
}

func StopRSS() {
	if DefaultScheduler != nil {
		DefaultScheduler.Stop()
	}
}
