package types

import (
	"slices"
	"strconv"
	"strings"
	"sync"

	"enc/internal/bot/source"
)

// QueueKey uniquely identifies an incoming message.
type QueueKey struct {
	ChatID    int64 `bson:"chat_id"`
	MessageID int   `bson:"message_id"`
}

// FileMeta holds metadata about a file.
type FileMeta struct {
	Type       string `bson:"type"` // "document" or "video"
	DocumentID int64  `bson:"document_id,omitempty"`
	AccessHash int64  `bson:"access_hash,omitempty"`
	FileRef    []byte `bson:"file_ref,omitempty"` // file_reference byte slice
	FileName   string `bson:"file_name,omitempty"`
	MimeType   string `bson:"mime_type,omitempty"`
	Size       int64  `bson:"size,omitempty"`
}

// QueueSource defines the source of a queue item.
type QueueSource struct {
	Source source.QueueSourceType
	Select string
}

// BatchItem represents an item in a batch.
type BatchItem struct {
	FileName string
	ID       string
	Status   int
}

// QueueEntry holds all the data for a queue item.
type QueueEntry struct {
	// Key       QueueKey     `bson:"key"`
	Name      string      `bson:"name"`
	ChatID    int64       `bson:"chat_id"`
	MessageID int         `bson:"message_id"`
	Sender    int64       `bson:"sender"`  // sender_id
	Version   string      `bson:"version"` // cust_v or get_v()
	Filter    *FilterData `bson:"filter"`  // holds Fil1, Fil2, Fil3
	Source    QueueSource `bson:"source"`  // e.g. {"tg","None"}
	ForceName string      `bson:"force_name"`
	AniList   bool        `bson:"anilist"` // true/false
	URI       string      `bson:"uri"`     // generic URI
	Doc       *FileMeta   `bson:"file_meta,omitempty"`
	Batch     []BatchItem `bson:"batch"`
}

// Queue is a FIFO container implemented with a slice.
type Queue struct {
	entries   []QueueEntry
	keysExist map[string]struct{}
	mu        sync.RWMutex
	waiters   map[int64]chan struct{}
}

// NewQueue creates an empty, initialized queue.
func NewQueue() *Queue {
	return &Queue{
		entries:   make([]QueueEntry, 0),
		keysExist: make(map[string]struct{}),
		waiters:   make(map[int64]chan struct{}),
	}
}

// Add appends a new entry to the end of the queue if it does not already exist.
// and returns the index position of the added entry
func (q *Queue) Add(entry QueueEntry) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	if _, exists := q.keysExist[entry.Name]; exists {
		return -1
	}

	q.entries = append(q.entries, entry)
	q.keysExist[entry.Name] = struct{}{}
	go q.notifyWaiters()
	return (len(q.entries) - 1)
}

// Find checks if an entry with the given name exists.
func (q *Queue) Find(key string) bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	if _, exists := q.keysExist[key]; exists {
		return true
	}
	return false
}

// Update finds an entry by its key and updates it. If the entry does not exist,
// it is a no-op
func (q *Queue) Update(entry QueueEntry) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if _, exists := q.keysExist[entry.Name]; !exists {
		return
	}
	for i := range q.entries {
		if q.entries[i].Name == entry.Name {
			q.entries[i] = entry
			go q.notifyWaiters()
			return
		}
	}
}

// Replace updates an entry along with its name. It reports false if name is
// not queued or another entry already uses entry.Name.
func (q *Queue) Replace(name string, entry QueueEntry) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if _, exists := q.keysExist[name]; !exists {
		return false
	}
	if _, taken := q.keysExist[entry.Name]; taken && entry.Name != name {
		return false
	}
	for i := range q.entries {
		if q.entries[i].Name == name {
			delete(q.keysExist, name)
			q.keysExist[entry.Name] = struct{}{}
			q.entries[i] = entry
			go q.notifyWaiters()
			return true
		}
	}
	return false
}

// Get finds an entry by its key and returns a copy.
func (q *Queue) Get(key string) (QueueEntry, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if _, exists := q.keysExist[key]; !exists {
		return QueueEntry{}, false
	}

	for _, entry := range q.entries {
		if entry.Name == key {
			return entry, true
		}
	}
	return QueueEntry{}, false
}

// GetByIndex finds an entry by its index and returns a copy.
func (q *Queue) GetByIndex(idx int) (QueueEntry, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if idx < 0 || idx >= len(q.entries) {
		return QueueEntry{}, false
	}
	return q.entries[idx], true
}

// LockForSave locks the queue and performs a zero-copy of its contents.
// It returns the contents and a closure to be called after the save completes.
func (q *Queue) LockForSave() (map[string]struct{}, []QueueEntry, func()) {
	q.mu.Lock()

	keys := q.keysExist
	slice := q.entries

	// The unlock function returns the data to the queue if the save fails.
	unlock := func() {
		q.mu.Unlock()
	}

	return keys, slice, unlock
}

