// Package atomicfile replaces files whole, so that a failure part way leaves the
// old file as it was.
package atomicfile

import (
	"io/fs"
	"os"
	"path/filepath"
)

// Write puts the data in the file, through a temporary file beside it, which ends
// up with the permissions given.
func Write(path string, data []byte, perm fs.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
