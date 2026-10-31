package rssmonitor

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/mmcdole/gofeed"

	"enc/internal/bot/types"
	"enc/internal/logger"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "rssmonitor-test")
	if err != nil {
		panic(err)
	}
	if err := logger.Init(filepath.Join(dir, "log.txt"), false); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func item(title, link string) *gofeed.Item {
	return &gofeed.Item{Title: title, Link: link}
}

func TestItemLink(t *testing.T) {
	if got := ItemLink(&gofeed.Item{Link: "page"}); got != "page" {
		t.Errorf("single link: got %q, want %q", got, "page")
	}
	multi := &gofeed.Item{Link: "page", Links: []string{"page", "torrent"}}
	if got := ItemLink(multi); got != "torrent" {
		t.Errorf("several links: got %q, want %q", got, "torrent")
	}
}

func TestItemSeen(t *testing.T) {
	tests := []struct {
		name        string
		data        types.FeedData
		link, title string
		want        bool
	}{
		{"either: both match", types.FeedData{LastFeed: "l", LastTitle: "t"}, "l", "t", true},
		{"either: only title matches", types.FeedData{LastFeed: "l", LastTitle: "t"}, "l2", "t", true},
		{"either: only link matches", types.FeedData{LastFeed: "l", LastTitle: "t"}, "l", "t2", true},
		{"either: nothing matches", types.FeedData{LastFeed: "l", LastTitle: "t"}, "l2", "t2", false},

		{"both: both match", types.FeedData{MatchBoth: true, LastFeed: "l", LastTitle: "t"}, "l", "t", true},
		{"both: re-release with same title", types.FeedData{MatchBoth: true, LastFeed: "l", LastTitle: "t"}, "l2", "t", false},
		{"both: only link matches", types.FeedData{MatchBoth: true, LastFeed: "l", LastTitle: "t"}, "l", "t2", false},
		{"both: after rollback title decides", types.FeedData{MatchBoth: true, LastTitle: "t"}, "any", "t", true},
		{"both: after rollback other title", types.FeedData{MatchBoth: true, LastTitle: "t"}, "any", "t2", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := itemSeen(&tt.data, tt.link, tt.title); got != tt.want {
				t.Errorf("itemSeen(%+v, %q, %q) = %v, want %v", tt.data, tt.link, tt.title, got, tt.want)
			}
		})
	}
}

func taskURLs(tasks []feedTask) []string {
	urls := make([]string, len(tasks))
	for i, task := range tasks {
		urls[i] = task.url
	}
	return urls
}

func TestCollectFeedTasks(t *testing.T) {
	tests := []struct {
		name  string
		data  types.FeedData
		items []*gofeed.Item
		want  []string // task urls, newest first
	}{
		{
			name:  "stops at the last seen item",
			data:  types.FeedData{Command: "/leech", LastFeed: "l2", LastTitle: "t2"},
			items: []*gofeed.Item{item("t4", "l4"), item("t3", "l3"), item("t2", "l2"), item("t1", "l1")},
			want:  []string{"l4", "l3"},
		},
		{
			name:  "single new item in a one-item feed is sent",
			data:  types.FeedData{Command: "/leech", LastFeed: "old", LastTitle: "old"},
			items: []*gofeed.Item{item("new", "new")},
			want:  []string{"new"},
		},
		{
			name:  "all new without spam allowed is reset",
			data:  types.FeedData{Command: "/leech", LastFeed: "old", LastTitle: "old"},
			items: []*gofeed.Item{item("t2", "l2"), item("t1", "l1")},
			want:  []string{},
		},
		{
			name:  "all new with spam allowed includes the last item",
			data:  types.FeedData{Command: "/leech", LastFeed: "old", LastTitle: "old", AllowSpam: true},
			items: []*gofeed.Item{item("t2", "l2"), item("t1", "l1")},
			want:  []string{"l2", "l1"},
		},
		{
			name:  "match-both sends a re-release with the same title",
			data:  types.FeedData{Command: "/leech", MatchBoth: true, LastFeed: "l1", LastTitle: "Show 01"},
			items: []*gofeed.Item{item("Show 01", "l1-v2"), item("Show 01", "l1")},
			want:  []string{"l1-v2"},
		},
		{
			name:  "match-either skips a re-release with the same title",
			data:  types.FeedData{Command: "/leech", LastFeed: "l1", LastTitle: "Show 01"},
			items: []*gofeed.Item{item("Show 01", "l1-v2"), item("Show 01", "l1")},
			want:  []string{},
		},
		{
			name:  "rollback with match-both stops at the title",
			data:  types.FeedData{Command: "/leech", MatchBoth: true, LastTitle: "t2", AllowSpam: true},
			items: []*gofeed.Item{item("t3", "l3"), item("t2", "l2"), item("t1", "l1")},
			want:  []string{"l3"},
		},
		{
			name: "include and exclude filters apply",
			data: types.FeedData{
				Command: "/leech", LastFeed: "l0", LastTitle: "t0",
				Inf: [][]string{{"1080p"}},
				Exf: [][]string{{"hevc"}},
			},
			items: []*gofeed.Item{
				item("Show 1080p HEVC", "a"), item("Show 1080p", "b"), item("Show 720p", "c"), item("t0", "l0"),
			},
			want: []string{"b"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := taskURLs(collectFeedTasks("test", &tt.data, tt.items))
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCollectFeedTasksMessage(t *testing.T) {
	data := &types.FeedData{Command: "qbleech -s 1", LastFeed: "old", LastTitle: "old"}
	tasks := collectFeedTasks("test", data, []*gofeed.Item{item("new", "magnet:x")})
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(tasks))
	}
	if want := "/qbleech magnet:x -s 1"; tasks[0].msg != want {
		t.Errorf("msg = %q, want %q", tasks[0].msg, want)
	}

	data = &types.FeedData{LastFeed: "old", LastTitle: "old"}
	tasks = collectFeedTasks("test", data, []*gofeed.Item{item("new", "link")})
	if want := "Title: new\nLink: link"; len(tasks) != 1 || tasks[0].msg != want || tasks[0].url != "" {
		t.Errorf("no command: got %+v, want msg %q and no url", tasks, want)
	}
}
