package gdrivestore_test

import (
	"context"
	"os"
	"testing"

	"github.com/melihemreguler/claude-code-sync/internal/adapters/blobstore/blobstoretest"
	"github.com/melihemreguler/claude-code-sync/internal/adapters/gdrivestore"
)

// TestGDriveContract runs the BlobStore contract against a real Google Drive
// folder. It is skipped unless CCSYNC_GDRIVE_TEST_FOLDER and
// CCSYNC_GDRIVE_TEST_CREDENTIALS are set. The first run opens a browser consent
// flow and caches a token at CCSYNC_GDRIVE_TEST_TOKEN (or ./gdrive-test-token.json).
//
//	CCSYNC_GDRIVE_TEST_FOLDER=<folderId> \
//	CCSYNC_GDRIVE_TEST_CREDENTIALS=client_secret.json \
//	  go test ./internal/adapters/gdrivestore/ -run TestGDriveContract -v
func TestGDriveContract(t *testing.T) {
	folder := os.Getenv("CCSYNC_GDRIVE_TEST_FOLDER")
	creds := os.Getenv("CCSYNC_GDRIVE_TEST_CREDENTIALS")
	if folder == "" || creds == "" {
		t.Skip("set CCSYNC_GDRIVE_TEST_FOLDER and CCSYNC_GDRIVE_TEST_CREDENTIALS to run the live Drive contract test")
	}
	token := os.Getenv("CCSYNC_GDRIVE_TEST_TOKEN")
	if token == "" {
		token = "gdrive-test-token.json"
	}
	store, err := gdrivestore.New(context.Background(), folder, creds, token)
	if err != nil {
		t.Fatalf("gdrivestore.New: %v", err)
	}
	blobstoretest.Run(t, store)
}
