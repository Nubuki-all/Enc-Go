package gdrive

// Shared links without credentials, as gdown does it. A file's download link
// (drive.usercontent.google.com) answers with the file itself or, for one too
// large to scan for viruses, a warning page whose form leads to it. A folder
// lists itself in embeddedfolderview: a plain page with every entry. Google
// Docs, Sheets and Slides have no file of their own and are saved as Office
// exports.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// maxPage caps how much of an HTML page is read.
const maxPage = 4 << 20

func (c *Client) downloadURL(id string) string {
	return c.usercontentBase + "/download?id=" + url.QueryEscape(id) + "&export=download"
}

func (c *Client) exportURL(native, id string) string {
	return c.docsBase + "/" + native + "/d/" + url.PathEscape(id) + "/export?format=" + exportExt(native)
}

func (c *Client) get(ctx context.Context, client *http.Client, u, byteRange string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if byteRange != "" {
		req.Header.Set("Range", byteRange)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("couldn't reach Google Drive: %w", err)
	}
	return resp, nil
}

func readPage(resp *http.Response) string {
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, maxPage))
	return string(b)
}

func (c *Client) publicResolve(ctx context.Context, id, native string) (Resolved, error) {
	if native != "" {
		return c.publicExport(ctx, native, id)
	}
	u := c.downloadURL(id)
	resp, err := c.get(ctx, c.noRedirect, u, "bytes=0-0")
	if err != nil {
		return Resolved{}, err
	}
	if found, ok := fileResponse(u, resp); ok {
		_ = resp.Body.Close()
		return found, nil
	}
	switch status := resp.StatusCode; {
	case status >= 300 && status < 400:
		_ = resp.Body.Close()
		location := resp.Header.Get("Location")
		if strings.Contains(location, "accounts.google.com") {
			return Resolved{}, ErrNotShared
		}
		return Resolved{}, fmt.Errorf("google Drive sent it elsewhere (%s)", location)
	case status == 401 || status == 403:
		return Resolved{}, errorOr(readPage(resp), ErrNotShared)
	case status == 404 || status == 410:
		_ = resp.Body.Close()
		return Resolved{}, ErrGone
	case status == 429:
		_ = resp.Body.Close()
		return Resolved{}, ErrQuota
	case status == 416:
		// An empty file has no first byte; nor has a Docs editors file,
		// which then fails whole (500).
		_ = resp.Body.Close()
		resp, err := c.get(ctx, c.noRedirect, u, "")
		if err != nil {
			return Resolved{}, err
		}
		defer func() { _ = resp.Body.Close() }()
		if found, ok := fileResponse(u, resp); ok {
			return found, nil
		}
		if resp.StatusCode == 500 {
			return c.publicNative(ctx, id)
		}
		return Resolved{}, fmt.Errorf("google Drive: HTTP %d", resp.StatusCode)
	case status == 500:
		_ = resp.Body.Close()
		return c.publicNative(ctx, id)
	case status >= 200 && status < 300:
		page := readPage(resp)
		confirmed, ok := parseWarning(page, c.usercontentBase, c.docsBase)
		if !ok {
			return Resolved{}, errorOr(page, errors.New("google Drive sent a page instead of the file"))
		}
		resp, err := c.get(ctx, c.noRedirect, confirmed, "bytes=0-0")
		if err != nil {
			return Resolved{}, err
		}
		if found, ok := fileResponse(confirmed, resp); ok {
			_ = resp.Body.Close()
			return found, nil
		}
		return Resolved{}, errorOr(readPage(resp), errors.New("google Drive's download page has changed"))
	default:
		_ = resp.Body.Close()
		return Resolved{}, fmt.Errorf("google Drive: HTTP %d", status)
	}
}

// publicNative handles a Docs editors file (no download of its own): its
// kind from where its page is, and its export.
func (c *Client) publicNative(ctx context.Context, id string) (Resolved, error) {
	location := c.redirectOf(ctx, c.driveBase+"/open?id="+id)
	for _, kind := range []string{"document", "spreadsheets", "presentation"} {
		if strings.Contains(location, "docs.google.com/"+kind+"/d/") {
			return c.publicExport(ctx, kind, id)
		}
	}
	return Resolved{}, errors.New("google Drive couldn't send this file")
}

