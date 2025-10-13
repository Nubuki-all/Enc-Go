// Package source contains enum like properties for queue
package source

import (
	"encoding/json"
	"fmt"
)

// QueueSourceType defines the source of a queue item.
type QueueSourceType string

// Constants for the defined queue sources.
const (
	Mega     QueueSourceType = "Mega"
	Aria2    QueueSourceType = "Aria2"
	Qbit     QueueSourceType = "Qbit"
	Ytdl     QueueSourceType = "Ytdl"
	Telegram QueueSourceType = "Telegram"
)

func (qst *QueueSourceType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	switch QueueSourceType(s) {
	case Mega, Aria2, Qbit, Ytdl, Telegram:
		*qst = QueueSourceType(s)
		return nil
	default:
		return fmt.Errorf("invalid QueueSourceType: %s", s)
	}
}

func (qst QueueSourceType) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(qst))
}
