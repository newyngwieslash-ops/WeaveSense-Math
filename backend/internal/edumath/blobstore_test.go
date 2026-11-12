package edumath

import (
	"context"
	"strings"
	"testing"
)

func TestMemoryBlobStoreRoundTripAndDelete(t *testing.T) {
	s := NewMemoryBlobStore()
	ctx := context.Background()
	if err := s.Put(ctx, "edu/problem-images/tn/img1", "image/png", []byte("png-bytes")); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, "edu/problem-images/tn/img1")
	if err != nil || string(got) != "png-bytes" {
		t.Fatalf("get: %q %v", got, err)
	}
	if err := s.Delete(ctx, "edu/problem-images/tn/img1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, "edu/problem-images/tn/img1"); err == nil {
		t.Fatal("expected missing object")
	}
	if s.Len() != 0 {
		t.Fatalf("len=%d", s.Len())
	}
}

func TestBlobStoreFromEnvRequiresCompleteSettings(t *testing.T) {
	t.Setenv("EDU_OBJECT_STORE_ENDPOINT", "")
	t.Setenv("EDU_OBJECT_STORE_BUCKET", "")
	t.Setenv("EDU_OBJECT_STORE_ACCESS_KEY", "")
	t.Setenv("EDU_OBJECT_STORE_SECRET_KEY", "")
	s, err := BlobStoreFromEnv()
	if err != nil || s != nil {
		t.Fatalf("empty env: %v %v", s, err)
	}
	t.Setenv("EDU_OBJECT_STORE_ENDPOINT", "http://127.0.0.1:9000")
	t.Setenv("EDU_OBJECT_STORE_BUCKET", "edu-assets")
	if _, err := BlobStoreFromEnv(); err == nil || !strings.Contains(err.Error(), "must be set together") {
		t.Fatalf("partial env: %v", err)
	}
	t.Setenv("EDU_OBJECT_STORE_ACCESS_KEY", "minioadmin")
	t.Setenv("EDU_OBJECT_STORE_SECRET_KEY", "minioadmin")
	t.Setenv("EDU_OBJECT_STORE_PREFIX", "edu-test")
	store, err := BlobStoreFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	ss, ok := store.(*S3BlobStore)
	if !ok || ss.Setting.Endpoint != "http://127.0.0.1:9000" || ss.Setting.Bucket != "edu-assets" || !ss.Setting.PathStyle || ss.Setting.PathPrefix != "edu-test" || ss.Setting.Region != "us-east-1" {
		t.Fatalf("%+v", ss)
	}
}

func TestObjectKeyIsolatesTenant(t *testing.T) {
	a := objectKey("edu", "tn_a", "epi_1")
	b := objectKey("edu", "tn_b", "epi_1")
	if a == b || !strings.Contains(a, "tn_a") || !strings.HasPrefix(a, "edu/problem-images/") {
		t.Fatalf("%s %s", a, b)
	}
}
