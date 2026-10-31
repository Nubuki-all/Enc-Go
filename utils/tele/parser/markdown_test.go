package parser

import (
	"testing"

	"github.com/gotd/td/tg"
)

func TestUnparse(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		entities []tg.MessageEntityClass
		want     string
	}{
		{
			name:     "emoji survive and offsets count UTF-16 units",
			text:     "hi 😀 bold é",
			entities: []tg.MessageEntityClass{&tg.MessageEntityBold{Offset: 6, Length: 4}},
			want:     "hi 😀 **bold** é",
		},
		{
			name:     "plain text with emoji is unchanged",
			text:     "/cmd 🎬 name",
			entities: nil,
			want:     "/cmd 🎬 name",
		},
		{
			name:     "blockquote marks every line",
			text:     "a\nb😀\nc",
			entities: []tg.MessageEntityClass{&tg.MessageEntityBlockquote{Offset: 0, Length: 7}},
			want:     "> a\n> b😀\n> c",
		},
		{
			name:     "out of range entity does not panic",
			text:     "x",
			entities: []tg.MessageEntityClass{&tg.MessageEntityBlockquote{Offset: 0, Length: 99}},
			want:     "> x",
		},
		{
			name: "text url after non-ASCII",
			text: "é link",
			entities: []tg.MessageEntityClass{
				&tg.MessageEntityTextURL{Offset: 2, Length: 4, URL: "https://x.y"},
			},
			want: "é [link](https://x.y)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Unparse(tt.text, tt.entities)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("Unparse = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseLeavesCodeAndURLsAlone(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		wantText string
		check    func(t *testing.T, ents []tg.MessageEntityClass)
	}{
		{
			name:     "inline code keeps -- and __",
			in:       "`My--File__v2--.mkv`",
			wantText: "My--File__v2--.mkv",
			check: func(t *testing.T, ents []tg.MessageEntityClass) {
				if len(ents) != 1 {
					t.Fatalf("got %d entities, want 1 code entity: %+v", len(ents), ents)
				}
				if _, ok := ents[0].(*tg.MessageEntityCode); !ok {
					t.Errorf("got %T, want *tg.MessageEntityCode", ents[0])
				}
			},
		},
		{
			name:     "pre block keeps --",
			in:       "```\na--b--c\n```",
			wantText: "a--b--c",
			check: func(t *testing.T, ents []tg.MessageEntityClass) {
				if len(ents) != 1 {
					t.Fatalf("got %d entities, want 1 pre entity: %+v", len(ents), ents)
				}
			},
		},
		{
			name:     "link target keeps ---",
			in:       "[label](https://telegra.ph/a---b--c)",
			wantText: "label",
			check: func(t *testing.T, ents []tg.MessageEntityClass) {
				u, ok := ents[0].(*tg.MessageEntityTextURL)
				if len(ents) != 1 || !ok {
					t.Fatalf("got %+v, want one text url", ents)
				}
				if u.URL != "https://telegra.ph/a---b--c" {
					t.Errorf("URL = %q", u.URL)
				}
			},
		},
		{
			name:     "formatting in a link label still works",
			in:       "[**bold** label](https://x.y/__a__)",
			wantText: "bold label",
			check: func(t *testing.T, ents []tg.MessageEntityClass) {
				var bold, url bool
				for _, e := range ents {
					switch v := e.(type) {
					case *tg.MessageEntityBold:
						bold = v.Offset == 0 && v.Length == 4
					case *tg.MessageEntityTextURL:
						url = v.URL == "https://x.y/__a__"
					}
				}
				if !bold || !url {
					t.Errorf("got %+v, want bold 0:4 and the untouched URL", ents)
				}
			},
		},
		{
			name:     "a NUL byte in the input does not hang",
			in:       "raw \x00 nul **b**",
			wantText: "raw \x00 nul b",
			check:    func(t *testing.T, ents []tg.MessageEntityClass) {},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, ents, err := Parse(tt.in, true)
			if err != nil {
				t.Fatal(err)
			}
			if text != tt.wantText {
				t.Errorf("text = %q, want %q", text, tt.wantText)
			}
			tt.check(t, ents)
		})
	}
}
