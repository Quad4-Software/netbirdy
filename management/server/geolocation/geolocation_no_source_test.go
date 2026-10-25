package geolocation

import (
	"context"
	"net"
	"testing"
)

// A server with no configured download URL and no local database files must
// still start: NewGeolocation returns a disabled implementation instead of
// an error.
func TestNewGeolocationNoSourceNoFiles(t *testing.T) {
	t.Setenv("NB_GEOLOCATION_CITY_TAR_URL", "")
	t.Setenv("NB_GEOLOCATION_CITY_TAR_SHA256_URL", "")
	t.Setenv("NB_GEOLOCATION_CITY_ZIP_URL", "")
	t.Setenv("NB_GEOLOCATION_CITY_ZIP_SHA256_URL", "")

	geo, err := NewGeolocation(context.Background(), t.TempDir(), true)
	if err != nil {
		t.Fatalf("NewGeolocation should not fail without sources: %v", err)
	}
	defer geo.Stop()

	if _, err := geo.Lookup(net.ParseIP("1.2.3.4")); err != nil {
		// disabled impl returns an empty record, no error
		t.Fatalf("Lookup on disabled geolocation: %v", err)
	}
}
