package eventstream

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func headerValue(t *testing.T, raw, name string) string {
	t.Helper()
	headers, err := parseHeaders(raw)
	require.NoError(t, err)
	return headers[name]
}

// storedConfig returns the unmasked stored config of an integration, which
// List never exposes.
func storedConfig(t *testing.T, mgr *Manager, accountID string, id int64) Config {
	t.Helper()
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	in := mgr.findLocked(accountID, id)
	require.NotNil(t, in)
	return in.Config
}

func TestCreateListRoundtripMasksSecrets(t *testing.T) {
	mgr := NewManager(t.TempDir())

	created, err := mgr.Create(context.Background(), "acc-1", PlatformGenericHTTP, true, Config{
		URL:     "https://receiver.internal/events",
		Headers: `{"Authorization":"Bearer secret-token","X-Team":"blue"}`,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), created.ID)
	assert.Equal(t, "********", headerValue(t, created.Config.Headers, "Authorization"))
	assert.Equal(t, "blue", headerValue(t, created.Config.Headers, "X-Team"))

	list := mgr.List("acc-1")
	require.Len(t, list, 1)
	assert.True(t, list[0].Enabled)
	assert.Equal(t, PlatformGenericHTTP, list[0].Platform)
	assert.Equal(t, "https://receiver.internal/events", list[0].Config.URL)
	assert.NotContains(t, list[0].Config.Headers, "secret-token")

	// The unmasked copy for delivery still carries the stored secret.
	enabled := mgr.EnabledFor("acc-1")
	require.NotNil(t, enabled)
	assert.Equal(t, "Bearer secret-token", headerValue(t, enabled.Config.Headers, "Authorization"))
}

func TestCreateRejectsUnsupportedPlatforms(t *testing.T) {
	mgr := NewManager(t.TempDir())

	for _, platform := range []string{PlatformS3, PlatformFirehose} {
		_, err := mgr.Create(context.Background(), "acc-1", platform, true, Config{})
		require.ErrorIs(t, err, ErrPlatformNotSupported, "platform %s", platform)
	}

	_, err := mgr.Create(context.Background(), "acc-1", "bogus", true, Config{})
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrPlatformNotSupported)
}

func TestCreateValidatesConfig(t *testing.T) {
	mgr := NewManager(t.TempDir())

	_, err := mgr.Create(context.Background(), "acc-1", PlatformGenericHTTP, true, Config{})
	assert.ErrorContains(t, err, "config.url")

	_, err = mgr.Create(context.Background(), "acc-1", PlatformGenericHTTP, true, Config{URL: "ftp://x"})
	assert.ErrorContains(t, err, "http")

	_, err = mgr.Create(context.Background(), "acc-1", PlatformGenericHTTP, true, Config{
		URL:     "https://x",
		Headers: "not-json",
	})
	assert.ErrorContains(t, err, "config.headers")

	_, err = mgr.Create(context.Background(), "acc-1", PlatformGenericHTTP, true, Config{
		URL:          "https://x",
		BodyTemplate: "{{",
	})
	assert.ErrorContains(t, err, "body_template")

	_, err = mgr.Create(context.Background(), "acc-1", PlatformDatadog, true, Config{
		APIURL: "https://http-intake.logs.datadoghq.com/api/v2/logs",
	})
	assert.ErrorContains(t, err, "api_key")
}

func TestEnableOneAtATime(t *testing.T) {
	mgr := NewManager(t.TempDir())
	ctx := context.Background()

	first, err := mgr.Create(ctx, "acc-1", PlatformGenericHTTP, true, Config{URL: "https://one"})
	require.NoError(t, err)
	second, err := mgr.Create(ctx, "acc-1", PlatformDatadog, true, Config{
		APIKey: "dd-key", APIURL: "https://http-intake.logs.datadoghq.com/api/v2/logs",
	})
	require.NoError(t, err)

	list := mgr.List("acc-1")
	require.Len(t, list, 2)
	var enabledCount int
	for _, i := range list {
		if i.Enabled {
			enabledCount++
			assert.Equal(t, second.ID, i.ID)
		}
	}
	assert.Equal(t, 1, enabledCount)

	// Enabling the first one again over update disables the second.
	on := true
	updated, err := mgr.Update(ctx, "acc-1", first.ID, &on, nil)
	require.NoError(t, err)
	assert.True(t, updated.Enabled)

	enabled := mgr.EnabledFor("acc-1")
	require.NotNil(t, enabled)
	assert.Equal(t, first.ID, enabled.ID)
	assert.Equal(t, "https://one", enabled.Config.URL)
}

