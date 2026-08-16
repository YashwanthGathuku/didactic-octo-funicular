package objectstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"sentinel-gateway/internal/secrets"
)

// S3Store keeps objects in an S3-compatible bucket.
//
// The immutability guarantee here is weaker than the filesystem adapter's, and
// the difference must not be glossed over:
//
//   - The filesystem adapter refuses an existing key at the syscall, so a
//     concurrent second write cannot win.
//   - S3 has no portable atomic create-if-absent. This adapter stats before
//     putting, which is a time-of-check-to-time-of-use gap. What actually makes
//     an overwrite infeasible is that keys carry 128 bits from crypto/rand, so
//     a second writer would have to guess one.
//
// Durable immutability in S3 is a bucket property, not an application one. A
// production bucket must have versioning and Object Lock in compliance mode
// enabled. **This code does not configure or verify that**, and cannot: a
// client with only PutObject/GetObject rights cannot report on a retention
// policy it is not permitted to read. It is deployment configuration, and it is
// recorded as such in docs/engineering/ARTIFACT_STORAGE.md.
type S3Store struct {
	client *minio.Client
	bucket string
}

// S3Config describes the target bucket.
type S3Config struct {
	// Endpoint is host[:port], without a scheme.
	Endpoint string
	Bucket   string
	Region   string
	UseSSL   bool

	AccessKey string
	// SecretKey is a secrets.Value so it redacts itself if a config carrying it
	// is ever logged. minio-go needs the plaintext, and Expose is called once,
	// at construction.
	SecretKey secrets.Value
}

// NewS3Store builds the adapter and confirms the bucket is reachable.
//
// The reachability check is deliberate: a store that constructs successfully
// and fails on first upload turns a configuration error into a rejected
// financial file. Failing at startup makes it an operator's problem before it
// is a customer's.
func NewS3Store(ctx context.Context, cfg S3Config) (*S3Store, error) {
	if cfg.Endpoint == "" || cfg.Bucket == "" {
		return nil, errors.New("an S3 object store requires an endpoint and a bucket")
	}
	if cfg.AccessKey == "" || cfg.SecretKey.IsZero() {
		return nil, errors.New("an S3 object store requires credentials")
	}

	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey.Expose(), ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("build s3 client: %w", err)
	}

	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		// The error can carry the endpoint and, in some failure modes, request
		// signing details. Redaction here keeps a startup failure diagnosable
		// without putting credential material in the log.
		return nil, secrets.RedactError(fmt.Errorf("reach bucket %q: %w", cfg.Bucket, err))
	}
	if !exists {
		return nil, fmt.Errorf("bucket %q does not exist; this process does not create it, because creating a bucket without versioning or object lock would silently produce mutable artifact storage", cfg.Bucket)
	}

	return &S3Store{client: client, bucket: cfg.Bucket}, nil
}

// ParseS3URL splits an s3://bucket/... or http(s)://host/bucket style
// OBJECT_STORE_URL into its parts.
//
// It accepts the two forms operators actually write and refuses anything else,
// rather than guessing. A misparsed store URL sends artifacts somewhere
// unintended, which is worse than refusing to start.
func ParseS3URL(raw string) (endpoint, bucket string, useSSL bool, err error) {
	u, perr := url.Parse(raw)
	if perr != nil {
		return "", "", false, fmt.Errorf("OBJECT_STORE_URL %q is not a URL: %w", raw, perr)
	}
	switch strings.ToLower(u.Scheme) {
	case "s3":
		// s3://bucket/optional-prefix -- the endpoint comes from the region and
		// is not expressible here, so this form is refused with an explanation
		// rather than defaulted to AWS.
		return "", "", false, errors.New("OBJECT_STORE_URL uses the s3:// form, which does not name an endpoint; use https://endpoint/bucket")
	case "http", "https":
		trimmed := strings.Trim(u.Path, "/")
		if trimmed == "" || strings.Contains(trimmed, "/") {
			return "", "", false, fmt.Errorf("OBJECT_STORE_URL %q must be scheme://endpoint/bucket with exactly one path segment", raw)
		}
		return u.Host, trimmed, strings.EqualFold(u.Scheme, "https"), nil
	default:
		return "", "", false, fmt.Errorf("OBJECT_STORE_URL scheme %q is not supported", u.Scheme)
	}
}

