// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package objectstore

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"cloud.google.com/go/storage"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"gocloud.dev/blob"
	"gocloud.dev/blob/fileblob"
	"gocloud.dev/blob/gcsblob"
	"gocloud.dev/blob/memblob"
	"gocloud.dev/blob/s3blob"
	"google.golang.org/api/option"
)

// Opener opens buckets by declared URL and keeps process-shared state for
// the schemes that have none of their own: a mem:// bucket opened twice must
// be the same bucket, or a write could never be read back or undone within
// one run (srd059 R2.1).
type Opener struct {
	mu  sync.Mutex
	mem map[string]*blob.Bucket
}

// NewOpener returns an Opener with an empty memory-bucket table.
func NewOpener() *Opener {
	return &Opener{mem: map[string]*blob.Bucket{}}
}

// Open opens the connection's bucket by its URL scheme. The endpoint field
// is the only emulator path: no environment variable is consulted for
// endpoint, credential, or backend selection (srd059 R2.2).
func (o *Opener) Open(ctx context.Context, connection ConnectionConfig) (*blob.Bucket, func(), error) {
	parsed, err := url.Parse(connection.BucketURL)
	if err != nil {
		return nil, nil, fmt.Errorf("bucket_url %q: %w", connection.BucketURL, err)
	}
	switch parsed.Scheme {
	case "mem":
		bucket := o.memBucket(connection.BucketURL)
		return bucket, func() {}, nil
	case "file":
		bucket, err := fileblob.OpenBucket(parsed.Path, &fileblob.Options{CreateDir: true})
		if err != nil {
			return nil, nil, fmt.Errorf("open file bucket %q: %w", connection.BucketURL, err)
		}
		return bucket, closerOf(bucket), nil
	case "gs":
		bucket, err := openGCS(ctx, parsed.Host, connection.Endpoint)
		if err != nil {
			return nil, nil, err
		}
		return bucket, closerOf(bucket), nil
	case "s3":
		bucket, err := openS3(ctx, parsed.Host, connection.Endpoint)
		if err != nil {
			return nil, nil, err
		}
		return bucket, closerOf(bucket), nil
	default:
		return nil, nil, fmt.Errorf(
			"bucket_url %q: unknown scheme %q (supported: gs, s3, file, mem)",
			connection.BucketURL, parsed.Scheme)
	}
}

// memBucket returns the one shared in-memory bucket for a URL, creating it
// on first use.
func (o *Opener) memBucket(bucketURL string) *blob.Bucket {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.mem == nil {
		o.mem = map[string]*blob.Bucket{}
	}
	bucket, ok := o.mem[bucketURL]
	if !ok {
		bucket = memblob.OpenBucket(nil)
		o.mem[bucketURL] = bucket
	}
	return bucket
}

// openGCS builds the storage client explicitly. With an endpoint the client
// speaks to the local emulator unauthenticated; without one the SDK resolves
// the platform's ambient identity. Both paths are declared configuration,
// and the deprecated emulator environment variable plays no part.
func openGCS(ctx context.Context, bucketName, endpoint string) (*blob.Bucket, error) {
	var options []option.ClientOption
	if strings.TrimSpace(endpoint) != "" {
		options = append(options,
			option.WithEndpoint(endpoint),
			option.WithoutAuthentication(),
		)
	}
	client, err := storage.NewClient(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("open gs client: %w", err)
	}
	bucket, err := gcsblob.OpenBucket(ctx, nil, bucketName, &gcsblob.Options{Client: client})
	if err != nil {
		return nil, fmt.Errorf("open gs bucket %q: %w", bucketName, err)
	}
	return bucket, nil
}

// openS3 mirrors openGCS for s3://. The SDK's default resolution supplies
// ambient identity; an explicit endpoint points at a local or alternate
// S3-compatible service.
func openS3(ctx context.Context, bucketName, endpoint string) (*blob.Bucket, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("open s3 config: %w", err)
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if strings.TrimSpace(endpoint) != "" {
			o.BaseEndpoint = &endpoint
			o.UsePathStyle = true
		}
	})
	bucket, err := s3blob.OpenBucketV2(ctx, client, bucketName, nil)
	if err != nil {
		return nil, fmt.Errorf("open s3 bucket %q: %w", bucketName, err)
	}
	return bucket, nil
}

func closerOf(bucket *blob.Bucket) func() {
	return func() { _ = bucket.Close() }
}

// probeScheme validates the declared URL at configuration time without
// opening any client, so an unknown scheme or an unparseable URL fails the
// load with the fault named (srd059 R2.1).
func probeScheme(connection ConnectionConfig) (string, string, error) {
	parsed, err := url.Parse(connection.BucketURL)
	if err != nil {
		return "", "", fmt.Errorf("bucket_url %q: %w", connection.BucketURL, err)
	}
	switch parsed.Scheme {
	case "mem", "file", "gs", "s3":
		return parsed.Scheme, parsed.Host, nil
	default:
		return "", "", fmt.Errorf(
			"bucket_url %q: unknown scheme %q (supported: gs, s3, file, mem)",
			connection.BucketURL, parsed.Scheme)
	}
}
