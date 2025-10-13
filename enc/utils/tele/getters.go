package tele

import (
	"fmt"
	"strconv"
	"strings"

	//"strconv"
	//"strings"

	//"github.com/celestix/gotgproto/ext"
	mtp_errors "github.com/celestix/gotgproto/errors"
	"github.com/celestix/gotgproto/storage"
	gp "github.com/celestix/gotgproto/types"
	"github.com/gotd/td/constant"
	"github.com/gotd/td/tg"

	"enc/internal/clients/telegram"
)

func GetUser(userID int64) (*tg.User, error) {
	ctx := telegram.Context
	peer := ctx.PeerStorage.GetPeerById(userID)
	if peer.ID == 0 {
		return nil, mtp_errors.ErrPeerNotFound
	}
	if peer.Type == storage.TypeUser.GetInt() {
		users, err := ctx.Raw.UsersGetUsers(ctx, []tg.InputUserClass{
			&tg.InputUser{
				UserID:     peer.ID,
				AccessHash: peer.AccessHash,
			},
		})
		if err != nil {
			return nil, err
		}
		user, ok := tg.UserClassArray(users).FirstAsNotEmpty()
		if !ok {
			return nil, fmt.Errorf("users response count: %v", users)
		}
		return user, nil
	} else {
		return nil, mtp_errors.ErrNotUser
	}
}

func GetMessages(chatID int64, ids ...tg.InputMessageClass) (msgs []*gp.Message, err error) {
	ctx := telegram.Context
	peer := ctx.ResolvePeerById(chatID)
	if peer.ID == 0 {
		return nil, mtp_errors.ErrPeerNotFound
	}
	var messages tg.MessagesMessagesClass
	switch storage.EntityType(peer.Type) {
	case storage.TypeChannel:
		ID := constant.TDLibPeerID(peer.ID)
		channel := &tg.InputChannel{
			ChannelID:  ID.ToPlain(),
			AccessHash: peer.AccessHash,
		}
		messages, err = ctx.Raw.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
			Channel: channel,
			ID:      ids,
		})
	default:
		messages, err = ctx.Raw.MessagesGetMessages(ctx, ids)
	}
	if err != nil {
		return msgs, err
	}
	var msgArray tg.MessageClassArray
	switch m := messages.(type) {
	case *tg.MessagesMessages:
		// SavePeersFromClassArray(p, m.Chats, m.Users)
		msgArray = m.Messages
	case *tg.MessagesMessagesSlice:
		// SavePeersFromClassArray(p, m.Chats, m.Users)
		msgArray = m.Messages
	case *tg.MessagesChannelMessages:
		// SavePeersFromClassArray(p, m.Chats, m.Users)
		msgArray = m.Messages
	default:
		return msgs, err
	}

	for _, msg := range msgArray {
		msgs = append(msgs, gp.ConstructMessage(msg))
	}
	if len(msgs) == 0 {
		err = fmt.Errorf("no messages was found")
	}
	return msgs, err
}

func GetMessageFromLink(url string) *Message {
	if url == "" {
		return nil
	}
	var chatID int64
	tokens := strings.Split(url, "/")
	if len(tokens) < 4 {
		return nil
	}
	chatIDStr := tokens[len(tokens)-2]
	if !strings.HasPrefix(chatIDStr, "-100") {
		chatIDStr = "-100" + chatIDStr
	}
	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		return nil
	}
	ID, err := strconv.Atoi(tokens[len(tokens)-1])
	if err != nil {
		return nil
	}
	msgs, err := GetMessages(chatID, &tg.InputMessageID{ID: ID})
	if err != nil {
		return nil
	}
	var userID int64
	if userPeer, ok := msgs[0].FromID.(*tg.PeerUser); ok {
		userID = userPeer.UserID
	}
	return NewMsgFromGotg(msgs[0], telegram.Context, chatID, userID)
}

func GetSenderIDFromMessage(msg *gp.Message) int64 {
	if msg == nil {
		return 0
	}
	if userPeer, ok := msg.PeerID.(*tg.PeerUser); ok {
		return userPeer.UserID
	}
	var id constant.TDLibPeerID
	switch p := msg.FromID.(type) {
	case *tg.PeerUser:
		return p.UserID
	case *tg.PeerChannel:
		id.Channel(p.ChannelID)
		return int64(id)
	case *tg.PeerChat:
		id.Chat(p.ChatID)
		return int64(id)
	default:
		return 0
	}
}
