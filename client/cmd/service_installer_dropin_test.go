package cmd

import "testing"

func TestLegacyNetbirdServicePresent(t *testing.T) {
	present := legacyNetbirdServicePresent(func(string) bool { return false })
	if present {
		t.Fatal("expected no legacy service")
	}
	present = legacyNetbirdServicePresent(func(p string) bool {
		return p == "/lib/systemd/system/netbird.service"
	})
	if !present {
		t.Fatal("expected legacy service detected")
	}
}
