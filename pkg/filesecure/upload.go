package filesecure

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const MaxBytes = 10 << 20 // 10 MiB

var (
	ErrTooLarge  = errors.New("file is larger than 10MB")
	ErrBadType   = errors.New("file type is not allowed")
	ErrEmptyFile = errors.New("file is empty")
)

var allowedExt = map[string]struct{}{
	".pdf":  {},
	".png":  {},
	".jpg":  {},
	".jpeg": {},
	".txt":  {},
	".csv":  {},
}

func ExtOf(filename string) string {
	return strings.ToLower(filepath.Ext(filename))
}

func Check(filename string, data []byte) (ext string, err error) {
	if len(data) == 0 {
		return "", ErrEmptyFile
	}
	if len(data) > MaxBytes {
		return "", ErrTooLarge
	}
	ext = ExtOf(filename)
	if _, ok := allowedExt[ext]; !ok {
		return "", ErrBadType
	}
	if !matchMagic(ext, data) {
		return "", ErrBadType
	}
	return ext, nil
}

// CheckAllowed 按库里的 magic 规则校验内容和大小。扩展名是否放行由调用方先查白名单。
func CheckAllowed(filename string, data []byte, magic string) (ext string, err error) {
	if len(data) == 0 {
		return "", ErrEmptyFile
	}
	if len(data) > MaxBytes {
		return "", ErrTooLarge
	}
	ext = ExtOf(filename)
	if !matchNamedMagic(magic, ext, data) {
		return "", ErrBadType
	}
	return ext, nil
}

func matchNamedMagic(magic, ext string, data []byte) bool {
	switch strings.ToLower(strings.TrimSpace(magic)) {
	case "pdf", ".pdf":
		return bytes.HasPrefix(data, []byte("%PDF"))
	case "png", ".png":
		return bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a})
	case "jpeg", "jpg", ".jpg", ".jpeg":
		return bytes.HasPrefix(data, []byte{0xff, 0xd8, 0xff})
	case "text", ".txt", ".csv":
		return matchMagic(".txt", data)
	case "zip":
		return bytes.HasPrefix(data, []byte("PK"))
	case "any":
		return len(data) > 0
	default:
		return matchMagic(ext, data)
	}
}

func matchMagic(ext string, data []byte) bool {
	switch ext {
	case ".pdf":
		return bytes.HasPrefix(data, []byte("%PDF"))
	case ".png":
		return bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a})
	case ".jpg", ".jpeg":
		return bytes.HasPrefix(data, []byte{0xff, 0xd8, 0xff})
	case ".txt", ".csv":
		if bytes.IndexByte(data, 0) >= 0 {
			return false
		}
		return utf8.Valid(data) || isMostlyText(data)
	default:
		return false
	}
}

func isMostlyText(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	bad := 0
	n := len(data)
	if n > 4096 {
		n = 4096
	}
	for _, b := range data[:n] {
		if b < 0x09 || (b > 0x0d && b < 0x20) {
			bad++
		}
	}
	return bad*10 < n
}
