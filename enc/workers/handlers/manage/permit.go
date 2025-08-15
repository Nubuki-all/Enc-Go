package manage

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"

	"enc/internal/bot"
	"enc/internal/db"
	"enc/utils"
	"enc/utils/random"
	"enc/utils/tele"
)

// TempUnauth removes a temporarily allowed user (unpermit).
// It expects either a reply to a user's message, or an argument containing the user id.
func TempUnauth(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	senderID := msg.SenderID
	errPrefix := "Failed!,\nCan't remove from temporarily allowed users"
	help := `Un-authorise a user or chat
    Requires either reply to message or user_id as args`
	if args == "" || utils.SendHelp(args) {
		_, _ = msg.ReplyText("```" + help + "```")
		return nil
	}

	// permission check
	if !utils.UserIsOwner(senderID) {
		// Not authorized to run this command
		_, _ = msg.ReplyText("Not Happening.")
		return nil
	}

	var newID int64
	var err error
	message := update.EffectiveMessage

	// If the command is a reply, extract ID from replied-to message
	if repMsg := message.ReplyToMessage; repMsg != nil {
		peer := repMsg.Message.FromID
		if peerUser, ok := peer.(*tg.PeerUser); ok {
			newID = peerUser.UserID
		}
	} else {
		if args == "" {
			_, _ = msg.ReplyText("Either reply to a user's message or provide a user id.\nExample:\n`/unpermit 123456`")
			return nil
		}
		arg := strings.TrimSpace(parts[1])
		// allow negative IDs with leading '-' for channels/chats
		if _, err := strconv.Atoi(arg); err != nil {
			_, _ = msg.ReplyText(fmt.Sprintf("What do you mean by `%s` ?\nNeed help? send /unpermit", arg))
			return nil
		}
		newID, err = strconv.ParseInt(arg, 10, 64)
		if err != nil {
			_, _ = msg.ReplyText(fmt.Sprintf("Failed to parse id `%s`: %v", arg, err))
			return nil
		}
	}

	// disallow self-unpermit
	if newID == senderID {
		_, _ = msg.ReplyText("Why, oh why did you try to unpermit yourself?")
		return nil
	}

	// can't unpermit an owner / privileged user
	if utils.UserIsOwner(newID) {
		_, _ = msg.ReplyText(fmt.Sprintf("%s because user is already a privileged user", errPrefix))
		return nil
	}

	// ensure user is in the temporary allow list
	if !utils.UserIsTemp(newID) {
		_, _ = msg.ReplyText(fmt.Sprintf("%s because user is not in the temporary allowed user list", errPrefix))
		return nil
	}

	var displayName string
	if user, e := tele.GetUser(newID); e == nil && user != nil {
		displayName = user.FirstName
	} else {
		displayName = fmt.Sprintf("%d", newID)
	}
	utils.RmTempUser(newID)
	db.SaveTempUsers(context.Background())
	_, _ = msg.ReplyText(fmt.Sprintf("Removed `%s` from temporarily allowed users %s", displayName, random.Enmoji()))
	return nil
}