func (c *Client) publicExport(ctx context.Context, native, id string) (Resolved, error) {
	if exportExt(native) == "" {
		return Resolved{}, errors.New("this kind of Google file can't be downloaded")
	}
	u := c.exportURL(native, id)
	// Exports are made on request: the name only comes with the file.
	resp, err := c.get(ctx, c.http, u, "")
	if err != nil {
		return Resolved{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if found, ok := fileResponse(u, resp); ok {
		found.Size = -1
		return found, nil
	}
	switch {
	case resp.Request != nil && resp.Request.URL.Hostname() == "accounts.google.com":
		return Resolved{}, ErrNotShared
	case resp.StatusCode == 404:
		return Resolved{}, ErrGone
	}
	return Resolved{}, fmt.Errorf("google Drive couldn't export it (HTTP %d)", resp.StatusCode)
}

// fileResponse returns the file itself (an attachment), if that's what came.
func fileResponse(u string, resp *http.Response) (Resolved, bool) {
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Resolved{}, false
	}
	name := dispositionName(resp.Header.Get("Content-Disposition"))
	if name == "" {
		return Resolved{}, false
	}
	size := int64(-1)
	if r := resp.Header.Get("Content-Range"); r != "" {
		if total, err := strconv.ParseInt(r[strings.LastIndex(r, "/")+1:], 10, 64); err == nil {
			size = total
		}
	} else if resp.StatusCode != http.StatusPartialContent && resp.ContentLength >= 0 {
		size = resp.ContentLength
	}
	return Resolved{URL: u, Name: name, Size: size}, true
}

// dispositionName is the file name in a Content-Disposition header.
func dispositionName(header string) string {
	if header == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(header)
	if err != nil {
		return ""
	}
	return params["filename"]
}

// parseWarning returns the link behind a "can't scan this file for
// viruses" page: its form's action with its hidden fields, or an older
// page's "download anyway" link.
func parseWarning(page, usercontentBase, docsBase string) (string, bool) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(page))
	if err != nil {
		return "", false
	}
	if form := doc.Find("form#download-form").First(); form.Length() > 0 {
		action, ok := form.Attr("action")
		if !ok {
			return "", false
		}
		u, err := resolveRef(usercontentBase+"/", action)
		if err != nil {
			return "", false
		}
		q := u.Query()
		form.Find("input[type=hidden]").Each(func(_ int, in *goquery.Selection) {
			name, okName := in.Attr("name")
			value, okValue := in.Attr("value")
			if okName && okValue {
				q.Add(name, value)
			}
		})
		u.RawQuery = q.Encode()
		return u.String(), true
	}
	href, ok := doc.Find(`a[href*="/uc?export=download"]`).First().Attr("href")
	if !ok {
		return "", false
	}
	u, err := resolveRef(docsBase+"/", href)
	if err != nil {
		return "", false
	}
	return u.String(), true
}

func resolveRef(base, ref string) (*url.URL, error) {
	b, err := url.Parse(base)
	if err != nil {
		return nil, err
	}
	r, err := url.Parse(ref)
	if err != nil {
		return nil, err
	}
	return b.ResolveReference(r), nil
}

// pageError says what a Drive page reports went wrong, if it does.
func pageError(page string) error {
	if strings.Contains(page, "Too many users have viewed or downloaded this file") || strings.Contains(page, "Quota exceeded") {
		return ErrQuota
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(page))
	if err != nil {
		return nil
	}
	text := func(css string) string {
		return strings.Join(strings.Fields(doc.Find(css).First().Text()), " ")
	}
	if e := text(".uc-error-subcaption"); e != "" {
		return fmt.Errorf("google Drive: %s", e)
	}
	if e := text(".uc-error-caption"); e != "" {
		return fmt.Errorf("google Drive: %s", e)
	}
	title := text("title")
	if strings.Contains(title, "Sign in") || strings.Contains(title, "Sign-in") || strings.Contains(page, "accounts.google.com/ServiceLogin") {
		return ErrNotShared
	}
	return nil
}

