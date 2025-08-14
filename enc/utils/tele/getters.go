package tele

import (
	"fmt"
	//"strconv"
	//"strings"

	//"github.com/celestix/gotgproto/ext"
	"github.com/celestix/gotgproto/storage"
	mtp_errors "github.com/celestix/gotgproto/errors"
	"github.com/gotd/td/tg"

	"enc/internal/clients/telegram"
)

func GetUser (userId int64) (*tg.User, error) {
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