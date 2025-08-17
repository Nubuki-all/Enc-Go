package tele

import (
	"fmt"
	"strconv"
	"strings"

	//"github.com/celestix/gotgproto/ext"
	"github.com/celestix/gotgproto/storage"
	"github.com/gotd/td/tg"

	"enc/internal/clients/telegram"
)

func GetPeer(id int64) (tg.InputPeerClass, error) {
	ctx := telegram.Context
	peer := ctx.PeerStorage.GetInputPeerById(id)
	if _, isEmpty := peer.(*tg.InputPeerEmpty); !isEmpty {
		return peer, nil
	}

	idStr := fmt.Sprint(id)
	isChannel := strings.HasPrefix(idStr, "-100")

	if isChannel {
		// Extract raw channel ID from the peer ID
		channelID, err := strconv.ParseInt(idStr[4:], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse channel ID: %w", err)
		}

		chatsClass, err := ctx.Raw.ChannelsGetChannels(ctx, []tg.InputChannelClass{
			&tg.InputChannel{
				ChannelID:  channelID,
				AccessHash: 0,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to fetch channel: %w", err)
		}

		/*chats := chatsClass.GetChats()
		if len(chats) > 0 {*/
		chat, ok := chatsClass.MapChats().First()
		if ok {
			if ch, ok := chat.(*tg.Channel); ok {
				ctx.PeerStorage.AddPeer(id, ch.AccessHash, storage.TypeChannel, ch.Username)
				return ch, nil
			}
		}
	} else {
		// User
		users, err := ctx.Raw.UsersGetUsers(ctx, []tg.InputUserClass{
			&tg.InputUser{
				UserID:     id,
				AccessHash: 0,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to fetch user: %w", err)
		}

		if len(users) > 0 {
			if u, ok := users[0].(*tg.User); ok {
				peer := &tg.InputPeerUser{
					UserID:     u.ID,
					AccessHash: u.AccessHash,
				}
				// Store with correct AddPeer signature
				ctx.PeerStorage.AddPeer(id, u.AccessHash, storage.TypeUser, u.Username)
				return peer, nil
			}
		}
	}

	return nil, fmt.Errorf("peer not found for id %d", id)
}
