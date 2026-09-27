package generate

import (
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
)

func replaceAll(files map[string][]byte) error {
	temps := make(map[string]string, len(files))
	for name, source := range files {
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			return errors.Wrapf(err, "create output directory for %s", name)
		}
		temp, err := os.CreateTemp(filepath.Dir(name), ".spec-guardian-")
		if err != nil {
			return errors.Wrapf(err, "stage %s", name)
		}
		if _, err := temp.Write(source); err != nil {
			temp.Close()
			os.Remove(temp.Name())
			return errors.Wrapf(err, "stage %s", name)
		}
		if err := temp.Close(); err != nil {
			os.Remove(temp.Name())
			return errors.Wrapf(err, "stage %s", name)
		}
		temps[name] = temp.Name()
	}
	for name, temp := range temps {
		if err := os.Rename(temp, name); err != nil {
			return errors.Wrapf(err, "replace %s", name)
		}
	}
	return nil
}
