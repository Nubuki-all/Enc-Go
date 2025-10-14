// Package batchutils contains functions for generating batch previews
package batchutils

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"enc/config"
	"enc/internal/bot/types"
	"enc/internal/logger"
	"enc/utils"
	"enc/utils/parser"
	"enc/utils/tele"

	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/telegram/message/markup"
	"github.com/gotd/td/tg"
	"go.uber.org/zap"
)

const CallbackData = "preview"

func GetBatches(files []string, zeroID bool) (items []types.BatchItem) {
	id := 0
	if !zeroID {
		id += 1
	}
	for _, name := range files {
		item := types.BatchItem{FileName: filepath.Base(name), ID: strconv.Itoa(id)}
		if !utils.IsVideoFile(name) {
			item.Status = 2
		}
		items = append(items, item)
		id += 1
	}
	return items
}

func GetYtdlBatches(files []string) (items []types.BatchItem) {
	id := 1
	for _, name := range files {
		item := types.BatchItem{FileName: filepath.Base(name), ID: strconv.Itoa(id)}
		items = append(items, item)
		id += 1
	}
	return items
}

func GetPendingItems(items []types.BatchItem, limit int, anilist bool, version string, filter *types.FilterData, truncate bool) ([]string, int) {
	var pending []string
	for _, item := range items {
		if item.Status == 1 {
			pending = append(pending, item.FileName)
		}
	}
	if len(pending) > 0 {
		pending = pending[1:]
	}
	length := len(pending)
	list := make([]string, limit)
	for _, n := range pending[:limit] {
		q, e := parser.QParse(n, &parser.ParseOptions{
			UseAniList:     anilist,
			FilterOverride: filter,
			Version:        version,
		})
		if e != nil {
			q = n
		}
		if truncate && len(q) > 45 {
			q = q[:45] + "…"
		}
		list = append(list, q)
	}
	return list, length - limit
}

var StatusMap = map[int]string{
	3: "Not a video.",
	2: "DONE!",
	1: "✅",
	0: "⬜",
}

type BatchPreview struct {
	msg         *tele.Message
	Bmsg        *tele.Message
	Version     string
	Filter      *types.FilterData
	Items       []types.BatchItem
	Previewer   int64
	Mutex       sync.Mutex
	items       []types.BatchItem
	isCancelled bool
	done        bool
	previewed   chan struct{}
	start       int
	limit       int
	pageNo      int
	pages       int
	parse       bool
	prevMsg     string
}

func (bp *BatchPreview) Start(emptyOnCancel bool) (_ []types.BatchItem, err error) {
	if len(bp.Items) == 0 {
		return nil, nil
	}
	// set defaultw
	bp.pageNo = 1
	bp.pages = 1
	bp.limit = 12
	if !emptyOnCancel {
		copy(bp.items, bp.Items)
	}
	bp.msg, err = bp.Bmsg.ReplyText("…")
	if err != nil {
		return nil, err
	}
	bp.previewed = make(chan struct{})
	err = bp.Update()
	if err != nil {
		return nil, err
	}
	id := fmt.Sprintf("%d:%d", bp.msg.ChatID, bp.msg.ID)
	_ = storeBatchCallbackInfo(&batchCallBackInfo{bp}, id)
Out:
	for {
		select {
		case <-bp.previewed:
			break Out
		default:
			time.Sleep(1 * time.Second)
		}
	}
	delBatchCallbackInfo(id)
	_ = bp.msg.Delete()
	return bp.Items, nil
}

