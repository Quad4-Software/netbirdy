package reposign

import (
	"os"
	"path/filepath"
	"testing"
)

// The committed publickeys tree must validate against the embedded root.
func TestCommittedPublicKeysTree(t *testing.T) {
	rootKeys, err := loadEmbeddedPublicKeys()
	if err != nil {
		t.Fatalf("prod root keys: %v", err)
	}
	keysDir := filepath.Join("..", "..", "..", "..", "publickeys", "artifact-signatures", "keys")

	pubPEM, err := os.ReadFile(filepath.Join(keysDir, artifactPubKeysFileName))
	if err != nil {
		t.Fatalf("artifact pub bundle: %v", err)
	}
	sigBytes, err := os.ReadFile(filepath.Join(keysDir, artifactPubKeysSigFileName))
	if err != nil {
		t.Fatalf("artifact pub sig: %v", err)
	}
	sig, err := ParseSignature(sigBytes)
	if err != nil {
		t.Fatalf("decode sig: %v", err)
	}
	revBytes, err := os.ReadFile(filepath.Join(keysDir, revocationFileName))
	if err != nil {
		t.Fatalf("revocation list: %v", err)
	}
	revSigBytes, err := os.ReadFile(filepath.Join(keysDir, revocationSignFileName))
	if err != nil {
		t.Fatalf("revocation sig: %v", err)
	}
	revSig, err := ParseSignature(revSigBytes)
	if err != nil {
		t.Fatalf("decode revocation sig: %v", err)
	}
	revList, err := ValidateRevocationList(rootKeys, revBytes, *revSig)
	if err != nil {
		t.Fatalf("revocation list: %v", err)
	}
	if _, err := ValidateArtifactKeys(rootKeys, pubPEM, *sig, revList); err != nil {
		t.Fatalf("committed artifact keys must validate: %v", err)
	}
}
