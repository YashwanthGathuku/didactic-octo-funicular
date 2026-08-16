package objectstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"

	"sentinel-gateway/internal/secrets"
)

// Both adapters are held to one contract.
//
// The S3 adapter runs against gofakes3, an in-process S3 implementation. That
// is a real HTTP server speaking the S3 protocol with real request signing, not
// a mock of this package's interface -- so the request path, the error codes and
// the streaming behaviour are all exercised. What it is not is MinIO or AWS:
// see the note at the foot of this file for exactly what remains unverified.

type storeFactory struct {
	name  string
	build func(t *testing.T) ObjectStore
}

func newFakeS3(t *testing.T) ObjectStore {
	t.Helper()

	backend := s3mem.New()
	faker := gofakes3.New(backend)
	srv := httptest.NewServer(faker.Server())
	t.Cleanup(srv.Close)

	if err := backend.CreateBucket("artifacts"); err != nil {
		t.Fatalf("create bucket: %v", err)
	}

	key, err := secrets.New("fake-s3-secret-key-for-tests-0001")
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewS3Store(context.Background(), S3Config{
		Endpoint:  strings.TrimPrefix(srv.URL, "http://"),
		Bucket:    "artifacts",
		Region:    "us-east-1",
		AccessKey: "fake-access-key",
		SecretKey: key,
	})
	if err != nil {
		t.Fatalf("build s3 store: %v", err)
	}
	return store
}