func (bp *BatchPreview) Update() (err error) {
	select {
	case <-bp.previewed:
		return nil
	default:
	}
	if bp.isCancelled {
		bp.Items = bp.items
	}
	if bp.isCancelled || bp.done {
		close(bp.previewed)
		return err
	}
	totalSelected := 0
	for _, item := range bp.Items {
		if item.Status == 1 {
			totalSelected += 1
		}
	}
	total := len(bp.Items)
	bp.pages = (total + bp.limit - 1) / bp.limit
	if bp.pageNo > bp.pages && bp.pages != 0 {
		bp.start = (bp.limit * bp.pages) - 12
		bp.pageNo = bp.pages
	}
	var msg strings.Builder
	for _, item := range bp.Items[bp.start : bp.limit+bp.start] {
		name := item.FileName
		if bp.parse && item.Status != 3 {
			name, err = parser.QParse(name, &parser.ParseOptions{
				FilterOverride: bp.Filter, Version: bp.Version,
			})
			if err != nil {
				logger.WarnW("Error parsing item", zap.Error(err))
			}
		}
		fmt.Fprintf(&msg, "%s. <code>%s</code>\n  └<b>Status:</b> <code>%s</code>\n\n", item.ID, name, StatusMap[item.Status])
	}
	// if there's by some miracle an empty Message return
	if msg.Len() == 0 {
		return nil
	}
	var buttonRows []tg.KeyboardButtonRow
	if total > bp.limit {
		buttonRows = append(buttonRows,
			markup.Row(
				markup.Callback(
					"<<",
					[]byte(CallbackData+" prev"),
				),
				markup.Callback(
					fmt.Sprintf("%d/%d (%d of %d)", bp.pageNo, bp.pages, totalSelected, total),
					[]byte(CallbackData+" 🙆🏻"),
				),
				markup.Callback(
					">>",
					[]byte(CallbackData+" next"),
				),
			),
		)
	} else {
		fmt.Fprintf(&msg, "<b>Selected:</b> %d of %d\n", totalSelected, total)
	}
	parseBtn := "Parse"
	if bp.parse {
		parseBtn = "Raw"
	}
	buttonRows = append(buttonRows,
		markup.Row(
			markup.Callback(
				parseBtn,
				[]byte(CallbackData+" parse"),
			),
			markup.Callback(
				"Done",
				[]byte(CallbackData+" done"),
			),
		),
		markup.Row(
			markup.Callback(
				"Cancel",
				[]byte(CallbackData+" cancel"),
			),
		),
	)
	cmdSuf := config.Conf.CmdSuffix
	fmt.Fprintf(&msg, "\n<b>📌 Tip: To change the state of an item reply this message with</b> /select%s", cmdSuf)
	txt := msg.String()
	if txt == bp.prevMsg {
		return err
	}
	bp.prevMsg = txt
	_, err = bp.msg.Edit(
		txt,
		&tele.EditOptions{
			HTMLParseMode: true,
			ReplyMarkup:   markup.InlineKeyboard(buttonRows...),
		})
	return err
}

type batchCallBackInfo struct {
	*BatchPreview
}

var (
	batchCallbackDataMutex sync.Mutex
	batchCallbackData      = make(map[string]*batchCallBackInfo)
)

func storeBatchCallbackInfo(data *batchCallBackInfo, key string) string {
	batchCallbackDataMutex.Lock()
	defer batchCallbackDataMutex.Unlock()
	if key == "" {
		k := len(batchCallbackData)
		key = strconv.Itoa(k)
	}
	batchCallbackData[key] = data
	return key
}

func GetBatchCallbackInfo(key string) *batchCallBackInfo {
	batchCallbackDataMutex.Lock()
	defer batchCallbackDataMutex.Unlock()
	return batchCallbackData[key]
}

func delBatchCallbackInfo(key string) {
	batchCallbackDataMutex.Lock()
	defer batchCallbackDataMutex.Unlock()
	delete(batchCallbackData, key)
}

func PreviewAction(ctx *ext.Context, update *ext.Update, query *tele.CallbackQuery) error {
	id := fmt.Sprintf("%d:%d", query.ChatID, query.ID)
	preview := GetBatchCallbackInfo(id)
	if preview == nil {
		_ = query.Answer("Preview Information is missing", false)
		_ = query.GetMessage().Delete()
		return nil
	}
	sender := query.SenderID
	if preview.Previewer != sender && !utils.UserIsOwner(sender) {
		_ = query.Answer("Nah…", false)
		return nil
	}
	args := string(query.Query.Data)
	var data string
	if d := strings.SplitN(args, " ", 2); len(d) == 2 {
		data = d[1]
	}
	_ = query.Answer(data+"...", false)
	preview.Mutex.Lock()
	defer preview.Mutex.Unlock()
	switch data {
	case "next":
		if preview.pageNo == preview.pages {
			preview.pageNo = 1
			preview.start = 1
		} else {
			preview.start += preview.limit
			preview.pageNo += 1
		}
	case "prev":
		if preview.pageNo == 1 {
			preview.start = preview.limit * (preview.pages - 1)
			preview.pageNo = preview.pages
		} else {
			preview.start -= preview.limit
			preview.pageNo -= 1
		}
	case "cancel":
		preview.isCancelled = true
	case "done":
		preview.done = true
	case "parse":
		preview.parse = !preview.parse
	default:
		logger.WarnW("Unknown Query data:", zap.String("Data", data))
	}
	_ = preview.Update()
	return nil
}
