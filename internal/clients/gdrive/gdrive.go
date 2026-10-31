// Package gdrive reads Google Drive files and folders: through the Drive API
// with a service account when one is configured, and otherwise (or when the
// service account can't see the item) as an anonymous visitor of a shared
// link, the way gdown does it.
package gdrive

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrNotShared       = errors.New("this Drive file isn't shared publicly: it needs a Google sign-in")
	ErrFolderNotShared = errors.New("this Drive folder isn't shared publicly (or doesn't exist)")
	ErrQuota           = errors.New("google Drive: too many people have viewed or downloaded this file recently; try again later")
	ErrGone            = errors.New("this Drive file doesn't exist (deleted, or the link is wrong)")
	ErrNotDrive        = errors.New("not a Google Drive link")
)

// A folder's files are listed up to MaxFiles (looking into up to maxFolders
// folders); the rest are left out.
const (
	MaxFiles   = 1000
	maxFolders = 300
)

type Kind int

const (
	File Kind = iota + 1
	Folder
)

// Link is a Drive file or folder id.
type Link struct {
	Kind Kind
	ID   string
}

// Entry is one of a folder's files.
type Entry struct {
	ID string
	// Path inside the folder, through its subfolders ("sub/file.ext").
	Path string
	// Size in bytes, or -1 when unknown.
	Size int64
	// Native is set for a Google Docs editors file, saved as an Office
	// export: "document", "spreadsheets" or "presentation".
	Native string
}

// FolderListing is a folder's name and files.
type FolderListing struct {
	Name    string
	Entries []Entry
	// Truncated is set when there were more than MaxFiles files.
	Truncated bool
}

// Resolved is a file's download, worked out: where to fetch it (with any
// headers it needs), its name and size.
type Resolved struct {
	URL    string
	Header http.Header
	Name   string
	// Size in bytes, or -1 when unknown (exports are made on request).
	Size int64
}

// Client talks to Google Drive.
type Client struct {
	http       *http.Client
	noRedirect *http.Client
	sa         *serviceAccount

	// Bases of the hosts used, so tests can point them at a local server.
	apiBase         string // www.googleapis.com
	driveBase       string // drive.google.com
	docsBase        string // docs.google.com
	usercontentBase string // drive.usercontent.google.com
}

// New returns a client. serviceAccountJSON is a service account key file's
// contents; empty means public links only.
func New(serviceAccountJSON []byte) (*Client, error) {
	c := &Client{
		http:            &http.Client{Timeout: 60 * time.Second},
		apiBase:         "https://www.googleapis.com",
		driveBase:       "https://drive.google.com",
		docsBase:        "https://docs.google.com",
		usercontentBase: "https://drive.usercontent.google.com",
	}
	c.noRedirect = &http.Client{
		Timeout: c.http.Timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	if len(serviceAccountJSON) > 0 {
		sa, err := parseServiceAccount(serviceAccountJSON)
		if err != nil {
			return nil, err
		}
		c.sa = sa
	}
	return c, nil
}

// WithBaseURL sends every request (API, drive, docs and usercontent hosts)
// to base instead of Google; for tests.
func (c *Client) WithBaseURL(base string) *Client {
	c.apiBase, c.driveBase, c.docsBase, c.usercontentBase = base, base, base, base
	return c
}

// HasServiceAccount reports whether the client uses the Drive API.
func (c *Client) HasServiceAccount() bool {
	return c.sa != nil
}

// ParseLink returns a Drive link's file or folder id. open?id= links can be
// either; they're taken as files (Identify asks Drive).
func ParseLink(raw string) (Link, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return Link{}, false
	}
	switch strings.TrimPrefix(u.Hostname(), "www.") {
	case "drive.google.com", "docs.google.com", "drive.usercontent.google.com":
	default:
		return Link{}, false
	}
	var segments []string
	for _, s := range strings.Split(u.Path, "/") {
		if s != "" {
			segments = append(segments, s)
		}
	}
	// A signed-in account's links: u/0/uc?id=…, u/1/open?id=…
	if len(segments) > 2 && segments[0] == "u" {
		segments = segments[2:]
	}
	queryID := func() (string, bool) {
		id := u.Query().Get("id")
		return id, validID(id)
	}
	after := func(marker string) (string, bool) {
		for i, s := range segments {
			if s == marker && i+1 < len(segments) && validID(segments[i+1]) {
				return segments[i+1], true
			}
		}
		return "", false
	}
	if len(segments) == 0 {
		return Link{}, false
	}
	switch {
	case segments[0] == "drive":
		// drive/folders/<id>, drive/u/0/folders/<id>, drive/mobile/folders/<id>
		if id, ok := after("folders"); ok {
			return Link{Folder, id}, true
		}
	case len(segments) == 1 && (segments[0] == "folderview" || segments[0] == "embeddedfolderview"):
		if id, ok := queryID(); ok {
			return Link{Folder, id}, true
		}
	case segments[0] == "file":
		// file/d/<id>/view, file/u/0/d/<id>
		if id, ok := after("d"); ok {
			return Link{File, id}, true
		}
	case len(segments) == 1 && (segments[0] == "uc" || segments[0] == "open" || segments[0] == "download"):
		if id, ok := queryID(); ok {
			return Link{File, id}, true
		}
	}
	return Link{}, false
}

