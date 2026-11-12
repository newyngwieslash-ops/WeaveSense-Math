package edumath

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"yingce/backend/internal/storage"
)

// BlobStore is the private object store used for education assets (DEV-059).
// A nil store means bytes stay in the database (the current default).
type BlobStore interface {
	Put(ctx context.Context, key, mime string, data []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
}

// MemoryBlobStore is an in-process stand-in for tests and local development
// without MinIO. It is not durable across processes.
type MemoryBlobStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func NewMemoryBlobStore() *MemoryBlobStore {
	return &MemoryBlobStore{data: map[string][]byte{}}
}

func (s *MemoryBlobStore) Put(_ context.Context, key, _ string, data []byte) error {
	if key == "" {
		return fmt.Errorf("object key required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]byte, len(data))
	copy(cp, data)
	s.data[key] = cp
	return nil
}

func (s *MemoryBlobStore) Get(_ context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.data[key]
	if !ok {
		return nil, fmt.Errorf("object not found")
	}
	cp := make([]byte, len(b))
	copy(cp, b)
	return cp, nil
}

func (s *MemoryBlobStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}

func (s *MemoryBlobStore) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.data)
}

// S3BlobStore talks to an S3-compatible endpoint (MinIO, AWS S3, and others)
// through the platform storage package. Path-style addressing is the default
// so a local MinIO stand-in works without virtual-host DNS.
type S3BlobStore struct {
	Setting storage.Settings
}

func (s *S3BlobStore) Put(_ context.Context, key, mime string, data []byte) error {
	_, err := storage.PutS3Object(s.Setting, key, mime, int64(len(data)), bytes.NewReader(data))
	return err
}

func (s *S3BlobStore) Get(_ context.Context, key string) ([]byte, error) {
	stream, err := storage.GetS3ObjectRange(s.Setting, key, "")
	if err != nil {
		return nil, err
	}
	defer stream.Body.Close()
	return io.ReadAll(stream.Body)
}

func (s *S3BlobStore) Delete(_ context.Context, key string) error {
	return storage.DeleteS3Object(s.Setting, key)
}

// BlobStoreFromEnv returns an S3-compatible store when EDU_OBJECT_STORE_* is
// fully set, or nil when education assets should stay in the database.
func BlobStoreFromEnv() (BlobStore, error) {
	endpoint := strings.TrimSpace(os.Getenv("EDU_OBJECT_STORE_ENDPOINT"))
	bucket := strings.TrimSpace(os.Getenv("EDU_OBJECT_STORE_BUCKET"))
	access := strings.TrimSpace(os.Getenv("EDU_OBJECT_STORE_ACCESS_KEY"))
	secret := strings.TrimSpace(os.Getenv("EDU_OBJECT_STORE_SECRET_KEY"))
	if endpoint == "" && bucket == "" && access == "" && secret == "" {
		return nil, nil
	}
	if endpoint == "" || bucket == "" || access == "" || secret == "" {
		return nil, fmt.Errorf("EDU_OBJECT_STORE_ENDPOINT, EDU_OBJECT_STORE_BUCKET, EDU_OBJECT_STORE_ACCESS_KEY and EDU_OBJECT_STORE_SECRET_KEY must be set together")
	}
	region := strings.TrimSpace(os.Getenv("EDU_OBJECT_STORE_REGION"))
	if region == "" {
		region = "us-east-1"
	}
	pathStyle := true
	if v := strings.TrimSpace(os.Getenv("EDU_OBJECT_STORE_PATH_STYLE")); v == "0" || strings.EqualFold(v, "false") {
		pathStyle = false
	}
	prefix := strings.Trim(strings.TrimSpace(os.Getenv("EDU_OBJECT_STORE_PREFIX")), "/")
	if prefix == "" {
		prefix = "edu"
	}
	return &S3BlobStore{Setting: storage.NormalizeSettings(storage.Settings{
		Enabled:         true,
		Provider:        "s3",
		Endpoint:        endpoint,
		Region:          region,
		Bucket:          bucket,
		AccessKeyID:     access,
		AccessKeySecret: secret,
		PathPrefix:      prefix,
		PathStyle:       pathStyle,
	})}, nil
}

func objectKey(prefix, tenantID, imageID string) string {
	prefix = strings.Trim(prefix, "/")
	if prefix == "" {
		prefix = "edu"
	}
	return prefix + "/problem-images/" + tenantID + "/" + imageID
}

func storePrefix(s BlobStore) string {
	if ss, ok := s.(*S3BlobStore); ok && ss.Setting.PathPrefix != "" {
		return ss.Setting.PathPrefix
	}
	return "edu"
}
