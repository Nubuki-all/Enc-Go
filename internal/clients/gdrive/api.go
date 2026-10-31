package gdrive

// The Drive API v3 with a service account: it reaches public files and the
// private ones shared with the service account's email. The access token is
// a self-signed JWT exchanged at the account's token_uri (RFC 7523), done
// with the standard library to avoid pulling in Google's client libraries.

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	driveScope = "https://www.googleapis.com/auth/drive.readonly"
	folderMime = "application/vnd.google-apps.folder"
)

// exportMimes are the Office formats Docs editors files are exported as.
var exportMimes = map[string]struct{ native, mime string }{
	"application/vnd.google-apps.document":     {"document", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
	"application/vnd.google-apps.spreadsheet":  {"spreadsheets", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
	"application/vnd.google-apps.presentation": {"presentation", "application/vnd.openxmlformats-officedocument.presentationml.presentation"},
}

type serviceAccount struct {
	email    string
	key      *rsa.PrivateKey
	tokenURI string

	mu      sync.Mutex
	token   string
	expires time.Time
}

func parseServiceAccount(data []byte) (*serviceAccount, error) {
	var raw struct {
		Type        string `json:"type"`
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
		TokenURI    string `json:"token_uri"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("gdrive service account: %w", err)
	}
	if raw.Type != "service_account" || raw.ClientEmail == "" || raw.PrivateKey == "" {
		return nil, errors.New("gdrive service account: not a service account key file")
	}
	block, _ := pem.Decode([]byte(raw.PrivateKey))
	if block == nil {
		return nil, errors.New("gdrive service account: private_key isn't PEM")
	}
	var key *rsa.PrivateKey
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rk, ok := k.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("gdrive service account: private_key isn't an RSA key")
		}
		key = rk
	} else if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		key = k
	} else {
		return nil, fmt.Errorf("gdrive service account: private_key: %w", err)
	}
	if raw.TokenURI == "" {
		raw.TokenURI = "https://oauth2.googleapis.com/token"
	}
	return &serviceAccount{email: raw.ClientEmail, key: key, tokenURI: raw.TokenURI}, nil
}

// accessToken returns a cached token, or a new one shortly before it expires.
func (sa *serviceAccount) accessToken(ctx context.Context, client *http.Client) (string, error) {
	sa.mu.Lock()
	defer sa.mu.Unlock()
	if sa.token != "" && time.Until(sa.expires) > time.Minute {
		return sa.token, nil
	}
	now := time.Now()
	enc := base64.RawURLEncoding
	header := enc.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, _ := json.Marshal(map[string]any{
		"iss":   sa.email,
		"scope": driveScope,
		"aud":   sa.tokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	})
	signingInput := header + "." + enc.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, sa.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	form := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {signingInput + "." + enc.EncodeToString(sig)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sa.tokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("gdrive token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gdrive token: HTTP %d: %s", resp.StatusCode, bytes.TrimSpace(body))
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tok); err != nil || tok.AccessToken == "" {
		return "", fmt.Errorf("gdrive token: unexpected answer: %s", bytes.TrimSpace(body))
	}
	sa.token = tok.AccessToken
	sa.expires = now.Add(time.Duration(tok.ExpiresIn) * time.Second)
	return sa.token, nil
}

// apiError is an error answer from the Drive API.
type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("google Drive API: HTTP %d: %s", e.Status, e.Message)
}

// fallBack reports whether an API failure means the service account can't
// see the item, so the public route is worth trying.
func fallBack(err error) bool {
	var ae *apiError
	return errors.As(err, &ae) && (ae.Status == http.StatusNotFound || ae.Status == http.StatusForbidden || ae.Status == http.StatusUnauthorized)
}

func (c *Client) authHeader(ctx context.Context) (http.Header, error) {
	token, err := c.sa.accessToken(ctx, c.http)
	if err != nil {
		return nil, err
	}
	return http.Header{"Authorization": {"Bearer " + token}}, nil
}

// apiGet fetches a Drive API JSON answer into v.
func (c *Client) apiGet(ctx context.Context, path string, query url.Values, v any) error {
	header, err := c.authHeader(ctx)
	if err != nil {
		return err
	}
	query.Set("supportsAllDrives", "true")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiBase+"/drive/v3/"+path+"?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header = header
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("couldn't reach the Google Drive API: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxPage))
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(body, &e)
		return &apiError{Status: resp.StatusCode, Message: e.Error.Message}
	}
	return json.Unmarshal(body, v)
}

type apiFile struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	MimeType string `json:"mimeType"`
	Size     string `json:"size"`
}

func (f apiFile) size() int64 {
	n, err := strconv.ParseInt(f.Size, 10, 64)
	if err != nil {
		return -1
	}
	return n
}

func (c *Client) apiMetadata(ctx context.Context, id string) (apiFile, error) {
	var f apiFile
	err := c.apiGet(ctx, "files/"+url.PathEscape(id), url.Values{"fields": {"id,name,mimeType,size"}}, &f)
	return f, err
}

func (c *Client) apiResolve(ctx context.Context, id string) (Resolved, error) {
	f, err := c.apiMetadata(ctx, id)
	if err != nil {
		return Resolved{}, err
	}
	header, err := c.authHeader(ctx)
	if err != nil {
		return Resolved{}, err
	}
	base := c.apiBase + "/drive/v3/files/" + url.PathEscape(id)
	if export, ok := exportMimes[f.MimeType]; ok {
		return Resolved{
			URL:    base + "/export?mimeType=" + url.QueryEscape(export.mime),
			Header: header,
			Name:   withExt(pathPart(f.Name), export.native),
			Size:   -1,
		}, nil
	}
	if strings.HasPrefix(f.MimeType, "application/vnd.google-apps.") {
		return Resolved{}, fmt.Errorf("%s: this kind of Google file can't be downloaded", f.Name)
	}
	return Resolved{
		URL:    base + "?alt=media&supportsAllDrives=true",
		Header: header,
		Name:   f.Name,
		Size:   f.size(),
	}, nil
}

func (c *Client) apiChildren(ctx context.Context, id string) ([]apiFile, error) {
	var all []apiFile
	pageToken := ""
	for {
		q := url.Values{
			"q":                         {"'" + id + "' in parents and trashed = false"},
			"fields":                    {"nextPageToken,files(id,name,mimeType,size)"},
			"pageSize":                  {"1000"},
			"orderBy":                   {"folder,name"},
			"includeItemsFromAllDrives": {"true"},
		}
		if pageToken != "" {
			q.Set("pageToken", pageToken)
		}
		var page struct {
			NextPageToken string    `json:"nextPageToken"`
			Files         []apiFile `json:"files"`
		}
		if err := c.apiGet(ctx, "files", q, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Files...)
		if page.NextPageToken == "" {
			return all, nil
		}
		pageToken = page.NextPageToken
	}
}

// apiList lists a folder breadth first, like publicList.
func (c *Client) apiList(ctx context.Context, id string) (*FolderListing, error) {
	root, err := c.apiMetadata(ctx, id)
	if err != nil {
		return nil, err
	}
	if root.MimeType != folderMime {
		return nil, fmt.Errorf("%s isn't a folder", root.Name)
	}
	listing := &FolderListing{Name: root.Name}
	type level struct{ prefix, id string }
	queue := []level{{"", id}}
	folders := 1
	for len(queue) > 0 {
		lv := queue[0]
		queue = queue[1:]
		children, err := c.apiChildren(ctx, lv.id)
		if err != nil {
			if lv.prefix == "" {
				return nil, err
			}
			continue // leave out a subfolder that can't be listed
		}
		for _, f := range children {
			path := lv.prefix + pathPart(f.Name)
			export, isExport := exportMimes[f.MimeType]
			switch {
			case f.MimeType == folderMime && folders < maxFolders:
				folders++
				queue = append(queue, level{path + "/", f.ID})
			case f.MimeType == folderMime:
				listing.Truncated = true
			case len(listing.Entries) >= MaxFiles:
				listing.Truncated = true
			case isExport:
				listing.Entries = append(listing.Entries, Entry{ID: f.ID, Path: withExt(path, export.native), Size: -1, Native: export.native})
			case strings.HasPrefix(f.MimeType, "application/vnd.google-apps."):
				// Forms, drawings, sites, shortcuts: nothing to save.
			default:
				listing.Entries = append(listing.Entries, Entry{ID: f.ID, Path: path, Size: f.size()})
			}
		}
	}
	return listing, nil
}
