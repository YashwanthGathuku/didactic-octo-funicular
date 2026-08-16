package objectstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// FilesystemStore keeps objects as files under a root directory.
//
// It is the local-development adapter and the one every conformance test runs
// against first, because its failure modes are the easiest to reason about. It
// is a legitimate production choice only on a filesystem that provides the
// durability the deployment needs; the contract's stated architecture is
// S3-compatible storage, and S3Store is that adapter.
//
// Immutability rests on two mechanisms, not one:
//
//   - The final file is created with O_EXCL, so a second write to the same key
//     fails at the syscall rather than after a check that something could race.
//   - The bytes are written to a temporary file first and moved into place only
//     after the stream completes. An interrupted upload therefore leaves a
//     temporary file that is removed, never a truncated object under a real
//     key.
type FilesystemStore struct {
	root string
}

// NewFilesystemStore creates the root if it does not exist.
func NewFilesystemStore(root string) (*FilesystemStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("a filesystem object store requires a root directory")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return nil, fmt.Errorf("create object store root: %w", err)
	}
	return &FilesystemStore{root: abs}, nil
}

// resolve turns a key into a path and refuses anything that would escape the
// root.
//
// Keys are server-generated, so this should be unreachable. It is here because
// "should be unreachable" is what every path traversal was before it happened,
// and because a future caller may pass a key read back from the database.
func (f *FilesystemStore) resolve(key string) (string, error) {
	if key == "" {
		return "", errors.New("empty object key")
	}
	full := filepath.Join(f.root, filepath.FromSlash(key))
	clean := filepath.Clean(full)
	if clean != f.root && !strings.HasPrefix(clean, f.root+string(os.PathSeparator)) {
		return "", fmt.Errorf("object key %q resolves outside the store root", key)
	}
	return clean, nil
}

// Put streams r into key, measuring as it goes.
func (f *FilesystemStore) Put(ctx context.Context, key string, r io.Reader, limit int64) (PutResult, error) {
	dest, err := f.resolve(key)
	if err != nil {
		return PutResult{}, err
	}
	if limit <= 0 {
		return PutResult{}, errors.New("a byte limit is required; an unbounded write is a memory and disk exhaustion vector")
	}

	// Refuse an existing key before doing any work.
	if _, err := os.Stat(dest); err == nil {
		return PutResult{}, ErrObjectExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return PutResult{}, err
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return PutResult{}, err
	}

	tmp, err := os.CreateTemp(filepath.Dir(dest), ".incoming-*")
	if err != nil {
		return PutResult{}, err
	}
	tmpName := tmp.Name()
	// Removed on every path that does not reach the rename. A rename makes this
	// a no-op because the name no longer exists.
	defer func() {
		tmp.Close()
		_ = os.Remove(tmpName)
	}()

	measured := newHashingWriter()
	bounded := &boundedReader{r: r, remaining: limit}

	// io.Copy uses a 32 KiB buffer; the payload never accumulates.
	if _, err := io.Copy(io.MultiWriter(tmp, measured), bounded); err != nil {
		return PutResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return PutResult{}, err
	}
	if measured.written == 0 {
		return PutResult{}, ErrEmpty
	}

	// fsync before the rename: without it the rename can be durable while the
	// contents are not, which produces an object that exists and is empty.
	if err := tmp.Sync(); err != nil {
		return PutResult{}, err
	}
	if err := tmp.Close(); err != nil {
		return PutResult{}, err
	}

	// O_EXCL on the final name. os.Rename would overwrite, so the link-then-
	// unlink pair is what makes the write refuse an existing object even
	// against a concurrent writer.
	if err := os.Link(tmpName, dest); err != nil {
		if errors.Is(err, os.ErrExist) {
			return PutResult{}, ErrObjectExists
		}
		return PutResult{}, err
	}
	if err := os.Chmod(dest, 0o440); err != nil {
		return PutResult{}, err
	}

	return PutResult{
		Key:       key,
		SizeBytes: measured.written,
		SHA256:    measured.sum(),
		MediaType: measured.mediaType(),
	}, nil
}

// Get opens the object for streaming. The caller closes it.
func (f *FilesystemStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	dest, err := f.resolve(key)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(dest)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrObjectNotFound
	}
	return file, err
}

// Stat returns metadata without reading the object.
//
// The SHA-256 is deliberately absent: recomputing it would require reading the
// whole object, and the hash recorded at ingest is the authoritative one. A
// caller that needs to confirm the stored bytes still match must read them and
// say so.
func (f *FilesystemStore) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	dest, err := f.resolve(key)
	if err != nil {
		return ObjectInfo{}, err
	}
	info, err := os.Stat(dest)
	if errors.Is(err, os.ErrNotExist) {
		return ObjectInfo{}, ErrObjectNotFound
	}
	if err != nil {
		return ObjectInfo{}, err
	}
	return ObjectInfo{
		Key:       key,
		SizeBytes: info.Size(),
		StoredAt:  info.ModTime().UTC(),
	}, nil
}

// Verify re-reads a stored object and reports whether its bytes still hash to
// the value recorded at ingest.
//
// This is the only honest way to answer "has this artifact been tampered with",
// and it costs a full read, which is why it is a separate method rather than
// something Stat pretends to do.
func (f *FilesystemStore) Verify(ctx context.Context, key, expectedSHA256 string) (bool, error) {
	body, err := f.Get(ctx, key)
	if err != nil {
		return false, err
	}
	defer body.Close()

	measured := newHashingWriter()
	if _, err := io.Copy(measured, body); err != nil {
		return false, err
	}
	return measured.sum() == expectedSHA256, nil
}

// Root reports the directory in use, for startup logging.
func (f *FilesystemStore) Root() string { return f.root }

var _ ObjectStore = (*FilesystemStore)(nil)
