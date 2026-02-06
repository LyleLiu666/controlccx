//go:build windows

package daemon

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func tryLockFileNonBlocking(f *os.File) error {
	if f == nil {
		return errors.New("daemon: lock file is nil")
	}
	var ol windows.Overlapped
	err := windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1,
		0,
		&ol,
	)
	if err != nil {
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return ErrAlreadyRunning
		}
		return err
	}
	return nil
}
