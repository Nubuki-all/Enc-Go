package transcode

import (
	"strings"
	"testing"
)

func TestFormatArgsEscapesTitles(t *testing.T) {
	args := `ffmpeg -i {input} -metadata title='This Episode' -metadata comment="Fileinfo" {output}`
	got := formatArgs(args, `JoJo's $(touch /tmp/x)`, "01", "", `Name "v2" `+"`id`", "")

	for _, bad := range []string{"'s", "$(", `"v2"`, "`id`"} {
		if strings.Contains(got, bad) {
			t.Errorf("formatArgs left %q in %q", bad, got)
		}
	}
	if !strings.Contains(got, "title='Episode 01 of JoJo’s (touch /tmp/x)'") {
		t.Errorf("title not substituted as expected: %q", got)
	}
	if !strings.Contains(got, `comment="Name ”v2” ’id’"`) {
		t.Errorf("Fileinfo not substituted as expected: %q", got)
	}
}
