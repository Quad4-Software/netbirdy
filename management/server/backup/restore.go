package backup

import (
	"context"
	"fmt"
)

// DownloadAndExtract fetches the encrypted snapshot stored under key,
// decrypts it with passphrase and extracts the archive into dir, returning
// the manifest and the names of the extracted store files.
func DownloadAndExtract(ctx context.Context, store ObjectStore, key, passphrase, dir string) (*Manifest, []string, error) {
	rc, err := store.Get(ctx, key)
	if err != nil {
		return nil, nil, err
	}
	defer rc.Close()

	plain, err := decrypt(rc, passphrase)
	if err != nil {
		return nil, nil, err
	}

	manifest, files, err := extractArchive(plain, dir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to extract %s: %w", key, err)
	}
	return manifest, files, nil
}
