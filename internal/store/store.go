package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jbuitt/nwws-go-client/internal/product"
)

// Result indicates what WriteProduct did with a product.
type Result int

const (
	Written Result = iota
	DuplicateSkipped
)

// WriteProduct saves p under archiveDir/<cccc>/<filename>. If that file
// already exists (a duplicate product), it is left untouched and
// DuplicateSkipped is returned instead of an error. The returned path is
// valid regardless of the result.
func WriteProduct(archiveDir string, p product.Product) (Result, string, error) {
	dir := filepath.Join(archiveDir, p.Dir())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, "", fmt.Errorf("creating directory %q: %w", dir, err)
	}

	path := filepath.Join(dir, p.Filename())
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return DuplicateSkipped, path, nil
		}
		return 0, "", fmt.Errorf("creating file %q: %w", path, err)
	}
	defer f.Close()

	if _, err := f.WriteString(p.Text); err != nil {
		return 0, "", fmt.Errorf("writing file %q: %w", path, err)
	}

	return Written, path, nil
}
