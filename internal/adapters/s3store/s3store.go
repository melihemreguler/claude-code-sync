// Package s3store implements blobstore.BlobStore on top of Amazon S3 (or any
// S3-compatible service). Credentials and region come from the standard AWS
// config chain (env, shared config, IAM role). Object ETags are used as the
// content version; this assumes small, single-part, non-KMS objects (true for
// ccsync's encrypted session files), for which the ETag is the content MD5.
package s3store

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// Store is an S3-backed blob store rooted at a bucket and key prefix.
type Store struct {
	client *s3.Client
	bucket string
	prefix string
}

// New builds a Store. region may be empty to defer to the AWS config chain.
func New(ctx context.Context, bucket, prefix, region string) (*Store, error) {
	var opts []func(*awsconfig.LoadOptions) error
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return &Store{
		client: s3.NewFromConfig(cfg),
		bucket: bucket,
		prefix: strings.Trim(prefix, "/"),
	}, nil
}

func (s *Store) key(rel string) string {
	if s.prefix == "" {
		return rel
	}
	return path.Join(s.prefix, rel)
}

func (s *Store) rel(key string) string {
	if s.prefix == "" {
		return key
	}
	return strings.TrimPrefix(key, s.prefix+"/")
}

// List returns every object under the prefix as rel -> content MD5 (ETag).
func (s *Store) List(ctx context.Context) (map[string]string, error) {
	out := map[string]string{}
	p := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(s.prefix),
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, obj := range page.Contents {
			rel := s.rel(aws.ToString(obj.Key))
			if rel == "" {
				continue
			}
			out[rel] = strings.Trim(aws.ToString(obj.ETag), `"`)
		}
	}
	return out, nil
}

// Get downloads an object's contents.
func (s *Store) Get(ctx context.Context, rel string) ([]byte, error) {
	resp, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(rel)),
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// Put uploads an object.
func (s *Store) Put(ctx context.Context, rel string, data []byte) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(rel)),
		Body:   bytes.NewReader(data),
	})
	return err
}

// Delete removes an object. Deleting a missing key is a no-op on S3, so this is
// safe to call when the object may already be gone.
func (s *Store) Delete(ctx context.Context, rel string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(rel)),
	})
	return err
}

// Exists reports whether an object is present.
func (s *Store) Exists(ctx context.Context, rel string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(rel)),
	})
	if err == nil {
		return true, nil
	}
	var notFound *types.NotFound
	if errors.As(err, &notFound) {
		return false, nil
	}
	return false, err
}
