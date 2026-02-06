//go:build !windows

package daemon

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func tryLockFileNonBlocking(f *os.File) error {
	if f == nil {
		return errors.New("daemon: lock file is nil")
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) {
			return ErrAlreadyRunning
		}
		return err
	}
	return nil
}
