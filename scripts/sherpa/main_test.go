package main

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func libraryArchive(t *testing.T, name string, kind byte) *tar.Reader {
	t.Helper()
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	h := &tar.Header{Name: name, Typeflag: kind, Mode: 0755}
	if kind == tar.TypeReg {
		h.Size = int64(len("library"))
	} else {
		h.Linkname = "../outside.dll"
	}
	if err := w.WriteHeader(h); err != nil {
		t.Fatal(err)
	}
	if kind == tar.TypeReg {
		if _, err := w.Write([]byte("library")); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return tar.NewReader(&b)
}

// Archive paths select a known library but never become its output path.
func TestExtractTarKeepsLibrariesInsideDestination(t *testing.T) {
	for _, name := range []string{"package/lib/library.dll", "../library.dll", "/library.dll", `C:\outside\library.dll`, `..\library.dll`} {
		t.Run(name, func(t *testing.T) {
			parent := t.TempDir()
			dir := filepath.Join(parent, "output")
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			outside := filepath.Join(parent, "library.dll")
			if err := os.WriteFile(outside, []byte("unchanged"), 0600); err != nil {
				t.Fatal(err)
			}
			err := extractTar(libraryArchive(t, name, tar.TypeReg), dir, []string{"library.dll"})
			if bytes.ContainsRune([]byte(name), '\\') {
				if err == nil {
					t.Fatal("non-tar path unexpectedly selected a library")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if b, err := os.ReadFile(filepath.Join(dir, "library.dll")); err != nil || string(b) != "library" {
					t.Fatal("library was not extracted into destination")
				}
			}
			if b, err := os.ReadFile(outside); err != nil || string(b) != "unchanged" {
				t.Fatal("archive changed a file outside destination")
			}
		})
	}
}

// An existing destination must not be truncated or followed during extraction.
func TestExtractTarRefusesExistingFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "library.dll")
	if err := os.WriteFile(file, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := extractTar(libraryArchive(t, "lib/library.dll", tar.TypeReg), dir, []string{"library.dll"}); err == nil {
		t.Fatal("overwrote an existing destination")
	}
	if b, err := os.ReadFile(file); err != nil || string(b) != "unchanged" {
		t.Fatal("existing file changed")
	}
}

// Link entries cannot satisfy the requested library or create a link on disk.
func TestExtractTarRejectsLinks(t *testing.T) {
	for _, kind := range []byte{tar.TypeSymlink, tar.TypeLink} {
		dir := t.TempDir()
		if err := extractTar(libraryArchive(t, "library.dll", kind), dir, []string{"library.dll"}); err == nil {
			t.Fatal("accepted a link as a library")
		}
		if _, err := os.Lstat(filepath.Join(dir, "library.dll")); !os.IsNotExist(err) {
			t.Fatal("link was created")
		}
	}
}
