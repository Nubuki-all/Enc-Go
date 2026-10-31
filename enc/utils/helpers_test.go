package utils

import (
	"path/filepath"
	"slices"
	"testing"

	"enc/config"
	"enc/internal/bot"
)

func TestUserPermissionsRejectSenderZero(t *testing.T) {
	owners, dev, temp := config.Rconf.Owners, config.Conf.Dev, bot.TempUsers
	t.Cleanup(func() {
		config.Rconf.Owners, config.Conf.Dev, bot.TempUsers = owners, dev, temp
	})

	// The worst case: DEV unset (0) and 0 present in both lists.
	config.Rconf.Owners = []int64{0, 10}
	config.Conf.Dev = 0
	bot.TempUsers = []int64{0, 20}

	for name, check := range map[string]func(int64) bool{
		"UserIsOwner":   UserIsOwner,
		"UserIsTemp":    UserIsTemp,
		"UserIsDev":     UserIsDev,
		"UserIsAllowed": UserIsAllowed,
	} {
		if check(0) {
			t.Errorf("%s(0) = true, want false", name)
		}
	}
	if !UserIsOwner(10) || !UserIsTemp(20) || !UserIsAllowed(10) || !UserIsAllowed(20) {
		t.Error("real users lost their permissions")
	}
	config.Conf.Dev = 30
	if !UserIsDev(30) {
		t.Error("UserIsDev(30) = false with DEV=30")
	}
}

func TestRange2Slice(t *testing.T) {
	tests := []struct {
		in   string
		want []int
	}{
		{"2-4", []int{2, 3, 4}},
		{" 1 - 2 ", []int{1, 2}},
		{"3-3", []int{3}},
		{"4-2", nil},
		{"-1-3", nil},
		{"0-2000000000", nil}, // must not try to allocate this
		{"0-10000", nil},      // 10001 items: over the limit
		{"a-3", nil},
		{"5", nil},
	}
	for _, tt := range tests {
		if got := Range2Slice(tt.in); !slices.Equal(got, tt.want) {
			t.Errorf("Range2Slice(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
	if got := len(Range2Slice("0-9999")); got != 10000 {
		t.Errorf("Range2Slice(\"0-9999\") has %d items, want 10000", got)
	}
	if got := Range2StringSlice("7-8"); !slices.Equal(got, []string{"7", "8"}) {
		t.Errorf("Range2StringSlice(\"7-8\") = %v", got)
	}
	if got := Range2StringSlice("0-2000000000"); got != nil {
		t.Errorf("Range2StringSlice huge range = %d items, want none", len(got))
	}
}

func TestSafeFileName(t *testing.T) {
	tests := map[string]string{
		"video.mkv":         "video.mkv",
		"../../.env":        ".env",
		"/etc/passwd":       "passwd",
		`..\..\win.ini`:     "win.ini",
		"dir/sub/file.mp4":  "file.mp4",
		"..":                "",
		".":                 "",
		"":                  "",
		"/":                 "",
		"Show - 01 [1080p]": "Show - 01 [1080p]",
	}
	for in, want := range tests {
		if got := SafeFileName(in); got != want {
			t.Errorf("SafeFileName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsInsideDir(t *testing.T) {
	dir := filepath.Join("downloads")
	tests := []struct {
		path string
		want bool
	}{
		{filepath.Join(dir, "a.mkv"), true},
		{filepath.Join(dir, "sub", "a.mkv"), true},
		{filepath.Join(dir, "..a.mkv"), true},
		{dir, false},
		{filepath.Join(dir, ".."), false},
		{filepath.Join(dir, "..", "x"), false},
		{filepath.Join(dir, "..", "..", ".env"), false},
	}
	for _, tt := range tests {
		if got := IsInsideDir(dir, tt.path); got != tt.want {
			t.Errorf("IsInsideDir(%q, %q) = %v, want %v", dir, tt.path, got, tt.want)
		}
	}
}

func TestShellSafeText(t *testing.T) {
	got := ShellSafeText("JoJo's \"Bizarre\" `x` $(rm -rf ~) \\n")
	want := "JoJo’s ”Bizarre” ’x’ (rm -rf ~) n"
	if got != want {
		t.Errorf("ShellSafeText = %q, want %q", got, want)
	}
	for _, c := range []string{"'", `"`, "`", "$", `\`} {
		if slices.Contains([]rune(got), []rune(c)[0]) {
			t.Errorf("ShellSafeText left %q in %q", c, got)
		}
	}
}

func TestSplitFlags(t *testing.T) {
	in := `ffmpeg -i {input} -map_metadata 0 -metadata title='A - B' -metadata:s:a:0 title="x" -c copy {output}`
	want := []string{
		"ffmpeg ",
		"-i {input} ",
		"-map_metadata 0 ",
		"-metadata title='A - B' ",
		`-metadata:s:a:0 title="x" `,
		"-c copy {output}",
	}
	if got := SplitFlags(in); !slices.Equal(got, want) {
		t.Errorf("SplitFlags =\n%q\nwant\n%q", got, want)
	}
	if got := SplitFlags(""); !slices.Equal(got, []string{""}) {
		t.Errorf("SplitFlags(\"\") = %q", got)
	}
}

func TestTruncateText(t *testing.T) {
	tests := []struct {
		in   string
		n    int
		want string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"abcdefghijk", 10, "abcdefghij…"},
		{"日本語のタイトル", 3, "日本語…"}, // must not split a multi-byte rune
		{"😀😀😀", 2, "😀😀…"},
	}
	for _, tt := range tests {
		if got := TruncateText(tt.in, tt.n); got != tt.want {
			t.Errorf("TruncateText(%q, %d) = %q, want %q", tt.in, tt.n, got, tt.want)
		}
	}
}
