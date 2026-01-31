//go:build !darwin && !linux

package runworkspace

import "io/fs"

func tryCloneFile(_, _ string, _ fs.FileMode) bool {
	return false
}
