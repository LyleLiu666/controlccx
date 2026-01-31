package runworkspace

import (
	"io/fs"
	"os"

	"golang.org/x/sys/unix"
)

func tryCloneFile(src, dst string, mode fs.FileMode) bool {
	in, err := os.Open(src)
	if err != nil {
		return false
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return false
	}
	defer func() { _ = out.Close() }()

	if err := unix.IoctlFileClone(int(out.Fd()), int(in.Fd())); err != nil {
		return false
	}
	return true
}
