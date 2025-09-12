// Package floodwait handles telegram client floodwait errors
package floodwait

import (
	"context"
	"fmt"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/clock"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"time"

	"enc/internal/logger"
)

const FloodWaitMsg = "FLOOD_WAIT: wait %s for new request #%d"

// SimpleWaiter
type SimpleWaiter struct {
	clock clock.Clock

	maxRetries uint
	maxWait    time.Duration
}

// NewSimpleWaiter
func NewSimpleWaiter() *SimpleWaiter {
	return &SimpleWaiter{
		clock: clock.System,
	}
}

// clone
func (w *SimpleWaiter) clone() *SimpleWaiter {
	return &SimpleWaiter{
		clock:      w.clock,
		maxWait:    w.maxWait,
		maxRetries: w.maxRetries,
	}
}

// WithClock
func (w *SimpleWaiter) WithClock(c clock.Clock) *SimpleWaiter {
	w = w.clone()
	w.clock = c
	return w
}

// WithMaxRetries
func (w *SimpleWaiter) WithMaxRetries(m uint) *SimpleWaiter {
	w = w.clone()
	w.maxRetries = m
	return w
}

// WithMaxWait
func (w *SimpleWaiter) WithMaxWait(m time.Duration) *SimpleWaiter {
	w = w.clone()
	w.maxWait = m
	return w
}

// Handle
func (w *SimpleWaiter) Handle(next tg.Invoker) telegram.InvokeFunc {
	return func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		var t clock.Timer
		var retries uint
		for {
			err := next.Invoke(ctx, input, output)
			if err == nil {
				return nil
			}
			d, ok := tgerr.AsFloodWait(err)
			if !ok {
				return err
			}

			retries++
			logger.Infof(FloodWaitMsg, d, retries)

			if v := w.maxRetries; v != 0 && retries > v {
				return fmt.Errorf("flood wait retry limit exceeded (%d > %d): %w", retries, v, err)
			}

			if d == 0 {
				d = time.Second
			}

			if v := w.maxWait; v != 0 && d > v {
				return fmt.Errorf("flood wait retry limit exceeded (%d > %d): %w", d, v, err)
			}

			if t == nil {
				t = w.clock.Timer(d)
			} else {
				clock.StopTimer(t)
				t.Reset(d)
			}
			select {
			case <-t.C():
				continue
			case <-ctx.Done():
				clock.StopTimer(t)
				return ctx.Err()
			}
		}
	}
}
