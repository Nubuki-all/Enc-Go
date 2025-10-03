package types

import "time"

type TransferInfo struct {
	Path          string
	Sender        int64
	IsCancelled   bool
	ID            string
	ExtraFlags    string
	Canceller     int64
	Time          time.Time
	PauseProgress bool
}
