package utils

import (
	"slices"
	"strconv"
	"strings"

	gp "github.com/celestix/gotgproto/types"
	"github.com/gotd/td/constant"
	"github.com/gotd/td/tg"

	"enc/config"
	"enc/internal/bot"
	"enc/internal/clients/telegram"
	"enc/internal/logger"
	"enc/utils/tele"
	"enc/utils/tele/parser"
)

func UserIsAllowed(userID int64) bool {
	return (UserIsOwner(userID) || UserIsTemp(userID))
}

func UserIsOwner(userID int64) bool {
	return slices.Contains(config.Rconf.Owners, userID)
}

func UserIsTemp(userID int64) bool {
	return slices.Contains(bot.TempUsers, userID)
}

func UserIsDev(userID int64) bool {
	return userID == config.Conf.Dev
}

func AllowPM() bool {
	return !config.Conf.NoPM
}

func AddTempUser(userID int64) {
	bot.TempUsers = append(bot.TempUsers, userID)
}

func RmTempUser(userID int64) {
	bot.TempUsers = remove(bot.TempUsers, userID)
}

func SendHelp(args string) bool {
	return strings.EqualFold(args, "-h") || strings.EqualFold(args, "--help")
}

func remove(s []int64, r int64) []int64 {
	out := s[:0]
	for _, v := range s {
		if v != r {
			out = append(out, v)
		}
	}
	return out
}

func CodeBlock(s string, html bool) string {
	if html {
		return "<pre>" + s + "</pre>"
	}
	return "```" + s + "```"
}

func StringFormat(s ...string) string {
	var sb strings.Builder
	for _, x := range s {
		sb.WriteString(x)
		if !strings.HasSuffix(x, " ") {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func YesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

// BcMsg broadcasts a message to every registered user in bot and returns a list of the messages sent.
func BcMsg(text string, exceptUser int64, mlist []*tele.Message) []*tele.Message {
	AddMessage := func(id int64) {
		ctx := telegram.Context
		pText, entities, _ := parser.Parse(text, true)
		resp, err := ctx.SendMessage(
			id, &tg.MessagesSendMessageRequest{
				Message:  pText,
				Entities: entities,
			},
		)
		if err != nil {
			logger.ErrorW(err.Error())
			return
		}
		msg := tele.NewMsgFromGotg(resp, ctx, id, ctx.Self.ID)
		mlist = append(mlist, msg)
	}
	for _, owner := range config.Rconf.Owners {
		if owner == exceptUser {
			continue
		}
		AddMessage(owner)
	}
	for _, user := range bot.TempUsers {
		if user == exceptUser {
			continue
		}
		AddMessage(user)
	}
	if logchat := config.Conf.LogChannel; logchat != 0 {
		AddMessage(logchat)
	}
	return mlist
}

func ExtractMessageText(message *gp.Message) string {
	var isText bool
	if message.Media != nil {
		switch message.Media.(type) {
		case *tg.MessageMediaWebPage:
			isText = true
		default:
		}
	}
	if message.Media == nil || isText {
		return message.Text
	}
	return ""
}

func GetChatID(msg *gp.Message) int64 {
	peer := msg.PeerID
	if peer == nil {
		return 0
	}
	var ID constant.TDLibPeerID
	switch p := peer.(type) {
	case *tg.PeerChannel:
		ID.Channel(p.ChannelID)
		return int64(ID)
	case *tg.PeerChat:
		ID.Chat(p.ChatID)
		return int64(ID)
	case *tg.PeerUser:
		return p.UserID
	default:
		return 0
	}
}

func Range2Slice(s string) (out []int) {
	r := strings.Split(s, "-")
	if len(r) != 2 {
		return out
	}
	s = strings.TrimSpace(r[0])
	e := strings.TrimSpace(r[1])
	var start, end int
	var err error
	if start, err = strconv.Atoi(s); err != nil {
		return out
	}
	if end, err = strconv.Atoi(e); err != nil {
		return out
	}
	for i := start; i < end+1; i++ {
		out = append(out, i)
	}
	return out
}

func Range2StringSlice(s string) (out []string) {
	r := strings.Split(s, "-")
	if len(r) != 2 {
		return out
	}
	s = strings.TrimSpace(r[0])
	e := strings.TrimSpace(r[1])
	var start, end int
	var err error
	if start, err = strconv.Atoi(s); err != nil {
		return out
	}
	if end, err = strconv.Atoi(e); err != nil {
		return out
	}
	for i := start; i < end+1; i++ {
		out = append(out, strconv.Itoa(i))
	}
	return out
}
