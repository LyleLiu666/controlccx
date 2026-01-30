package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

var fingerprintIgnoreNames = map[string]bool{
	".git":       true,
	".DS_Store":  true,
	"Thumbs.db":  true,
	".gitignore": true,
}

func dirFingerprint(root string) (string, error) {
	root = filepath.Clean(root)
	if root == "" {
		return "", fmt.Errorf("skills: fingerprint: empty root")
	}

	hasher := sha256.New()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if fingerprintIgnoreNames[d.Name()] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		_, _ = hasher.Write([]byte(rel))
		_, _ = hasher.Write([]byte{'\n'})

		typ := d.Type()
		if typ.IsRegular() {
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			_, _ = hasher.Write(b)
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
