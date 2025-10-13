package queueutils

import (
	"cmp"
	"enc/config"
	"enc/internal/bot"
	"enc/utils"
	"enc/utils/tele"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/constant"
	"github.com/gotd/td/telegram/message/markup"
	"github.com/gotd/td/tg"
)

const CallbackData = "status/"

type QueuePreview struct {
	Bmsg  *tele.Message
	msg   *tele.Message
	Mutex sync.Mutex
	close bool
	previewed chan struct{}
	updated *chan struct{}
	start  int
	limit  int
	pageNo int
	pages  int
	prevMsg string
}

var qpReg map[int64]int
var qpRegMux sync.Mutex

func register(chatID int64, ID int) {
	qpRegMux.Lock()
	defer qpRegMux.Unlock()
	qpReg[chatID] = ID
}

func unRegister(chatID int64, ID int) {
	qpRegMux.Lock()
	defer qpRegMux.Unlock()
	if qpReg[chatID] == ID {
		delete(qpReg, chatID)
	}
}

func isRegistered(chatID int64, ID int) bool {
	qpRegMux.Lock()
	defer qpRegMux.Unlock()
	return qpReg[chatID] == ID 
}

func (qp *QueuePreview) Start() (err error) {
	// set defaultw
	qp.pageNo =1
	qp.pages =1
	qp.limit = 10
	qp.msg, err = qp.Bmsg.ReplyText("`Listing queue pls wait…`")
	if err != nil {
		return err
	}
	err = qp.update()
	if err != nil {
		return err
	}
	register(qp.msg.ChatID, qp.msg.ID)
	id := fmt.Sprintf("%d:%d", qp.msg.ChatID, qp.msg.ID)
	_ = storeqpCallbackInfo(qp, id)
	ch := make(chan struct{}, 1)
	bot.Queue.Subscribe(qp.msg.ChatID, ch)
	uTicker := time.NewTicker(5 * time.Second)
	defer uTicker.Stop()
	Out:
	for  {
		select {
		case <-qp.previewed:
			break Out
		case <-uTicker.C:
			select {
			case <- ch:
				qp.Mutex.Lock()
				_ = qp.update()
				qp.Mutex.Unlock()
			default:
			}
		default:
			if !isRegistered(qp.msg.ChatID, qp.msg.ID) {
				break Out
			}
		}	
	}
	bot.Queue.UnSubscribe(qp.msg.ChatID, ch)
	_ = qp.msg.Delete()
	delBatchCallbackInfo(id)
	return nil
}

