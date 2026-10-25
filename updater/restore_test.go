package updater

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRestoreDir(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "a.txt"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(src, "b.txt"), []byte("b"), 0o600)
	bk := t.TempDir()
	m, err := backupDir(src, bk)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeManifest(bk, m); err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	os.WriteFile(filepath.Join(dst, "a.txt"), []byte("old"), 0o644)
	os.WriteFile(filepath.Join(dst, "extra.txt"), []byte("extra"), 0o644)
	if err := restoreDir(bk, dst); err != nil {
		t.Fatalf("restore: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dst, "a.txt"))
	if string(got) != "a" {
		t.Fatalf("a.txt %q", got)
	}
	if _, err := os.Stat(filepath.Join(dst, "extra.txt")); !os.IsNotExist(err) {
		t.Fatal("extra not removed")
	}
	os.WriteFile(filepath.Join(bk, "a.txt"), []byte("tampered"), 0o644)
	if err := restoreDir(bk, dst); err == nil {
		t.Fatal("tampered backup restored")
	}
}
