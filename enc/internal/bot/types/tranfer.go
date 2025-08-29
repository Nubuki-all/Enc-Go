package types

import "time"

type TransferInfo struct {
	Path          string
	Sender        string
	IsCancelled   bool
	ID            string
	Canceller     int64
	Time          time.Time
	PauseProgress bool
}