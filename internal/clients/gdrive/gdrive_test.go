package gdrive

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseLink(t *testing.T) {
	file := func(id string) Link { return Link{File, id} }
	folder := func(id string) Link { return Link{Folder, id} }
	tests := []struct {
		in   string
		want Link
		ok   bool
	}{
		{"https://drive.google.com/file/d/0B9P1L--7Wd2vU3VUVlFnbTgtS2c/view?usp=sharing", file("0B9P1L--7Wd2vU3VUVlFnbTgtS2c"), true},
		{"https://drive.google.com/file/u/1/d/13AhW1Z1GYGaiTpJ0Pr2TTXoQivb6jx-a", file("13AhW1Z1GYGaiTpJ0Pr2TTXoQivb6jx-a"), true},
		{"https://drive.google.com/open?id=0B9P1L--7Wd2vU3VUVlFnbTgtS2c", file("0B9P1L--7Wd2vU3VUVlFnbTgtS2c"), true},
		{"https://drive.google.com/u/0/uc?id=0B9P1L--7Wd2vU3VUVlFnbTgtS2c&export=download", file("0B9P1L--7Wd2vU3VUVlFnbTgtS2c"), true},
		{"https://docs.google.com/uc?export=download&id=1l_5RK28JRL19wpT22B-DY9We3TVXnnQQ", file("1l_5RK28JRL19wpT22B-DY9We3TVXnnQQ"), true},
		{"https://drive.usercontent.google.com/download?id=1l_5RK28JRL19wpT22B-DY9We3TVXnnQQ&export=download&confirm=t", file("1l_5RK28JRL19wpT22B-DY9We3TVXnnQQ"), true},
		{"https://drive.google.com/drive/folders/15uNXeRBIhVvZJIhL4yTw4IsStMhUaaxl?usp=sharing", folder("15uNXeRBIhVvZJIhL4yTw4IsStMhUaaxl"), true},
		{"https://drive.google.com/drive/u/0/folders/1KpLl_1tcK0eeehzN980zbG-3M2nhbVks", folder("1KpLl_1tcK0eeehzN980zbG-3M2nhbVks"), true},
		{"https://drive.google.com/embeddedfolderview?id=1KpLl_1tcK0eeehzN980zbG-3M2nhbVks#list", folder("1KpLl_1tcK0eeehzN980zbG-3M2nhbVks"), true},
		{"https://drive.google.com/drive/my-drive", Link{}, false},
		{"https://docs.google.com/document/d/1TFYNzuZJTgNGzGmjraZ58ZVOh9_YoKeBnU-opWgXQL4/edit", Link{}, false},
		{"https://example.com/file/d/0B9P1L--7Wd2vU3VUVlFnbTgtS2c/view", Link{}, false},
		{"https://drive.google.com/file/d/short/view", Link{}, false},
		{"not a url at all", Link{}, false},
	}
	for _, tt := range tests {
		got, ok := ParseLink(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("ParseLink(%q) = %+v, %v; want %+v, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

// drive.usercontent.google.com's answer for a 476 MB file, its style cut
// (from eddy's tests).
const warningPage = `<!DOCTYPE html><html><head><title>Google Drive - Virus scan warning</title></head><body><div class="uc-main"><div id="uc-text"><p class="uc-warning-caption">Google Drive can't scan this file for viruses.</p><form id="download-form" action="https://drive.usercontent.google.com/download" method="get"><input type="submit" id="uc-download-link" value="Download anyway"/><input type="hidden" name="id" value="1l_5RK28JRL19wpT22B-DY9We3TVXnnQQ"><input type="hidden" name="export" value="download"><input type="hidden" name="confirm" value="t"><input type="hidden" name="uuid" value="6c013360-8f97-49af-9ece-10d3d6ee2d17"></form></div></div></body></html>`

func TestParseWarning(t *testing.T) {
	got, ok := parseWarning(warningPage, "https://drive.usercontent.google.com", "https://docs.google.com")
	want := "https://drive.usercontent.google.com/download?confirm=t&export=download&id=1l_5RK28JRL19wpT22B-DY9We3TVXnnQQ&uuid=6c013360-8f97-49af-9ece-10d3d6ee2d17"
	if !ok || got != want {
		t.Errorf("parseWarning = %q, %v; want %q", got, ok, want)
	}
	if err := pageError(warningPage); err != nil {
		t.Errorf("pageError(warning) = %v, want nil", err)
	}
	old := `<a id="uc-download-link" href="/uc?export=download&amp;confirm=AbCd&amp;id=0B9P1L--7Wd2vU3VUVlFnbTgtS2c">Download anyway</a>`
	got, ok = parseWarning(old, "https://drive.usercontent.google.com", "https://docs.google.com")
	if want := "https://docs.google.com/uc?export=download&confirm=AbCd&id=0B9P1L--7Wd2vU3VUVlFnbTgtS2c"; !ok || got != want {
		t.Errorf("parseWarning(old) = %q, %v; want %q", got, ok, want)
	}
}

func TestPageError(t *testing.T) {
	quota := `<html><head><title>Google Drive - Quota exceeded</title></head><body><p class="uc-error-caption">Sorry, you can't view or download this file at this time.</p><p class="uc-error-subcaption">Too many users have viewed or downloaded this file recently.</p></body></html>`
	if err := pageError(quota); !errors.Is(err, ErrQuota) {
		t.Errorf("quota page: %v", err)
	}
	other := `<p class="uc-error-caption">Sorry, the file you have requested does not exist.</p>`
	if err := pageError(other); err == nil || err.Error() != "google Drive: Sorry, the file you have requested does not exist." {
		t.Errorf("error page: %v", err)
	}
	if err := pageError(`<html><head><title>Sign in - Google Accounts</title></head></html>`); !errors.Is(err, ErrNotShared) {
		t.Errorf("sign-in page: %v", err)
	}
}

// embeddedfolderview for gdown's test folder, cut down, with a Docs entry
// (from eddy's tests).
const folderPage = `<!DOCTYPE html><html><head><title>gdown_folder_test</title></head><body class="flip-embedded"><div id="flip-contents"><div class="flip-entries"><div class="flip-entry" id="entry-1aMZqPaU03E7XOQNXtjSCdguRHBaIQ82m"><div class="flip-entry-info"><a href="https://drive.google.com/drive/folders/1aMZqPaU03E7XOQNXtjSCdguRHBaIQ82m"><div class="flip-entry-title">directory-0</div></a></div></div><div class="flip-entry" id="entry-1Z2VYnXb01h-3uvEptoQ48Fo__eAn0wc1"><div class="flip-entry-info"><a href="https://drive.google.com/file/d/1Z2VYnXb01h-3uvEptoQ48Fo__eAn0wc1/view?usp=drive_web"><div class="flip-entry-title">fractal.jpg</div></a></div></div><div class="flip-entry" id="entry-14xzOzvKjP0at07jfonV7qVrTKoctFijz"><div class="flip-entry-info"><a href="https://drive.google.com/file/d/14xzOzvKjP0at07jfonV7qVrTKoctFijz/view?usp=drive_web"><div class="flip-entry-title">this is a file.txt</div></a></div></div><div class="flip-entry" id="entry-1TFYNzuZJTgNGzGmjraZ58ZVOh9_YoKeBnU-opWgXQL4"><div class="flip-entry-info"><a href="https://docs.google.com/document/d/1TFYNzuZJTgNGzGmjraZ58ZVOh9_YoKeBnU-opWgXQL4/edit?usp=drive_web"><div class="flip-entry-title">gdown</div></a></div></div></div></div></body></html>`

func TestParseFolder(t *testing.T) {
	name, items, ok := parseFolder(folderPage)
	if !ok || name != "gdown_folder_test" {
		t.Fatalf("parseFolder = %q, ok=%v", name, ok)
	}
	want := []item{
		{id: "1aMZqPaU03E7XOQNXtjSCdguRHBaIQ82m", title: "directory-0", folder: true},
		{id: "1Z2VYnXb01h-3uvEptoQ48Fo__eAn0wc1", title: "fractal.jpg"},
		{id: "14xzOzvKjP0at07jfonV7qVrTKoctFijz", title: "this is a file.txt"},
		{id: "1TFYNzuZJTgNGzGmjraZ58ZVOh9_YoKeBnU-opWgXQL4", title: "gdown", native: "document"},
	}
	if fmt.Sprint(items) != fmt.Sprint(want) {
		t.Errorf("items =\n%+v\nwant\n%+v", items, want)
	}
	empty := `<html><head><title>directory-0</title></head><body><div class="flip-entries"></div></body></html>`
	if name, items, ok := parseFolder(empty); !ok || name != "directory-0" || len(items) != 0 {
		t.Errorf("empty folder = %q %v %v", name, items, ok)
	}
	if _, _, ok := parseFolder("<html><title>Error 404 (Not Found)!!1</title></html>"); ok {
		t.Error("a 404 page parsed as a folder")
	}
}

func TestPathPart(t *testing.T) {
	for in, want := range map[string]string{
		"Show - 01.mkv": "Show - 01.mkv",
		"a/b":           "a_b",
		"..":            "_",
		" .hidden. ":    "hidden",
		`x:y*z?"<>|`:    "x_y_z_____",
		"":              "_",
	} {
		if got := pathPart(in); got != want {
			t.Errorf("pathPart(%q) = %q, want %q", in, got, want)
		}
	}
}

// fakeGoogle serves the public endpoints for a few files and folders.
func fakeGoogle(t *testing.T) *Client {
	t.Helper()
	mux := http.NewServeMux()
	var srv *httptest.Server
	attach := func(w http.ResponseWriter, name string, size int) {
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
		w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-0/%d", size))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("x"))
	}
	mux.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch q.Get("id") {
		case "smallfile0001":
			attach(w, "small.mkv", 1234)
		case "largefile0001":
			if q.Get("confirm") == "t" && q.Get("uuid") == "u-1" {
				attach(w, "Large Show - 01.mkv", 5<<30)
				return
			}
			_, _ = fmt.Fprintf(w, `<form id="download-form" action="%s/download"><input type="hidden" name="id" value="largefile0001"><input type="hidden" name="export" value="download"><input type="hidden" name="confirm" value="t"><input type="hidden" name="uuid" value="u-1"></form>`, srv.URL)
		case "privatefile01":
			w.Header().Set("Location", "https://accounts.google.com/ServiceLogin")
			w.WriteHeader(http.StatusFound)
		case "quotafile0001":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<p class="uc-error-subcaption">Too many users have viewed or downloaded this file recently.</p>`))
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("/embeddedfolderview", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("id") {
		case "rootfolder001":
			_, _ = w.Write([]byte(`<title>Season 1</title><div class="flip-entries">` +
				`<div class="flip-entry" id="entry-subfolder0001"><a href="https://drive.google.com/drive/folders/subfolder0001"><div class="flip-entry-title">Extras</div></a></div>` +
				`<div class="flip-entry" id="entry-episode000001"><a href="https://drive.google.com/file/d/episode000001/view"><div class="flip-entry-title">Ep 01.mkv</div></a></div>` +
				`<div class="flip-entry" id="entry-sheet00000001"><a href="https://docs.google.com/spreadsheets/d/sheet00000001/edit"><div class="flip-entry-title">Notes</div></a></div>` +
				`<div class="flip-entry" id="entry-form000000001"><a href="https://docs.google.com/forms/d/form000000001/edit"><div class="flip-entry-title">Survey</div></a></div>` +
				`</div>`))
		case "subfolder0001":
			_, _ = w.Write([]byte(`<title>Extras</title><div class="flip-entries"><div class="flip-entry" id="entry-ncop00000001"><a href="https://drive.google.com/file/d/ncop00000001/view"><div class="flip-entry-title">../NCOP.mkv</div></a></div></div>`))
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("/open", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("id") == "rootfolder001" {
			w.Header().Set("Location", "https://drive.google.com/drive/folders/rootfolder001")
			w.WriteHeader(http.StatusFound)
			return
		}
		http.NotFound(w, r)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	c.driveBase, c.docsBase, c.usercontentBase, c.apiBase = srv.URL, srv.URL, srv.URL, srv.URL
	return c
}

func TestPublicResolve(t *testing.T) {
	c := fakeGoogle(t)
	ctx := context.Background()

	r, err := c.Resolve(ctx, "smallfile0001", "")
	if err != nil || r.Name != "small.mkv" || r.Size != 1234 || r.Header != nil {
		t.Errorf("small file = %+v, %v", r, err)
	}
	r, err = c.Resolve(ctx, "largefile0001", "")
	if err != nil || r.Name != "Large Show - 01.mkv" || r.Size != 5<<30 || !strings.Contains(r.URL, "uuid=u-1") {
		t.Errorf("large file (virus-scan warning) = %+v, %v", r, err)
	}
	if _, err := c.Resolve(ctx, "privatefile01", ""); !errors.Is(err, ErrNotShared) {
		t.Errorf("private file: %v, want ErrNotShared", err)
	}
	if _, err := c.Resolve(ctx, "quotafile0001", ""); !errors.Is(err, ErrQuota) {
		t.Errorf("quota: %v, want ErrQuota", err)
	}
	if _, err := c.Resolve(ctx, "missingfile01", ""); !errors.Is(err, ErrGone) {
		t.Errorf("missing file: %v, want ErrGone", err)
	}
}

func TestPublicListAndIdentify(t *testing.T) {
	c := fakeGoogle(t)
	ctx := context.Background()

	link, err := c.Identify(ctx, "https://drive.google.com/open?id=rootfolder001")
	if err != nil || link != (Link{Folder, "rootfolder001"}) {
		t.Fatalf("Identify(open?id= folder) = %+v, %v", link, err)
	}
	f, err := c.List(ctx, "rootfolder001")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range f.Entries {
		got = append(got, e.Path+"|"+e.Native)
	}
	want := []string{"Ep 01.mkv|", "Notes.xlsx|spreadsheets", "Extras/_NCOP.mkv|"}
	if f.Name != "Season 1" || strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("List = %q %q, want %q", f.Name, got, want)
	}
	if _, err := c.List(ctx, "nosuchfolder1"); !errors.Is(err, ErrFolderNotShared) {
		t.Errorf("missing folder: %v", err)
	}
}

// fakeAPI serves a token endpoint and the Drive API for a service account.
func fakeAPI(t *testing.T) (*Client, *int) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	tokenCalls := 0
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		parts := strings.Split(r.PostForm.Get("assertion"), ".")
		if r.PostForm.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" || len(parts) != 3 {
			http.Error(w, "bad grant", http.StatusBadRequest)
			return
		}
		tokenCalls++
		_, _ = w.Write([]byte(`{"access_token":"tok-1","expires_in":3600}`))
	})
	files := map[string]apiFile{
		"apifolder0001": {ID: "apifolder0001", Name: "Private Show", MimeType: folderMime},
		"apisub0000001": {ID: "apisub0000001", Name: "Specials", MimeType: folderMime},
		"apiepisode001": {ID: "apiepisode001", Name: "Private Show - 01.mkv", MimeType: "video/x-matroska", Size: "42"},
		"apidoc0000001": {ID: "apidoc0000001", Name: "Script", MimeType: "application/vnd.google-apps.document"},
		"apiova0000001": {ID: "apiova0000001", Name: "OVA.mkv", MimeType: "video/x-matroska", Size: "7"},
	}
	children := map[string][]string{
		"apifolder0001": {"apisub0000001", "apiepisode001", "apidoc0000001"},
		"apisub0000001": {"apiova0000001"},
	}
	mux.HandleFunc("/drive/v3/files/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok-1" {
			http.Error(w, `{"error":{"message":"unauthenticated"}}`, http.StatusUnauthorized)
			return
		}
		f, ok := files[strings.TrimPrefix(r.URL.Path, "/drive/v3/files/")]
		if !ok {
			http.Error(w, `{"error":{"message":"File not found"}}`, http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(f)
	})
	mux.HandleFunc("/drive/v3/files", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		parent := strings.TrimSuffix(strings.TrimPrefix(q, "'"), "' in parents and trashed = false")
		var page struct {
			Files []apiFile `json:"files"`
		}
		for _, id := range children[parent] {
			page.Files = append(page.Files, files[id])
		}
		_ = json.NewEncoder(w).Encode(page)
	})
	sa, _ := json.Marshal(map[string]string{
		"type":         "service_account",
		"client_email": "bot@project.iam.gserviceaccount.com",
		"private_key":  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		"token_uri":    srv.URL + "/token",
	})
	c, err := New(sa)
	if err != nil {
		t.Fatal(err)
	}
	c.apiBase = srv.URL
	return c, &tokenCalls
}

func TestServiceAccount(t *testing.T) {
	c, tokenCalls := fakeAPI(t)
	ctx := context.Background()
	if !c.HasServiceAccount() {
		t.Fatal("service account not loaded")
	}

	r, err := c.Resolve(ctx, "apiepisode001", "")
	if err != nil || r.Name != "Private Show - 01.mkv" || r.Size != 42 ||
		!strings.HasSuffix(r.URL, "/drive/v3/files/apiepisode001?alt=media&supportsAllDrives=true") ||
		r.Header.Get("Authorization") != "Bearer tok-1" {
		t.Errorf("Resolve = %+v, %v", r, err)
	}
	r, err = c.Resolve(ctx, "apidoc0000001", "document")
	if err != nil || r.Name != "Script.docx" || !strings.Contains(r.URL, "/export?mimeType=") {
		t.Errorf("Resolve(doc) = %+v, %v", r, err)
	}

	f, err := c.List(ctx, "apifolder0001")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range f.Entries {
		got = append(got, e.Path)
	}
	if want := "Private Show - 01.mkv,Script.docx,Specials/OVA.mkv"; f.Name != "Private Show" || strings.Join(got, ",") != want {
		t.Errorf("List = %q %q, want %q", f.Name, got, want)
	}
	if *tokenCalls != 1 {
		t.Errorf("token fetched %d times, want once (cached)", *tokenCalls)
	}
}

func TestServiceAccountFallsBackToPublic(t *testing.T) {
	c, _ := fakeAPI(t)
	public := fakeGoogle(t)
	c.driveBase, c.docsBase, c.usercontentBase = public.driveBase, public.docsBase, public.usercontentBase

	// Not visible to the service account (404 from the API): the public
	// route still finds it.
	r, err := c.Resolve(context.Background(), "smallfile0001", "")
	if err != nil || r.Name != "small.mkv" || r.Header != nil {
		t.Errorf("fallback Resolve = %+v, %v", r, err)
	}
	f, err := c.List(context.Background(), "rootfolder001")
	if err != nil || f.Name != "Season 1" {
		t.Errorf("fallback List = %+v, %v", f, err)
	}
}

func TestBadServiceAccount(t *testing.T) {
	for _, in := range []string{`not json`, `{"type":"authorized_user"}`, `{"type":"service_account","client_email":"a@b","private_key":"nope"}`} {
		if _, err := New([]byte(in)); err == nil {
			t.Errorf("New(%s) succeeded", in)
		}
	}
}
