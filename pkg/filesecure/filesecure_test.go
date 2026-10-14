package filesecure

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolveRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	if _, err := Resolve(root, "../secret.txt"); err != ErrPathEscape {
		t.Fatalf("got %v", err)
	}
	if _, err := Resolve(root, `..\windows\system32`); err != ErrPathEscape {
		t.Fatalf("got %v", err)
	}
	got, err := Resolve(root, "a/b.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, filepath.Clean(root)) {
		t.Fatalf("got %s", got)
	}
}

func TestCheckPDFAndRejectExe(t *testing.T) {
	if _, err := Check("a.pdf", []byte("%PDF-1.4")); err != nil {
		t.Fatal(err)
	}
	if _, err := Check("a.exe", []byte("MZ")); err != ErrBadType {
		t.Fatalf("got %v", err)
	}
	if _, err := Check("a.pdf", []byte("not a pdf")); err != ErrBadType {
		t.Fatalf("got %v", err)
	}
}

func TestCheckAllowedUsesNamedMagic(t *testing.T) {
	if _, err := CheckAllowed("a.xlsx", []byte("PK\x03\x04rest"), "zip"); err != nil {
		t.Fatal(err)
	}
	if _, err := CheckAllowed("a.xlsx", []byte("notzip"), "zip"); err != ErrBadType {
		t.Fatalf("got %v", err)
	}
}

func TestRegistryOwnerAndExpiry(t *testing.T) {
	reg := NewRegistry(time.Minute)
	reg.Put(Record{ID: "f1", OwnerID: "u1", AbsPath: filepath.Join(os.TempDir(), "f1.pdf")})
	if _, err := reg.Get("u2", "f1", false); err != ErrFileOwner {
		t.Fatalf("got %v", err)
	}
	if _, err := reg.Get("u2", "f1", true); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Get("u1", "f1", false)
	if err != nil || got.ID != "f1" {
		t.Fatalf("%+v %v", got, err)
	}
}