func (qp *QueuePreview) update() (err error) {
	select {
	case <-qp.previewed:
		return nil
	default:
	}
	total := bot.Queue.Len()
	if qp.close || total < 2 {
		unRegister(qp.msg.ChatID, qp.msg.ID)
		close(qp.previewed)
		return nil
	}
	qp.pages = (total + qp.limit -2) / qp.limit
	if qp.pageNo > qp.pages && qp.pages != 0 {
		qp.start = (qp.limit * qp.pages) -9
		qp.pageNo = qp.pages
	}
	queue := bot.Queue.AsSlice()
	var msg strings.Builder
	for i := qp.start; i < (qp.limit + qp.start); i++ {
		entry := queue[i]
		user := entry.Sender
		if userID := constant.TDLibPeerID(user); !userID.IsUser() {
			user = 777000
		}
		userName := "Unknown"
		userInfo, err := tele.GetUser(user)
		if err == nil {
			userName = userInfo.FirstName
		}
		var aniL, batch, filStr, fRename string
		if entry.Filter != nil {
			fil := entry.Filter
			filStr = fmt.Sprintf("[-rm: `%s`, -tf: `%s`, -tc: `%s`]", cmp.Or(fil.Fil1, "None"), cmp.Or(fil.Fil2, "None"), cmp.Or(fil.Fil3, "None"))
		}
		if len(entry.Batch) != 0 {
			batch = "  ├**Batch:** Yes\n"
		}
		if entry.ForceName != "" {
			fRename = "  ├**Force rename to:** `" + entry.ForceName + "`\n"
		}
		if !entry.AniList {
			aniL = "  ├**Anilist:** Off\n"
		}
		fmt.Fprintf(
			&msg,
			"%d. `%s`\n  ├**Filter:** %s\n  ├**Release version:** %s\n%s  └**Added by:** [%s](tg://user?id=%d)\n\n",
			i,
			entry.Name,
			cmp.Or(filStr, "[None]"),
			cmp.Or(entry.Version, "None"),
			batch+fRename+aniL,
			userName,
			user,
		)
	}
	if msg.Len() == 0 {
		return nil
	}
	var buttonRows []tg.KeyboardButtonRow
	if (total -1) > qp.limit {
		buttonRows = append(buttonRows,	
			markup.Row(
				markup.Callback(
					"<<",
					[]byte(CallbackData+" prev"),
				),
				markup.Callback(
					fmt.Sprintf("%d/%d (%d)", qp.pageNo, qp.pages, total-1),
					[]byte(CallbackData+" 🙆🏻"),
				),
				markup.Callback(
					">>",
					[]byte(CallbackData+" next"),
				),
			),	
		)
	} else {
		fmt.Fprintf(&msg, "**Pending Tasks:** %d\n", total-1)
	}
	fmt.Fprintf(&msg, "\n**📌 Tip: To remove an item from queue use** /clear%s <queue number>", config.Conf.CmdSuffix)
	txt := msg.String()
	if qp.prevMsg == txt {return nil}
	qp.prevMsg = txt
	buttonRows = append(buttonRows,	
		markup.Row(
			markup.Callback(
				"Close",
				[]byte(CallbackData+" close"),
			),
		),
	)
	_, err = qp.msg.Edit(
		txt,
		&tele.EditOptions{ReplyMarkup: markup.InlineKeyboard(buttonRows...),
	})
	return
}

var qpCallbackDataMutex sync.Mutex
var qpCallbackData  map[string]*QueuePreview


func storeqpCallbackInfo(data *QueuePreview, key string) string {
	qpCallbackDataMutex.Lock()
	defer qpCallbackDataMutex.Unlock()
	if key == "" {
		k := len(qpCallbackData)
		key = strconv.Itoa(k)
	}
	qpCallbackData[key] = data
	return key
}

func getqpCallbackInfo(key string) *QueuePreview {
	qpCallbackDataMutex.Lock()
	defer qpCallbackDataMutex.Unlock()
	return qpCallbackData[key]
}

func delBatchCallbackInfo(key string) {
	qpCallbackDataMutex.Lock()
	defer qpCallbackDataMutex.Unlock()
	delete(qpCallbackData, key)
}

func PreviewAction(ctx *ext.Context, update *ext.Update, query *tele.CallbackQuery) error {
	id := fmt.Sprintf("%d:%d", query.ChatID, query.ID)
	preview := getqpCallbackInfo(id)
	if preview == nil {
		_ = query.Answer("Preview Information is missing", false)
		_ = query.GetMessage().Delete()
		return nil
	}
	sender := query.SenderID
	if !utils.UserIsAllowed(sender) {
		_ = query.Answer("Nah…", false)
		return nil
	}
	args := string(query.Query.Data)
	var data string
	if d := strings.SplitN(args, " ", 2); len(d) == 2 {
		data = d[1]
	}
	_=query.Answer(data +"...", true)
	preview.Mutex.Lock()
	defer preview.Mutex.Unlock()
	switch data{
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
			preview.start = (preview.limit * (preview.pages - 1)) + 1
			preview.pageNo = preview.pages
		} else {
			preview.start -= preview.limit
			preview.pageNo -= 1
		}
	case "close":
		preview.close = true	
	default:
	}
	_ = preview.update()
	return nil
}
