package types

import (
	"sync"
	"sync/atomic"
	"time"
)

type TransferInfo struct {
	Path          string
	Sender        int64
	IsCancelled   atomic.Bool // set from cancel handlers, read by workers
	ID            string
	ExtraFlags    string
	Canceller     int64
	Time          time.Time
	PauseProgress atomic.Bool
	Mutex         sync.Mutex
}
