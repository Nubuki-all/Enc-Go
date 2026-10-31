package types

type FeedData struct {
	Link      string
	Paused    bool
	LastFeed  string
	LastTitle string
	AllowSpam bool
	Command   string
	Chat      []FeedChat // chat to send to
	Direct    bool       // whether to pass to fakeEventHandler
	MatchBoth bool       // an item counts as seen only if title and link both match
	Inf       [][]string // inclusion lists (list of lists)
	Exf       [][]string // exclusion lists
}

type FeedChat struct {
	ChatID  int64
	Default bool
	TopicID int
}
