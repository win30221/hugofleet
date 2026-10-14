//go:build linux

package repository

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
)

// The M2 Hub is a single Linux/WSL process, not an HA control plane.
// flock is released by the kernel even on SIGKILL, permitting crash recovery.
func lockDatabase(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("database is already owned by another Hub: %w", err)
	}
	return f, nil
}
