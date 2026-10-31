package mega

import "testing"

func TestSafeName(t *testing.T) {
	tests := map[string]string{
		"":                "",
		"video.mkv":       "video.mkv",
		"..":              "_",
		".":               "_",
		"/":               "_",
		"../../x":         "x",
		`..\..\x`:         "x",
		"Season 1/ep.mkv": "ep.mkv",
	}
	for in, want := range tests {
		if got := safeName(in); got != want {
			t.Errorf("safeName(%q) = %q, want %q", in, got, want)
		}
	}
}