// AsMapSlice returns a copy of the queue keys and queue entries.
func (q *Queue) AsMapSlice() (map[string]struct{}, []QueueEntry) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	dup := make([]QueueEntry, len(q.entries))
	dupm := make(map[string]struct{}, len(q.keysExist))
	copy(dup, q.entries)
	for n := range q.keysExist {
		dupm[n] = struct{}{}
	}
	return dupm, dup
}

// AsSlice returns a copy of the queue entries in order.
func (q *Queue) AsSlice() []QueueEntry {
	q.mu.RLock()
	defer q.mu.RUnlock()
	dup := make([]QueueEntry, len(q.entries))
	copy(dup, q.entries)
	return dup
}

// Slice returns the internal slice.
// WARNING: The returned slice is a reference to the internal data. Modifying it
// externally can lead to race conditions. Use `AsSlice()` for a safe copy.
func (q *Queue) Slice() []QueueEntry {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.entries
}

// Pop removes and returns an entry by its key. This is an O(n) operation.
func (q *Queue) Pop(key string) (QueueEntry, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Check if the key exists before iterating
	if _, ok := q.keysExist[key]; !ok {
		return QueueEntry{}, false
	}

	// Iterate to find and remove from the slice
	for i, entry := range q.entries {
		if entry.Name == key {
			// Remove the entry from the slice
			q.entries = append(q.entries[:i], q.entries[i+1:]...)
			// Remove the key from the map
			delete(q.keysExist, key)
			go q.notifyWaiters()
			return entry, true
		}
	}

	return QueueEntry{}, false
}

// Remove removes and returns the entry at a specific index..
func (q *Queue) Remove(index int) (QueueEntry, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if index < 0 || index >= len(q.entries) {
		return QueueEntry{}, false
	}

	entry := q.entries[index]
	// Remove the key from the existence map
	delete(q.keysExist, entry.Name)
	// Remove the entry from the slice
	q.entries = append(q.entries[:index], q.entries[index+1:]...)
	go q.notifyWaiters()
	return entry, true
}

// RemoveItems remove all entries for the given indexes verbosely
func (q *Queue) RemoveItems(indexes []int, requester int64, md bool) string {
	q.mu.Lock()
	defer q.mu.Unlock()
	var sb strings.Builder
	total := len(q.entries)
	slices.Reverse(indexes)
	for _, idx := range indexes {
		if idx < 0 || idx >= total {
			continue
		}
		entry := q.entries[idx]
		if requester != 0 && entry.Sender != requester {
			continue
		}
		delete(q.keysExist, entry.Name)
		q.entries = append(q.entries[:idx], q.entries[idx+1:]...)
		if md {
			sb.WriteString(strconv.Itoa(idx) + ". `" + entry.Name + "`\n")
		} else {
			sb.WriteString(strconv.Itoa(idx) + ". " + entry.Name + "\n")
		}
	}
	if sb.Len() != 0 {
		go q.notifyWaiters()
	}
	return sb.String()
}

// Clear clears all queue entries
func (q *Queue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.entries = make([]QueueEntry, 0)
	q.keysExist = make(map[string]struct{})

	go q.notifyWaiters()
}

// LoadFromSlice replaces the queue’s contents from a provided slice.
func (q *Queue) LoadFromSlice(slice []QueueEntry) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Create a new slice and copy the content to ensure isolation.
	q.entries = make([]QueueEntry, len(slice))
	copy(q.entries, slice)

	// Rebuild the keysExist map from the new slice
	q.keysExist = make(map[string]struct{}, len(slice))
	for _, item := range q.entries {
		q.keysExist[item.Name] = struct{}{}
	}
}

// LoadFromMapSlice replaces the queue's contents from a provided slice and map.
func (q *Queue) LoadFromMapSlice(keys map[string]struct{}, slice []QueueEntry) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Replace the old slice with the new one.
	q.entries = slice

	// Replace the old map with the new one.
	q.keysExist = keys
}

// Len returns the length of the queue
func (q *Queue) Len() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.entries)
}

// Subscribe registers a channel to update when queue is modified
// For wake events.
func (q *Queue) Subscribe(ChatID int64, ch chan struct{}) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.waiters[ChatID] = ch
}

// UnSubscribe unregisters a channel previously registered using Subscribe
func (q *Queue) UnSubscribe(ChatID int64, ch chan struct{}) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.waiters[ChatID] == ch {
		delete(q.waiters, ChatID)
	}
}

// notifyWaiters notifies all Subscribed waiters.
// Must be go(ed) to avoid deadlocks.
func (q *Queue) notifyWaiters() {
	q.mu.RLock()
	defer q.mu.RUnlock()
	for _, ch := range q.waiters {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
