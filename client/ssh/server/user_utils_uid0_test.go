//go:build unix

package server

import (
	"os/user"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckPrivileges_RejectsRootAliasesAndNumericNames(t *testing.T) {
	current := createTestUser("root", "0", "0", "/root")
	lookups := map[string]*user.User{
		"toor":  createTestUser("toor", "0", "0", "/root"),
		"root":  createTestUser("root", "0", "0", "/root"),
		"alice": createTestUser("alice", "1000", "1000", "/home/alice"),
		// A getent style lookup that resolves a UID string to root.
		"0": createTestUser("root", "0", "0", "/root"),
	}

	tests := []struct {
		name      string
		requested string
		allowRoot bool
		allowed   bool
	}{
		{"alice_allowed", "alice", false, true},
		{"root_denied", "root", false, false},
		{"uid0_alias_denied", "toor", false, false},
		{"uid0_alias_allowed_with_root_enabled", "toor", true, true},
		{"numeric_zero_denied", "0", false, false},
		{"numeric_zero_denied_even_with_root_enabled", "0", true, false},
		{"leading_dash_denied", "-i", true, false},
		{"leading_dash_long_denied", "--no-idn", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := setupTestDependencies(current, nil, "linux", 0, lookups, nil)
			defer cleanup()

			s := &Server{allowRootLogin: tt.allowRoot}
			result := s.CheckPrivileges(PrivilegeCheckRequest{
				RequestedUsername:         tt.requested,
				FeatureSupportsUserSwitch: true,
				FeatureName:               FeatureSSHLogin,
			})
			assert.Equal(t, tt.allowed, result.Allowed)
		})
	}
}
