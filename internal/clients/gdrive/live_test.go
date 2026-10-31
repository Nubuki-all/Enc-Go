package gdrive

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLive talks to the real Google Drive; run with GDRIVE_LIVE=1. It uses
// gdown's public test items and only resolves the large file.
func TestLive(t *testing.T) {
	if os.Getenv("GDRIVE_LIVE") == "" {
		t.Skip("set GDRIVE_LIVE=1 to run against Google Drive")
	}
	c, _ := New(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	r, err := c.Resolve(ctx, "1l_5RK28JRL19wpT22B-DY9We3TVXnnQQ", "")
	t.Logf("large file: name=%q size=%d err=%v", r.Name, r.Size, err)
	if err != nil || r.Name == "" || r.Size < 100<<20 {
		t.Error("large file (virus-scan warning) didn't resolve")
	}

	link, err := c.Identify(ctx, "https://drive.google.com/drive/folders/1KpLl_1tcK0eeehzN980zbG-3M2nhbVks")
	if err != nil || link.Kind != Folder {
		t.Fatalf("Identify = %+v, %v", link, err)
	}
	f, err := c.List(ctx, link.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("folder %q: %d entries, truncated=%v", f.Name, len(f.Entries), f.Truncated)
	for _, e := range f.Entries {
		t.Logf("  %s (native=%q)", e.Path, e.Native)
	}
	if len(f.Entries) == 0 {
		t.Error("folder listed no files")
	}
	small := f.Entries[0]
	r, err = c.Resolve(ctx, small.ID, small.Native)
	t.Logf("first entry: name=%q size=%d err=%v", r.Name, r.Size, err)
	if err != nil {
		t.Error(err)
	}

	docs, err := c.List(ctx, "15uNXeRBIhVvZJIhL4yTw4IsStMhUaaxl")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range docs.Entries {
		if e.Native == "" {
			continue
		}
		r, err := c.Resolve(ctx, e.ID, e.Native)
		t.Logf("export %s: name=%q err=%v", e.Path, r.Name, err)
		if err != nil {
			t.Error(err)
		}
		break
	}
}
