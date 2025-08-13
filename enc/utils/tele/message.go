package tele

import (

	//"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"

	"enc/internal/clients/telegram"
	"enc/utils/tele/parser"
)


func SendMessage(id int64, text string) error {
    peer, _ := GetPeer(id)
    pText, entities, _ := parser.Parse(text, true)
    //ParseMarkdown(text, true)
    _, err := telegram.Context.SendMessage(id, &tg.MessagesSendMessageRequest{
			Message:  pText,
			Peer:     peer,
			Entities: entities,
		},
	)
	return err
}