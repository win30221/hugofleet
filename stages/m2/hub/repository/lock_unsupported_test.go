//go:build !linux

package repository

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLockDatabaseUnsupportedPlatform(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hub.db.lock")
	lock, err := lockDatabase(path)
	if lock != nil {
		lock.Close()
		t.Fatal("unsupported platform returned a database lock")
	}
	if err == nil || !strings.Contains(err.Error(), "Linux/WSL2") {
		t.Fatalf("expected Linux/WSL2 requirement, got %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unsupported platform created a lock file: %v", err)
	}
}
