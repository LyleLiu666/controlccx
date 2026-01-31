package runworkspace

import (
	"io/fs"
	"os"

	"golang.org/x/sys/unix"
)

func tryCloneFile(src, dst string, mode fs.FileMode) bool {
	if err := unix.Clonefile(src, dst, 0); err != nil {
		return false
	}
	_ = os.Chmod(dst, mode.Perm())
	return true
}
