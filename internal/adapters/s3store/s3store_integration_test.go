package s3store_test

import (
	"context"
	"os"
	"testing"

	"github.com/melihemreguler/claude-code-sync/internal/adapters/blobstore/blobstoretest"
	"github.com/melihemreguler/claude-code-sync/internal/adapters/s3store"
)

// TestS3Contract runs the BlobStore contract against a real S3 bucket. It is
// skipped unless CCSYNC_S3_TEST_BUCKET is set; AWS credentials come from the
// standard config chain. Run it to live-verify the S3 backend, e.g.:
//
//	CCSYNC_S3_TEST_BUCKET=my-bucket CCSYNC_S3_TEST_REGION=us-east-1 \
//	  go test ./internal/adapters/s3store/ -run TestS3Contract -v
func TestS3Contract(t *testing.T) {
	bucket := os.Getenv("CCSYNC_S3_TEST_BUCKET")
	if bucket == "" {
		t.Skip("set CCSYNC_S3_TEST_BUCKET (and AWS credentials) to run the live S3 contract test")
	}
	store, err := s3store.New(context.Background(), bucket, "ccsync-contract-test", os.Getenv("CCSYNC_S3_TEST_REGION"))
	if err != nil {
		t.Fatalf("s3store.New: %v", err)
	}
	blobstoretest.Run(t, store)
}
