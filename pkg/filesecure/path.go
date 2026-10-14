package filesecure

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrPathEscape = errors.New("file path is outside the upload root")
	ErrEmptyPath  = errors.New("file path is empty")
)

// Resolve returns an absolute path under root. name may be a file id or relative path.
func Resolve(root, name string) (string, error) {
	root = strings.TrimSpace(root)
	name = strings.TrimSpace(name)
	if root == "" || name == "" {
		return "", ErrEmptyPath
	}
	name = strings.ReplaceAll(name, `\`, `/`)
	name = strings.TrimPrefix(name, "/")
	if strings.Contains(name, "..") {
		return "", ErrPathEscape
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	absRoot = filepath.Clean(absRoot)
	candidate := filepath.Clean(filepath.Join(absRoot, filepath.FromSlash(name)))
	sep := string(os.PathSeparator)
	if candidate != absRoot && !strings.HasPrefix(candidate, absRoot+sep) {
		return "", ErrPathEscape
	}
	return candidate, nil
}
