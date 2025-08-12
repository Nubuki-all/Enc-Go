package utils

import (
	"fmt"
	"strings"

	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"
)

func FetchPeer(ctx *ext.Context, id int64) (tg.InputPeerClass, error) {
	peer, ok := ctx.PeerStorage.GetInputPeerByID(id)
	if ok {
		return peer, nil
	} else {
		return GetPeer(ctx, id)
	}
}

func GetPeer(ctx *ext.Context, id int64) (tg.InputPeerClass, error) {
	// 1️⃣ First try PeerStorage
	if peer, ok := ctx.PeerStorage.GetInputPeerByID(id); ok {
		return peer, nil
	}

	// 2️⃣ Detect channel/supergroup vs user
	idStr := fmt.Sprint(id)
	isChannel := strings.HasPrefix(idStr, "-100")

	if isChannel {
		// Channel/Supergroup
		chats, err := ctx.Raw.ChannelsGetChannels(ctx, []tg.InputChannelClass{
			&tg.InputChannel{
				ChannelID:  id,
				AccessHash: 0,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to fetch channel: %w", err)
		}

		if len(chats) > 0 {
			if ch, ok := chats[0].(*tg.Channel); ok {
				peer := &tg.InputPeerChannel{
					ChannelID:  ch.ID,
					AccessHash: ch.AccessHash,
				}
				ctx.PeerStorage.Put(peer)
				return peer, nil
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
				ctx.PeerStorage.Put(peer)
				return peer, nil
			}
		}
	}

	return nil, fmt.Errorf("peer not found for id %d", id)
}