func errorOr(page string, fallback error) error {
	if err := pageError(page); err != nil {
		return err
	}
	return fallback
}

// item is one entry of a folder page.
type item struct {
	id     string
	title  string
	folder bool
	native string // a Docs editors file's kind
}

// parseFolder reads an embeddedfolderview page: the folder's name and its
// entries (their links tell folders, files and Docs files apart).
func parseFolder(page string) (string, []item, bool) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(page))
	if err != nil || doc.Find(".flip-entries").Length() == 0 {
		// Every listing has the entries' box, even an empty one.
		return "", nil, false
	}
	name := strings.TrimSpace(doc.Find("title").First().Text())
	var items []item
	doc.Find("div.flip-entry").Each(func(_ int, entry *goquery.Selection) {
		raw, _ := entry.Attr("id")
		id, ok := strings.CutPrefix(raw, "entry-")
		if !ok || !validID(id) {
			return
		}
		href, _ := entry.Find("a").First().Attr("href")
		it := item{id: id, title: strings.TrimSpace(entry.Find(".flip-entry-title").First().Text())}
		switch {
		case strings.Contains(href, "/folders/") || strings.Contains(href, "folderview"):
			it.folder = true
		case strings.Contains(href, "docs.google.com/"):
			kind, _, _ := strings.Cut(href[strings.Index(href, "docs.google.com/")+len("docs.google.com/"):], "/")
			if kind != "uc" {
				it.native = kind
			}
		}
		items = append(items, it)
	})
	return name, items, true
}

func (c *Client) folderPage(ctx context.Context, id string) (string, []item, error) {
	resp, err := c.get(ctx, c.http, c.driveBase+"/embeddedfolderview?id="+url.QueryEscape(id), "")
	if err != nil {
		return "", nil, err
	}
	if (resp.Request != nil && resp.Request.URL.Hostname() == "accounts.google.com") ||
		resp.StatusCode == 401 || resp.StatusCode == 403 || resp.StatusCode == 404 {
		_ = resp.Body.Close()
		return "", nil, ErrFolderNotShared
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		_ = resp.Body.Close()
		return "", nil, fmt.Errorf("google Drive: HTTP %d", resp.StatusCode)
	}
	page := readPage(resp)
	name, items, ok := parseFolder(page)
	if !ok {
		switch err := pageError(page); {
		case errors.Is(err, ErrNotShared):
			return "", nil, ErrFolderNotShared
		case err != nil:
			return "", nil, err
		}
		return "", nil, errors.New("google Drive's folder page has changed")
	}
	return name, items, nil
}

// publicList lists a shared folder breadth first, so a cap leaves out the
// deepest files.
func (c *Client) publicList(ctx context.Context, id string) (*FolderListing, error) {
	name, items, err := c.folderPage(ctx, id)
	if err != nil {
		return nil, err
	}
	listing := &FolderListing{Name: name}
	type level struct {
		prefix string
		items  []item
	}
	queue := []level{{"", items}}
	folders := 1
	for len(queue) > 0 {
		lv := queue[0]
		queue = queue[1:]
		for _, it := range lv.items {
			path := lv.prefix + pathPart(it.title)
			switch {
			case it.folder && folders < maxFolders:
				folders++
				_, inside, err := c.folderPage(ctx, it.id)
				if err != nil {
					// Leave out a subfolder that can't be listed.
					continue
				}
				queue = append(queue, level{path + "/", inside})
			case it.folder:
				listing.Truncated = true
			case len(listing.Entries) >= MaxFiles:
				listing.Truncated = true
			case it.native != "":
				// Forms, drawings and sites have nothing to save.
				if exportExt(it.native) != "" {
					listing.Entries = append(listing.Entries, Entry{ID: it.id, Path: withExt(path, it.native), Size: -1, Native: it.native})
				}
			default:
				listing.Entries = append(listing.Entries, Entry{ID: it.id, Path: path, Size: -1})
			}
		}
	}
	return listing, nil
}
