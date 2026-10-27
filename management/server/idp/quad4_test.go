package idp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netbirdio/netbird/management/server/telemetry"
)

func TestNewQuad4Manager(t *testing.T) {
	type test struct {
		name                 string
		inputConfig          Quad4ClientConfig
		assertErrFunc        require.ErrorAssertionFunc
		assertErrFuncMessage string
	}

	defaultTestConfig := Quad4ClientConfig{
		APIToken:           "api_token",
		ManagementEndpoint: "http://localhost",
	}

	tests := []test{
		{
			name:                 "Good Configuration",
			inputConfig:          defaultTestConfig,
			assertErrFunc:        require.NoError,
			assertErrFuncMessage: "shouldn't return error",
		},
		{
			name: "Missing ManagementEndpoint",
			inputConfig: Quad4ClientConfig{
				APIToken:           defaultTestConfig.APIToken,
				ManagementEndpoint: "",
			},
			assertErrFunc:        require.Error,
			assertErrFuncMessage: "should return error when field empty",
		},
		{
			name: "Missing APIToken",
			inputConfig: Quad4ClientConfig{
				APIToken:           "",
				ManagementEndpoint: defaultTestConfig.ManagementEndpoint,
			},
			assertErrFunc:        require.Error,
			assertErrFuncMessage: "should return error when field empty",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewQuad4Manager(tc.inputConfig, &telemetry.MockAppMetrics{})
			tc.assertErrFunc(t, err, tc.assertErrFuncMessage)
		})
	}
}

func TestNewManagerQuad4Type(t *testing.T) {
	manager, err := NewManager(context.Background(), Config{
		ManagerType: "quad4",
		ExtraConfig: ExtraConfig{
			"ApiToken":           "api_token",
			"ManagementEndpoint": "http://localhost",
		},
	}, &telemetry.MockAppMetrics{})
	require.NoError(t, err)
	require.IsType(t, &Quad4Manager{}, manager)
	assert.Equal(t, "Quad4 ID", manager.(*Quad4Manager).providerName)
}
