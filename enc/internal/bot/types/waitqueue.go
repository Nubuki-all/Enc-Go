package types

import (
	"context"
	"errors"
	"sync"

	"github.com/gotd/td/telegram/message/markup"
	"github.com/gotd/td/tg"

	"enc/utils/tele"
)

// WaitQueue is a concurrency-safe FIFO queue of turn IDs.
// It notifies registered waiters via per-wait channels when the queue changes.
type WaitQueue struct {
	callBackData string
	mu           sync.Mutex
	items        []string
	waiters      map[string][]chan struct{} // per-turnID list of waiter channels
}

// New constructs a WaitQueue.
func NewWaitQueue(cBData string) *WaitQueue {
	return &WaitQueue{
		callBackData: cBData,
		waiters:      make(map[string][]chan struct{}),
	}
}

// notifyAll sends a non-blocking notification to every registered waiter channel.
// Must be called with q.mu held.
func (q *WaitQueue) notifyAllLocked() {
	for _, chans := range q.waiters {
		for _, ch := range chans {
			// non-blocking send; channels are buffered size 1
			select {
			case ch <- struct{}{}:
			default:
			}
		}
	}
}

// Enqueue appends id to the queue and notifies waiters.
func (q *WaitQueue) Enqueue(id string) {
	q.mu.Lock()
	q.items = append(q.items, id)
	q.notifyAllLocked()
	q.mu.Unlock()
}

// Remove removes the first occurrence of id from the queue.
// Returns true if removed. Notifies waiters.
func (q *WaitQueue) Remove(id string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, v := range q.items {
		if v == id {
			q.items = append(q.items[:i], q.items[i+1:]...)
			q.notifyAllLocked()
			return true
		}
	}
	return false
}

func (q *WaitQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

func (q *WaitQueue) Front() string {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return ""
	}
	return q.items[0]
}

func (q *WaitQueue) Contains(id string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.containsLocked(id)
}

func (q *WaitQueue) containsLocked(id string) bool {
	for _, v := range q.items {
		if v == id {
			return true
		}
	}
	return false
}

// Waiting reports whether there are more than one item in the queue.
func (q *WaitQueue) Waiting() bool {
	return q.Len() > 1
}

// WaitForTurn blocks until one of the following happens:
//   - turnID reaches the front of the queue -> returns (1, nil)
//   - turnID is removed from the queue -> returns (0, error)
//   - ctx is cancelled -> returns (0, ctx.Err())
//
// This implementation registers a per-wait buffered channel and waits on it plus ctx.
// No helper goroutine is created per waiter.
func (q *WaitQueue) WaitForTurn(ctx context.Context, turnID string, msg *tele.Message) (int, error) {
	// register waiter
	q.mu.Lock()
	if !q.containsLocked(turnID) {
		q.mu.Unlock()
		return 0, errors.New("turn id not in queue")
	}
	// immediate success
	if len(q.items) > 0 && q.items[0] == turnID {
		q.mu.Unlock()
		return 1, nil
	}

	ch := make(chan struct{}, 1)
	q.waiters[turnID] = append(q.waiters[turnID], ch)
	q.mu.Unlock()

	// cleanup routine to deregister the channel when this function exits
	defer func() {
		q.mu.Lock()
		// find and remove 'ch' from q.waiters[turnID]
		list := q.waiters[turnID]
		for i, c := range list {
			if c == ch {
				// remove
				list = append(list[:i], list[i+1:]...)
				break
			}
		}
		if len(list) == 0 {
			delete(q.waiters, turnID)
		} else {
			q.waiters[turnID] = list
		}
		q.mu.Unlock()
	}()
	buttonRows := []tg.KeyboardButtonRow{
		markup.Row(
			markup.Callback(
				"Cancel",
				[]byte(q.callBackData+" "+turnID),
			),
		),
	}
	_, _ = msg.Edit(
		"`Waiting till previous process gets completed.`",
		&tele.EditOptions{
			ReplyMarkup: markup.InlineKeyboard(buttonRows...),
		},
	)

	// wait loop: react to notifications or ctx cancellation
	for {
		select {
		case <-ch:
			// re-check queue state
			q.mu.Lock()
			if len(q.items) > 0 && q.items[0] == turnID {
				q.mu.Unlock()
				return 1, nil
			}
			if !q.containsLocked(turnID) {
				q.mu.Unlock()
				return 0, errors.New("turn removed from queue")
			}
			q.mu.Unlock()
			// otherwise continue waiting
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
}