// IsLink reports whether raw is a Drive file or folder link.
func IsLink(raw string) bool {
	_, ok := ParseLink(raw)
	return ok
}

func validID(id string) bool {
	if len(id) < 10 {
		return false
	}
	for _, c := range id {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

// Identify returns what a link is, asking Drive for open?id= links (which
// open folders too).
func (c *Client) Identify(ctx context.Context, raw string) (Link, error) {
	link, ok := ParseLink(raw)
	if !ok {
		return Link{}, ErrNotDrive
	}
	u, _ := url.Parse(strings.TrimSpace(raw))
	if link.Kind != File || strings.TrimSuffix(u.Path, "/") != "/open" {
		return link, nil
	}
	if c.sa != nil {
		if meta, err := c.apiMetadata(ctx, link.ID); err == nil && meta.MimeType == folderMime {
			return Link{Folder, link.ID}, nil
		}
	}
	if location := c.redirectOf(ctx, c.driveBase+"/open?id="+link.ID); location != "" {
		if found, ok := ParseLink(location); ok && found.Kind == Folder {
			return found, nil
		}
	}
	return link, nil
}

// Resolve works out a file's download. native is an Entry's Native ("" for
// a link to the file itself). Links to large public files don't last, so
// this should run right before each download.
func (c *Client) Resolve(ctx context.Context, id, native string) (Resolved, error) {
	if c.sa != nil {
		r, err := c.apiResolve(ctx, id)
		if !fallBack(err) {
			return r, err
		}
	}
	return c.publicResolve(ctx, id, native)
}

// List lists a folder's files, its subfolders' too (their paths keep the
// subfolders).
func (c *Client) List(ctx context.Context, id string) (*FolderListing, error) {
	if c.sa != nil {
		f, err := c.apiList(ctx, id)
		if !fallBack(err) {
			return f, err
		}
	}
	return c.publicList(ctx, id)
}

// pathPart makes a Drive title safe as one part of a path.
func pathPart(name string) string {
	var b strings.Builder
	for _, r := range name {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < 0x20 || r == 0x7f {
			b.WriteRune('_')
		} else {
			b.WriteRune(r)
		}
	}
	cleaned := strings.TrimSpace(strings.Trim(strings.TrimSpace(b.String()), "."))
	if cleaned == "" {
		return "_"
	}
	return cleaned
}

// exportExt is the Office format a Docs editors file is saved as.
func exportExt(native string) string {
	switch native {
	case "document":
		return "docx"
	case "spreadsheets":
		return "xlsx"
	case "presentation":
		return "pptx"
	}
	return ""
}

// withExt adds the export's extension unless the title already has it.
func withExt(path, native string) string {
	ext := "." + exportExt(native)
	if strings.HasSuffix(strings.ToLower(path), ext) {
		return path
	}
	return path + ext
}

func (c *Client) redirectOf(ctx context.Context, u string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return ""
	}
	resp, err := c.noRedirect.Do(req)
	if err != nil {
		return ""
	}
	_ = resp.Body.Close()
	return resp.Header.Get("Location")
}
