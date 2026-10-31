package types

import "testing"

func names(q *Queue) []string {
	var out []string
	for _, e := range q.AsSlice() {
		out = append(out, e.Name)
	}
	return out
}

func TestQueueReplace(t *testing.T) {
	q := NewQueue()
	q.Add(QueueEntry{Name: "a"})
	q.Add(QueueEntry{Name: "b"})

	if q.Replace("a", QueueEntry{Name: "b"}) {
		t.Fatal("Replace onto an existing name succeeded")
	}
	if got := names(q); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("queue changed after a refused Replace: %v", got)
	}

	if q.Replace("missing", QueueEntry{Name: "c"}) {
		t.Fatal("Replace of a missing name succeeded")
	}

	if !q.Replace("a", QueueEntry{Name: "c", Version: "v2"}) {
		t.Fatal("Replace to a free name failed")
	}
	if q.Find("a") || !q.Find("c") {
		t.Fatalf("keys not updated: a=%v c=%v", q.Find("a"), q.Find("c"))
	}
	if e, ok := q.Get("c"); !ok || e.Version != "v2" {
		t.Fatalf("Get(c) = %+v, %v", e, ok)
	}

	// Keeping the same name is allowed (only the other fields change).
	if !q.Replace("c", QueueEntry{Name: "c", Version: "v3"}) {
		t.Fatal("Replace keeping the same name failed")
	}

	// Every entry can still be popped: no orphaned duplicate keys.
	for _, n := range []string{"c", "b"} {
		if _, ok := q.Pop(n); !ok {
			t.Fatalf("Pop(%q) failed", n)
		}
	}
	if q.Len() != 0 {
		t.Fatalf("queue not empty: %v", names(q))
	}
}
