package app

import (
	"errors"
	"io/fs"
	"syscall"
	"testing"
)

func TestTolerateDirSyncError(t *testing.T) {
	pathErr := &fs.PathError{Op: "sync", Path: "data", Err: syscall.EINVAL}
	if err := tolerateDirSyncError("windows", pathErr); err != nil {
		t.Fatalf("windows directory sync error should be tolerated, got %v", err)
	}
	if err := tolerateDirSyncError("linux", pathErr); !errors.Is(err, syscall.EINVAL) {
		t.Fatalf("linux must keep the directory sync error, got %v", err)
	}
	if err := tolerateDirSyncError("darwin", pathErr); err == nil {
		t.Fatal("darwin must keep the directory sync error")
	}
	other := errors.New("not a path error")
	if err := tolerateDirSyncError("windows", other); err != other {
		t.Fatalf("windows must keep unrelated errors, got %v", err)
	}
	if err := tolerateDirSyncError("linux", nil); err != nil {
		t.Fatalf("nil stays nil, got %v", err)
	}
}
