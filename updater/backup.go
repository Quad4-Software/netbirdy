package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// backup copies dir to dst preserving modes and returns a sha256 manifest.
func backupDir(src, dst string) (map[string]string, error) {
	manifest := map[string]string{}
	err := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		to := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(to, info.Mode())
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		h := sha256.New()
		w, err := os.OpenFile(to, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
		if err != nil {
			f.Close()
			return err
		}
		if _, err := io.Copy(io.MultiWriter(w, h), f); err != nil {
			w.Close()
			f.Close()
			return err
		}
		w.Close()
		f.Close()
		manifest[rel] = hex.EncodeToString(h.Sum(nil))
		return nil
	})
	return manifest, err
}

func writeManifest(dir string, m map[string]string) error {
	f, err := os.Create(filepath.Join(dir, ".manifest"))
	if err != nil {
		return err
	}
	defer f.Close()
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(f, "%s  %s\n", m[k], k)
	}
	return nil
}

func verifyManifest(dir string) error {
	m := map[string]string{}
	b, err := os.ReadFile(filepath.Join(dir, ".manifest"))
	if err != nil {
		return err
	}
	for _, ln := range strings.Split(string(b), "\n") {
		f := strings.Fields(ln)
		if len(f) == 2 {
			m[f[1]] = f[0]
		}
	}
	for rel, want := range m {
		f, err := os.Open(filepath.Join(dir, rel))
		if err != nil {
			return err
		}
		h := sha256.New()
		io.Copy(h, f)
		f.Close()
		if hex.EncodeToString(h.Sum(nil)) != want {
			return fmt.Errorf("backup manifest mismatch %s", rel)
		}
	}
	return nil
}

func pruneBackups(backupDir string, keep int) {
	ents, _ := os.ReadDir(backupDir)
	var dirs []string
	for _, e := range ents {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	sort.Strings(dirs) // timestamped names sort oldest first
	for i := 0; i+keep < len(dirs); i++ {
		os.RemoveAll(filepath.Join(backupDir, dirs[i]))
	}
}

// restoreDir copies the backup tree back into dst, removing files that are
// absent from the manifest, after the manifest verifies.
func restoreDir(bk, dst string) error {
	if err := verifyManifest(bk); err != nil {
		return fmt.Errorf("backup manifest invalid: %w", err)
	}
	m := map[string]string{}
	b, _ := os.ReadFile(filepath.Join(bk, ".manifest"))
	for _, ln := range strings.Split(string(b), "\n") {
		f := strings.Fields(ln)
		if len(f) == 2 {
			m[f[1]] = f[0]
		}
	}
	// remove dst files not in the manifest
	if err := filepath.Walk(dst, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dst, p)
		if _, ok := m[rel]; !ok {
			os.Remove(p)
		}
		return nil
	}); err != nil {
		return err
	}
	_, err := backupDir(bk, dst)
	return err
}
