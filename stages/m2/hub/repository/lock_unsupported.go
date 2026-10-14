//go:build !linux

package repository

import (
	"fmt"
	"os"
	"runtime"
)

// Keep non-Linux builds usable by editors without bypassing the Hub's process lock.
func lockDatabase(path string) (*os.File, error) {
	return nil, fmt.Errorf("Hub database locking requires Linux/WSL2; unsupported GOOS: %s", runtime.GOOS)
}
