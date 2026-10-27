package backup

import (
	"fmt"
	"io"

	"filippo.io/age"
)

// encrypt copies plaintext from src into dst, encrypted with an age scrypt
// recipient derived from passphrase. The passphrase is never logged or
// persisted by this package.
func encrypt(dst io.Writer, src io.Reader, passphrase string) (int64, error) {
	recipient, err := age.NewScryptRecipient(passphrase)
	if err != nil {
		return 0, fmt.Errorf("failed to create age recipient: %w", err)
	}
	w, err := age.Encrypt(dst, recipient)
	if err != nil {
		return 0, fmt.Errorf("failed to initialize age encryption: %w", err)
	}
	n, err := io.Copy(w, src)
	if err != nil {
		_ = w.Close()
		return 0, fmt.Errorf("failed to encrypt backup payload: %w", err)
	}
	if err := w.Close(); err != nil {
		return 0, fmt.Errorf("failed to finalize age encryption: %w", err)
	}
	return n, nil
}

// decrypt returns a reader yielding the plaintext of the age-encrypted src,
// using a scrypt identity derived from passphrase.
func decrypt(src io.Reader, passphrase string) (io.Reader, error) {
	identity, err := age.NewScryptIdentity(passphrase)
	if err != nil {
		return nil, fmt.Errorf("failed to create age identity: %w", err)
	}
	r, err := age.Decrypt(src, identity)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt backup: %w", err)
	}
	return r, nil
}