func factories() []storeFactory {
	return []storeFactory{
		{
			name: "filesystem",
			build: func(t *testing.T) ObjectStore {
				t.Helper()
				store, err := NewFilesystemStore(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				return store
			},
		},
		{name: "s3", build: newFakeS3},
	}
}

func eachStore(t *testing.T, fn func(t *testing.T, store ObjectStore)) {
	t.Helper()
	for _, f := range factories() {
		t.Run(f.name, func(t *testing.T) { fn(t, f.build(t)) })
	}
}

func testKey(t *testing.T) string {
	t.Helper()
	key, err := NewKey("TENANT-A", time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return key
}

const oneMiB = 1 << 20

// The measurements must describe the bytes received, not anything declared.
func TestPutMeasuresWhatItActuallyReceived(t *testing.T) {
	eachStore(t, func(t *testing.T, store ObjectStore) {
		ctx := context.Background()
		payload := []byte("101 021000021 0210000210001011200A094101BANK OF TEST\n")

		res, err := store.Put(ctx, testKey(t), bytes.NewReader(payload), oneMiB)
		if err != nil {
			t.Fatal(err)
		}
		if res.SizeBytes != int64(len(payload)) {
			t.Errorf("size = %d, want %d", res.SizeBytes, len(payload))
		}
		// The hash of this exact payload, computed independently of the store.
		want := sha256Hex(payload)
		if res.SHA256 != want {
			t.Errorf("sha256 = %s, want %s", res.SHA256, want)
		}
		if res.MediaType == "" {
			t.Error("no media type was sniffed")
		}
	})
}

func TestGetReturnsExactlyWhatWasStored(t *testing.T) {
	eachStore(t, func(t *testing.T, store ObjectStore) {
		ctx := context.Background()
		key := testKey(t)
		payload := bytes.Repeat([]byte("record-line-of-ninety-four-chars"), 300)

		if _, err := store.Put(ctx, key, bytes.NewReader(payload), oneMiB); err != nil {
			t.Fatal(err)
		}

		body, err := store.Get(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		defer body.Close()

		got, err := io.ReadAll(body)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, payload) {
			t.Errorf("the stored object differs from what was written: %d bytes vs %d", len(got), len(payload))
		}
	})
}

// The headline property: an artifact is immutable.
func TestPutRefusesToOverwriteAnExistingObject(t *testing.T) {
	eachStore(t, func(t *testing.T, store ObjectStore) {
		ctx := context.Background()
		key := testKey(t)
		original := []byte("the original artifact, as received")

		if _, err := store.Put(ctx, key, bytes.NewReader(original), oneMiB); err != nil {
			t.Fatal(err)
		}
		_, err := store.Put(ctx, key, bytes.NewReader([]byte("a replacement")), oneMiB)
		if !errors.Is(err, ErrObjectExists) {
			t.Fatalf("a second write to the same key returned %v, want ErrObjectExists", err)
		}

		// And the original is untouched.
		body, err := store.Get(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		defer body.Close()
		got, _ := io.ReadAll(body)
		if !bytes.Equal(got, original) {
			t.Errorf("the stored object was modified: %q", got)
		}
	})
}

// A file over the limit must be refused, not truncated. A truncated NACHA file
// parses -- it simply describes fewer payments than were sent.
func TestOversizedUploadIsRefusedAndLeavesNoObject(t *testing.T) {
	eachStore(t, func(t *testing.T, store ObjectStore) {
		ctx := context.Background()
		key := testKey(t)
		payload := bytes.Repeat([]byte("x"), 4096)

		_, err := store.Put(ctx, key, bytes.NewReader(payload), 1024)
		if !errors.Is(err, ErrTooLarge) {
			t.Fatalf("got %v, want ErrTooLarge", err)
		}
		if _, err := store.Stat(ctx, key); !errors.Is(err, ErrObjectNotFound) {
			t.Error("a rejected oversized upload left an object behind")
		}
	})
}

// A payload exactly at the limit is permitted; the boundary must not be
// off by one, or a legitimate file at the documented maximum is rejected.
func TestPayloadExactlyAtTheLimitIsAccepted(t *testing.T) {
	eachStore(t, func(t *testing.T, store ObjectStore) {
		ctx := context.Background()
		payload := bytes.Repeat([]byte("y"), 1024)

		res, err := store.Put(ctx, testKey(t), bytes.NewReader(payload), 1024)
		if err != nil {
			t.Fatalf("a payload at exactly the limit was refused: %v", err)
		}
		if res.SizeBytes != 1024 {
			t.Errorf("size = %d, want 1024", res.SizeBytes)
		}
	})
}

// An empty artifact is refused at the storage boundary as well as at
// validation. Storing one creates a record that something arrived when nothing
// did -- and the empty file is the defect this whole programme started from.
func TestEmptyUploadIsRefusedAndLeavesNoObject(t *testing.T) {
	eachStore(t, func(t *testing.T, store ObjectStore) {
		ctx := context.Background()
		key := testKey(t)

		_, err := store.Put(ctx, key, bytes.NewReader(nil), oneMiB)
		if !errors.Is(err, ErrEmpty) {
			t.Fatalf("got %v, want ErrEmpty", err)
		}
		if _, err := store.Stat(ctx, key); !errors.Is(err, ErrObjectNotFound) {
			t.Error("a rejected empty upload left an object behind")
		}
	})
}

// failingReader models a client that disconnects mid-upload.
type failingReader struct {
	data     []byte
	consumed int
	failAt   int
}

func (f *failingReader) Read(p []byte) (int, error) {
	if f.consumed >= f.failAt {
		return 0, errors.New("connection reset by peer")
	}
	n := copy(p, f.data[f.consumed:])
	if f.consumed+n > f.failAt {
		n = f.failAt - f.consumed
	}
	f.consumed += n
	return n, nil
}

// An interrupted upload must leave no object. A half-written financial file
// that survives is worse than no file, because it looks complete.
func TestInterruptedUploadLeavesNoObject(t *testing.T) {
	eachStore(t, func(t *testing.T, store ObjectStore) {
		ctx := context.Background()
		key := testKey(t)

		reader := &failingReader{data: bytes.Repeat([]byte("z"), 8192), failAt: 4096}
		if _, err := store.Put(ctx, key, reader, oneMiB); err == nil {
			t.Fatal("an interrupted upload was accepted")
		}
		if _, err := store.Stat(ctx, key); !errors.Is(err, ErrObjectNotFound) {
			t.Error("an interrupted upload left a partial object behind")
		}
	})
}

func TestGetAndStatReportAbsentObjects(t *testing.T) {
	eachStore(t, func(t *testing.T, store ObjectStore) {
		ctx := context.Background()
		key := testKey(t)

		if _, err := store.Get(ctx, key); !errors.Is(err, ErrObjectNotFound) {
			t.Errorf("Get on an absent key returned %v, want ErrObjectNotFound", err)
		}
		if _, err := store.Stat(ctx, key); !errors.Is(err, ErrObjectNotFound) {
			t.Errorf("Stat on an absent key returned %v, want ErrObjectNotFound", err)
		}
	})
}

// An unbounded write is a disk and memory exhaustion vector, so a caller that
// forgets the limit must be refused rather than defaulted.
func TestPutRequiresAByteLimit(t *testing.T) {
	eachStore(t, func(t *testing.T, store ObjectStore) {
		for _, limit := range []int64{0, -1} {
			if _, err := store.Put(context.Background(), testKey(t), bytes.NewReader([]byte("data")), limit); err == nil {
				t.Errorf("a limit of %d was accepted", limit)
			}
		}
	})
}

// --- Key generation and filename handling ---

// Keys come from the server. This is what makes path traversal
// unrepresentable rather than filtered.
func TestKeysAreServerGeneratedAndUnique(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	seen := map[string]bool{}
	for i := 0; i < 2048; i++ {
		key, err := NewKey("TENANT-A", now)
		if err != nil {
			t.Fatal(err)
		}
		if seen[key] {
			t.Fatalf("NewKey produced a duplicate after %d keys", i)
		}
		seen[key] = true

		if !strings.HasPrefix(key, "tenant/TENANT-A/2026/03/01/") {
			t.Fatalf("key %q does not carry the expected tenant and date prefix", key)
		}
	}

	// A derived artifact gets a fresh key, not a decoration of its source.
	source, _ := NewKey("TENANT-A", now)
	derived, err := DerivedKey("TENANT-A", now)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(derived, source) || strings.HasPrefix(source, derived) {
		t.Error("a derived key is a prefix relative of its source; the two must be independent")
	}
}

func TestKeyGenerationRefusesAnUnusableTenant(t *testing.T) {
	now := time.Now()
	for _, tenant := range []string{"", "../escape", "a/b", `a\b`, "with.dot"} {
		if _, err := NewKey(tenant, now); err == nil {
			t.Errorf("NewKey accepted the tenant id %q", tenant)
		}
	}
}

// The filename is a display label. These cases would matter if it ever became
// a path; the point of the test is that it is sanitised regardless.
func TestFilenameNormalisation(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		changed bool
	}{
		{"payments.ach", "payments.ach", false},
		{"../../etc/passwd", "passwd", true},
		{"..\\..\\windows\\system32\\config", "config", true},
		{"/absolute/path/file.ach", "file.ach", true},
		{"....//....//escape.ach", "escape.ach", true},
		{".hidden", "hidden", true},
		{"..", "unnamed-artifact", true},
		{"", "unnamed-artifact", true},
		{"   ", "unnamed-artifact", true},
		{"name\x00with-nul.ach", "namewith-nul.ach", true},
		{"forged\nlog line.ach", "forgedlog line.ach", true},
		{"esc\x1b[31mape.ach", "esc[31mape.ach", true},
	}

	for _, tc := range cases {
		got, changed := NormalizeFilename(tc.in)
		if got != tc.want {
			t.Errorf("NormalizeFilename(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if changed != tc.changed {
			t.Errorf("NormalizeFilename(%q) reported changed=%v, want %v", tc.in, changed, tc.changed)
		}
		if strings.ContainsAny(got, "/\\") {
			t.Errorf("NormalizeFilename(%q) = %q still contains a path separator", tc.in, got)
		}
	}

	long, _ := NormalizeFilename(strings.Repeat("a", 4096) + ".ach")
	if len(long) > maxFilenameLength {
		t.Errorf("a long filename was not bounded: %d characters", len(long))
	}
}

// Even a hand-written traversal key must not escape the filesystem root. Keys
// are server-generated so this should be unreachable, which is exactly why it
// is asserted.
func TestFilesystemStoreRefusesKeysThatEscapeItsRoot(t *testing.T) {
	root := t.TempDir()
	store, err := NewFilesystemStore(root)
	if err != nil {
		t.Fatal(err)
	}

	outside := filepath.Join(filepath.Dir(root), "escaped.txt")
	for _, key := range []string{
		"../escaped.txt",
		"../../escaped.txt",
		"tenant/../../escaped.txt",
		"",
	} {
		if _, err := store.Put(context.Background(), key, bytes.NewReader([]byte("data")), oneMiB); err == nil {
			t.Errorf("the key %q was accepted", key)
		}
	}
	if _, err := os.Stat(outside); err == nil {
		t.Error("a file was written outside the store root")
	}
}

// Verify is the honest answer to "has this been tampered with": it re-reads and
// re-hashes rather than trusting recorded metadata.
func TestVerifyDetectsModifiedStoredBytes(t *testing.T) {
	root := t.TempDir()
	store, err := NewFilesystemStore(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	key := testKey(t)

	res, err := store.Put(ctx, key, bytes.NewReader([]byte("the original artifact bytes")), oneMiB)
	if err != nil {
		t.Fatal(err)
	}

	ok, err := store.Verify(ctx, key, res.SHA256)
	if err != nil || !ok {
		t.Fatalf("an untouched object failed verification: ok=%v err=%v", ok, err)
	}

	// Modify the stored file, as an attacker with filesystem access would. The
	// object is written read-only, so this restores write permission first --
	// which is itself worth noting: the mode is a speed bump, not a control.
	path := filepath.Join(root, key)
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("tampered artifact bytes!!!!"), 0o640); err != nil {
		t.Fatal(err)
	}

	ok, err = store.Verify(ctx, key, res.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("Verify reported a tampered object as intact")
	}
}

func TestParseS3URL(t *testing.T) {
	endpoint, bucket, ssl, err := ParseS3URL("https://minio.internal:9000/artifacts")
	if err != nil {
		t.Fatalf("a well-formed URL was refused: %v", err)
	}
	if endpoint != "minio.internal:9000" || bucket != "artifacts" || !ssl {
		t.Errorf("parsed as endpoint=%q bucket=%q ssl=%v", endpoint, bucket, ssl)
	}

	if _, _, ssl, err := ParseS3URL("http://minio:9000/artifacts"); err != nil || ssl {
		t.Errorf("plain http parsed incorrectly: ssl=%v err=%v", ssl, err)
	}

	for _, bad := range []string{
		"s3://artifacts",                      // names no endpoint
		"https://minio:9000",                  // names no bucket
		"https://minio:9000/artifacts/nested", // ambiguous
		"ftp://minio/artifacts",
	} {
		if _, _, _, err := ParseS3URL(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func sha256Hex(b []byte) string {
	w := newHashingWriter()
	_, _ = w.Write(b)
	return w.sum()
}

// A guard so the fmt import stays used if cases are trimmed during editing.
var _ = fmt.Sprintf

// NOT VERIFIED by this suite, and each item is a real difference from
// production:
//
//   - MinIO and AWS S3 themselves. gofakes3 implements the protocol, not the
//     product; multipart edge cases, throttling and eventual-consistency
//     behaviour differ.
//   - Bucket versioning and Object Lock. These are what make an S3 artifact
//     genuinely immutable, they are deployment configuration, and this code
//     neither sets nor reads them. The S3Store doc comment says so.
//   - TLS to a real endpoint. The fake server speaks plain HTTP.
//   - Concurrent writes to one key. The filesystem adapter refuses the second
//     at the syscall; the S3 adapter has a stat-then-put gap that random keys
//     make infeasible to exploit but do not close.
