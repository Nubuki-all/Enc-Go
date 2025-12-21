package uploaders

import (
	"cmp"
	"fmt"
	"path/filepath"

	"enc/config"
	"enc/utils"
	"enc/utils/tele"
)

const defSticker = "CAACAgEAAxkBAAI0aWKx36P2GY9Fq6xvN0SBU1V2xZYIAAKXAgACJ_hhR9HcWzoditT7HgQ"

func DumpDl(path, name string, user int64, msg *tele.Message) error {
	dmpPath := filepath.Join(config.Conf.DumpDir, filepath.Base(path))
	if err := utils.CopyFile(path, dmpPath); err != nil {
		return err
	}
	defer utils.SRemove(false, dmpPath)
	caption := "`" + name + "`"
	var err error
	if id := config.Conf.DumpChannel; id != 0 {
		msg, err = tele.SendMsg(id, "**NEW DUMP!**\n\n**FileName:** `"+filepath.Base(dmpPath)+"`", nil)
		if err != nil {
			return err
		}
		defer func() {
			sticker := cmp.Or(config.Conf.FSticker, defSticker)
			_, _ = tele.SendStickerByFileID(id, sticker)
		}()
	}
	dumpMsg, err := msg.ReplyText("**[Dumper]:** `Waiting for uploader...`]")
	if err != nil {
		return err
	}
	if utils.SizeOf(dmpPath) > 2126000000 {
		_, _ = dumpMsg.EditText("**[Dumper]:** `File too large to dump, Aborting…`")
		return nil
	}
	uID := fmt.Sprintf("%d:%d", dumpMsg.ChatID, dumpMsg.ID)
	upload := &Uploader{
		Msg:     dumpMsg,
		Caption: caption,
	}
	upload.Init(uID, dmpPath, user)
	if err := upload.Start(); err != nil || upload.IsCancelled {
		_, _ = upload.ReportStatus()
	}
	return nil
}
