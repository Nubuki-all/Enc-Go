package types

type FeedData struct {
	Link      string
	Paused    bool
	LastFeed  string
	LastTitle string
	AllowSpam bool
	Command   string
	Chat      []FeedChat // chat id to send to
	Direct    bool       // whether to pass to fakeEventHandler
	Inf       [][]string // inclusion lists (list of lists)
	Exf       [][]string // exclusion lists
	// add other fields as needed
}

type FeedChat struct {
	ChatID  int64
	Default bool
	TopicID int
}