func TestUpdateKeepsMaskedSecrets(t *testing.T) {
	mgr := NewManager(t.TempDir())
	ctx := context.Background()

	in, err := mgr.Create(ctx, "acc-1", PlatformDatadog, true, Config{
		APIKey: "real-dd-key",
		APIURL: "https://http-intake.logs.datadoghq.com/api/v2/logs",
	})
	require.NoError(t, err)
	assert.Equal(t, "********", in.Config.APIKey)

	// Sending back the masked key keeps the stored one.
	_, err = mgr.Update(ctx, "acc-1", in.ID, nil, &Config{
		APIKey: "********",
		APIURL: "https://http-intake.logs.eu.example/api/v2/logs",
	})
	require.NoError(t, err)
	stored := storedConfig(t, mgr, "acc-1", in.ID)
	assert.Equal(t, "real-dd-key", stored.APIKey)
	assert.Equal(t, "https://http-intake.logs.eu.example/api/v2/logs", stored.APIURL)

	// An explicit new key replaces the stored one.
	_, err = mgr.Update(ctx, "acc-1", in.ID, nil, &Config{
		APIKey: "new-dd-key",
		APIURL: "https://http-intake.logs.datadoghq.com/api/v2/logs",
	})
	require.NoError(t, err)
	assert.Equal(t, "new-dd-key", storedConfig(t, mgr, "acc-1", in.ID).APIKey)
}

func TestUpdateKeepsMaskedHeaderSecrets(t *testing.T) {
	mgr := NewManager(t.TempDir())
	ctx := context.Background()

	in, err := mgr.Create(ctx, "acc-1", PlatformGenericHTTP, true, Config{
		URL:     "https://receiver.internal/events",
		Headers: `{"Authorization":"Bearer secret-token","X-Team":"blue"}`,
	})
	require.NoError(t, err)

	// The masked GET payload sent back on update keeps the secret header.
	_, err = mgr.Update(ctx, "acc-1", in.ID, nil, &Config{
		URL:     "https://other.internal/events",
		Headers: `{"Authorization":"********","X-Team":"red"}`,
	})
	require.NoError(t, err)
	stored := storedConfig(t, mgr, "acc-1", in.ID)
	assert.Equal(t, "https://other.internal/events", stored.URL)
	assert.Equal(t, "Bearer secret-token", headerValue(t, stored.Headers, "Authorization"))
	assert.Equal(t, "red", headerValue(t, stored.Headers, "X-Team"))
}

func TestDeleteAndNotFound(t *testing.T) {
	mgr := NewManager(t.TempDir())
	ctx := context.Background()

	in, err := mgr.Create(ctx, "acc-1", PlatformGenericHTTP, true, Config{URL: "https://one"})
	require.NoError(t, err)

	require.NoError(t, mgr.Delete(ctx, "acc-1", in.ID))
	assert.Empty(t, mgr.List("acc-1"))
	assert.Nil(t, mgr.EnabledFor("acc-1"))

	require.ErrorIs(t, mgr.Delete(ctx, "acc-1", in.ID), ErrNotFound)
	_, err = mgr.Update(ctx, "acc-1", in.ID, nil, nil)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestAccountIsolationAndPersistence(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	mgr := NewManager(dir)

	_, err := mgr.Create(ctx, "acc-1", PlatformGenericHTTP, true, Config{URL: "https://one"})
	require.NoError(t, err)

	assert.Empty(t, mgr.List("acc-2"))
	assert.Nil(t, mgr.EnabledFor("acc-2"))

	// A fresh manager over the same datadir sees the stored integration.
	reloaded := NewManager(dir)
	list := reloaded.List("acc-1")
	require.Len(t, list, 1)
	assert.Equal(t, "https://one", list[0].Config.URL)
}
