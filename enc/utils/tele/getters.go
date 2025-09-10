package tele

import (
	"fmt"
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

func GetUser(userId int64) (*tg.User, error) {
	ctx := telegram.Context
	peer := ctx.PeerStorage.GetPeerById(userId)
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

func GetMessages(chatId int64, ids ...tg.InputMessageClass) (msgs []*gp.Message, err error) {
	ctx := telegram.Context
	peer := ctx.ResolvePeerById(chatId)
	if peer.ID == 0 {
		return nil, errors.ErrPeerNotFound
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
			ID:      Ids,
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
