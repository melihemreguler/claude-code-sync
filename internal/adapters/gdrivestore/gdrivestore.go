// Package gdrivestore implements blobstore.BlobStore on top of Google Drive.
//
// Objects are stored as flat files inside a single Drive folder, named by their
// relative path (e.g. "objects/<hash>/<file>.age"); Drive treats the slash as a
// literal character, so no folder hierarchy is needed. The file's md5Checksum is
// used as the content version. The app uses the least-privilege drive.file scope,
// so it can only see the files it created.
package gdrivestore

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

// Store is a Google Drive-backed blob store rooted at a folder.
type Store struct {
	svc      *drive.Service
	folderID string
	ids      map[string]string // rel -> Drive file id (warmed by List)
}

// New authenticates with Google Drive and returns a Store. credentialsPath is an
// OAuth client secret JSON; tokenPath caches the user authorization (created via
// an interactive consent flow the first time).
func New(ctx context.Context, folderID, credentialsPath, tokenPath string) (*Store, error) {
	client, err := loadClient(ctx, credentialsPath, tokenPath)
	if err != nil {
		return nil, err
	}
	svc, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, err
	}
	return &Store{svc: svc, folderID: folderID, ids: map[string]string{}}, nil
}

// List returns every object in the folder as rel -> md5 checksum.
func (s *Store) List(ctx context.Context) (map[string]string, error) {
	out := map[string]string{}
	q := fmt.Sprintf("'%s' in parents and trashed = false", s.folderID)
	call := s.svc.Files.List().Q(q).Fields("nextPageToken, files(id, name, md5Checksum)").Context(ctx)
	for {
		page, err := call.Do()
		if err != nil {
			return nil, err
		}
		for _, f := range page.Files {
			out[f.Name] = f.Md5Checksum
			s.ids[f.Name] = f.Id
		}
		if page.NextPageToken == "" {
			break
		}
		call = call.PageToken(page.NextPageToken)
	}
	return out, nil
}

// Get downloads an object's contents.
func (s *Store) Get(ctx context.Context, rel string) ([]byte, error) {
	id, err := s.idFor(ctx, rel)
	if err != nil {
		return nil, err
	}
	resp, err := s.svc.Files.Get(id).Context(ctx).Download()
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// Put creates or updates an object.
func (s *Store) Put(ctx context.Context, rel string, data []byte) error {
	if id, ok := s.ids[rel]; ok {
		_, err := s.svc.Files.Update(id, &drive.File{}).Media(bytes.NewReader(data)).Context(ctx).Do()
		return err
	}
	created, err := s.svc.Files.Create(&drive.File{
		Name:    rel,
		Parents: []string{s.folderID},
	}).Media(bytes.NewReader(data)).Context(ctx).Do()
	if err != nil {
		return err
	}
	s.ids[rel] = created.Id
	return nil
}

// Delete removes an object from the folder. A key that is already absent is
// treated as success.
func (s *Store) Delete(ctx context.Context, rel string) error {
	id, ok := s.ids[rel]
	if !ok {
		if _, err := s.List(ctx); err != nil {
			return err
		}
		id, ok = s.ids[rel]
		if !ok {
			return nil // already gone
		}
	}
	if err := s.svc.Files.Delete(id).Context(ctx).Do(); err != nil {
		return err
	}
	delete(s.ids, rel)
	return nil
}

// Exists reports whether an object is present in the folder.
func (s *Store) Exists(ctx context.Context, rel string) (bool, error) {
	if _, ok := s.ids[rel]; ok {
		return true, nil
	}
	list, err := s.List(ctx)
	if err != nil {
		return false, err
	}
	_, ok := list[rel]
	return ok, nil
}

func (s *Store) idFor(ctx context.Context, rel string) (string, error) {
	if id, ok := s.ids[rel]; ok {
		return id, nil
	}
	if _, err := s.List(ctx); err != nil {
		return "", err
	}
	id, ok := s.ids[rel]
	if !ok {
		return "", fmt.Errorf("drive object not found: %s", rel)
	}
	return id, nil
}

// --- OAuth helpers ---

func loadClient(ctx context.Context, credentialsPath, tokenPath string) (*http.Client, error) {
	raw, err := os.ReadFile(credentialsPath)
	if err != nil {
		return nil, fmt.Errorf("reading Drive credentials: %w", err)
	}
	conf, err := google.ConfigFromJSON(raw, drive.DriveFileScope)
	if err != nil {
		return nil, fmt.Errorf("parsing Drive credentials: %w", err)
	}
	tok, err := tokenFromFile(tokenPath)
	if err != nil {
		tok, err = tokenFromWeb(ctx, conf)
		if err != nil {
			return nil, err
		}
		if err := saveToken(tokenPath, tok); err != nil {
			return nil, err
		}
	}
	return conf.Client(ctx, tok), nil
}

func tokenFromFile(path string) (*oauth2.Token, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	return tok, json.NewDecoder(f).Decode(tok)
}

func saveToken(path string, tok *oauth2.Token) error {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(tok)
}

func tokenFromWeb(ctx context.Context, conf *oauth2.Config) (*oauth2.Token, error) {
	authURL := conf.AuthCodeURL("state", oauth2.AccessTypeOffline)
	fmt.Printf("Authorize ccsync in your browser, then paste the code:\n\n  %s\n\ncode: ", authURL)
	code, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return nil, err
	}
	tok, err := conf.Exchange(ctx, trimNewline(code))
	if err != nil {
		return nil, fmt.Errorf("exchanging auth code: %w", err)
	}
	return tok, nil
}

func trimNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
