package types

// QueueKey uniquely identifies an incoming message
type QueueKey struct {
	ChatID    int64 `bson:"chat_id"`
	MessageID int   `bson:"message_id"`
}

// QueueEntry holds all the data from your Python list
type QueueEntry struct {
	Key       QueueKey   `bson:"key"`
	Name      string     `bson:"name"`
	Sender    [2]int64   `bson:"sender"`  // [sender_id, message_id]
	Version   string     `bson:"version"` // cust_v or get_v()
	Filter    FilterData `bson:"filter"`  // holds Fil1, Fil2, Fil3
	Source    [2]string  `bson:"source"`  // e.g. {"tg","None"}
	ForceName string     `bson:"force_name"`
	AniList   bool       `bson:"anilist"` // true/false
	URI       string     `bson:"uri"`     // generic URI
}

// Queue is a FIFO container with fast lookup
type Queue struct {
	entries []QueueEntry
	idx     map[QueueKey]int
}

// NewQueue creates an empty, initialized queue
func NewQueue() *Queue {
	return &Queue{
		entries: make([]QueueEntry, 0),
		idx:     make(map[QueueKey]int),
	}
}

// Enqueue appends e if not already in queue
func (q *Queue) Enqueue(e QueueEntry) {
	if _, exists := q.idx[e.Key]; exists {
		return
	}
	q.entries = append(q.entries, e)
	q.idx[e.Key] = len(q.entries) - 1
}

// Dequeue pops and returns the oldest entry, false if empty
func (q *Queue) Dequeue() (QueueEntry, bool) {
	if len(q.entries) == 0 {
		return QueueEntry{}, false
	}
	e := q.entries[0]
	q.entries = q.entries[1:]
	// rebuild index
	q.idx = make(map[QueueKey]int, len(q.entries))
	for i, ent := range q.entries {
		q.idx[ent.Key] = i
	}
	return e, true
}

// AsSlice returns a copy of the queue entries in order
func (q *Queue) AsSlice() []QueueEntry {
	dup := make([]QueueEntry, len(q.entries))
	copy(dup, q.entries)
	return dup
}

// LoadFromSlice replaces the queue’s contents from slice
func (q *Queue) LoadFromSlice(slice []QueueEntry) {
	q.entries = make([]QueueEntry, len(slice))
	copy(q.entries, slice)
	q.idx = make(map[QueueKey]int, len(slice))
	for i, ent := range q.entries {
		q.idx[ent.Key] = i
	}
}