// Put stages r to a temporary file, measures it, and uploads only once the
// whole payload has arrived intact.
//
// Two designs were possible and the choice matters, so it is recorded here.
//
// Streaming the request body straight through to PutObject with an unknown
// length is the obvious approach. It has two problems. The upload begins before
// the payload is known to be complete, so a client that disconnects halfway
// leaves a partial object in immutable storage -- and a truncated NACHA file
// parses, it simply describes fewer payments. And an unknown length forces
// minio-go into streaming SigV4, whose chunk framing not every S3-compatible
// implementation decodes identically.
//
// Staging to a temporary file avoids both. The copy uses a 32 KiB buffer, so
// memory stays flat regardless of artifact size -- the requirement is bounded
// memory, not zero disk. The cost is honest and worth stating: an upload needs
// transient local disk equal to the artifact, and the object reaches the bucket
// after the client finishes rather than during.
func (s *S3Store) Put(ctx context.Context, key string, r io.Reader, limit int64) (PutResult, error) {
	if limit <= 0 {
		return PutResult{}, errors.New("a byte limit is required; an unbounded write is a memory and storage exhaustion vector")
	}

	if _, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{}); err == nil {
		return PutResult{}, ErrObjectExists
	} else if !isNotFound(err) {
		return PutResult{}, secrets.RedactError(err)
	}

	staged, err := os.CreateTemp("", "sentinel-artifact-*")
	if err != nil {
		return PutResult{}, err
	}
	stagedName := staged.Name()
	defer func() {
		staged.Close()
		_ = os.Remove(stagedName)
	}()

	measured := newHashingWriter()
	bounded := &boundedReader{r: r, remaining: limit}

	if _, err := io.Copy(io.MultiWriter(staged, measured), bounded); err != nil {
		if bounded.exceeded || errors.Is(err, ErrTooLarge) {
			return PutResult{}, ErrTooLarge
		}
		// A reader error means the client disconnected. Nothing has been sent,
		// so there is nothing to clean up in the bucket.
		return PutResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return PutResult{}, err
	}
	if measured.written == 0 {
		return PutResult{}, ErrEmpty
	}
	if _, err := staged.Seek(0, io.SeekStart); err != nil {
		return PutResult{}, err
	}

	// The length is now known exactly, from the bytes actually received.
	_, err = s.client.PutObject(ctx, s.bucket, key, staged, measured.written, minio.PutObjectOptions{
		ContentType: measured.mediaType(),
		PartSize:    16 << 20,
	})
	if err != nil {
		// A failed multipart upload can leave parts behind.
		_ = s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
		return PutResult{}, secrets.RedactError(err)
	}

	return PutResult{
		Key:       key,
		SizeBytes: measured.written,
		SHA256:    measured.sum(),
		MediaType: measured.mediaType(),
	}, nil
}

// Get opens a stored object for streaming.
func (s *S3Store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, secrets.RedactError(err)
	}
	// minio-go defers the request, so a missing key surfaces on first read
	// rather than here. Stat first, so the caller gets ErrObjectNotFound at the
	// point it asked rather than mid-stream.
	if _, err := obj.Stat(); err != nil {
		obj.Close()
		if isNotFound(err) {
			return nil, ErrObjectNotFound
		}
		return nil, secrets.RedactError(err)
	}
	return obj, nil
}

// Stat returns metadata without transferring the object.
func (s *S3Store) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	info, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		if isNotFound(err) {
			return ObjectInfo{}, ErrObjectNotFound
		}
		return ObjectInfo{}, secrets.RedactError(err)
	}
	return ObjectInfo{
		Key:       key,
		SizeBytes: info.Size,
		StoredAt:  info.LastModified.UTC(),
	}, nil
}

func isNotFound(err error) bool {
	resp := minio.ToErrorResponse(err)
	return resp.Code == "NoSuchKey" || resp.StatusCode == 404
}

var _ ObjectStore = (*S3Store)(nil)
