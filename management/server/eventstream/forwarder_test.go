package eventstream

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netbirdio/netbird/management/server/activity"
)

// receiver collects deliveries of an httptest server.
type receiver struct {
	requests atomic.Int64
	failures atomic.Int64
	got      chan *http.Request
	bodies   chan []byte
	srv      *httptest.Server
}

// newReceiver starts a test endpoint that fails the first failFirst requests
// with a 500 and records every request on channels.
func newReceiver(t *testing.T, failFirst int64) *receiver {
	t.Helper()
	r := &receiver{
		got:    make(chan *http.Request, 16),
		bodies: make(chan []byte, 16),
	}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		_ = req.Body.Close()
		r.requests.Add(1)
		if r.failures.Add(1) <= failFirst {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		r.got <- req.Clone(context.Background())
		r.bodies <- body
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func testEvent(accountID string) *activity.Event {
	return &activity.Event{
		ID:             42,
		Timestamp:      time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Activity:       activity.UserCreated,
		InitiatorID:    "user-1",
		InitiatorName:  "Admin",
		InitiatorEmail: "admin@example.com",
		TargetID:       "user-2",
		AccountID:      accountID,
		Meta:           map[string]any{"email": "user-2@example.com"},
	}
}

// awaitRequest returns the next successful request within a deadline.
func awaitRequest(t *testing.T, r *receiver) (*http.Request, []byte) {
	t.Helper()
	select {
	case req := <-r.got:
		return req, <-r.bodies
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for delivery")
		return nil, nil
	}
}

func newTestForwarder(t *testing.T, mgr *Manager) *Forwarder {
	t.Helper()
	fwd := NewForwarder(mgr)
	fwd.retryDelay = func(int) time.Duration { return time.Millisecond }
	fwd.Start(context.Background())
	t.Cleanup(fwd.Stop)
	return fwd
}

func TestForwardGenericHTTP(t *testing.T) {
	mgr := NewManager(t.TempDir())
	fwd := newTestForwarder(t, mgr)

	recv := newReceiver(t, 0)
	_, err := mgr.Create(context.Background(), "acc-1", PlatformGenericHTTP, true, Config{
		URL:     recv.srv.URL,
		Headers: `{"Authorization":"Bearer abc123","X-Custom":"yes"}`,
	})
	require.NoError(t, err)

	store := NewStore(&activity.InMemoryEventStore{}, fwd)
	_, err = store.Save(context.Background(), testEvent("acc-1"))
	require.NoError(t, err)

	req, body := awaitRequest(t, recv)
	assert.Equal(t, "Bearer abc123", req.Header.Get("Authorization"))
	assert.Equal(t, "yes", req.Header.Get("X-Custom"))
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))

	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	assert.Equal(t, "user.create", payload["activity_code"])
	assert.Equal(t, "User created", payload["activity"])
	assert.Equal(t, "acc-1", payload["account_id"])
	assert.Equal(t, "user-2", payload["target_id"])
	assert.Equal(t, "user-2@example.com", payload["meta"].(map[string]any)["email"])
}

func TestForwardGenericHTTPBodyTemplate(t *testing.T) {
	mgr := NewManager(t.TempDir())
	fwd := newTestForwarder(t, mgr)

	recv := newReceiver(t, 0)
	_, err := mgr.Create(context.Background(), "acc-1", PlatformGenericHTTP, true, Config{
		URL:          recv.srv.URL,
		BodyTemplate: `{"event": "{{ .activity_code }}", "by": "{{ .initiator_id }}"}`,
	})
	require.NoError(t, err)

	fwd.Enqueue(testEvent("acc-1"))
	_, body := awaitRequest(t, recv)
	assert.JSONEq(t, `{"event": "user.create", "by": "user-1"}`, string(body))
}

func TestForwardDatadogMapping(t *testing.T) {
	mgr := NewManager(t.TempDir())
	fwd := newTestForwarder(t, mgr)

	recv := newReceiver(t, 0)
	_, err := mgr.Create(context.Background(), "acc-1", PlatformDatadog, true, Config{
		APIKey: "dd-secret-key",
		APIURL: recv.srv.URL + "/api/v2/logs",
	})
	require.NoError(t, err)

	fwd.Enqueue(testEvent("acc-1"))
	req, body := awaitRequest(t, recv)
	assert.Equal(t, "dd-secret-key", req.Header.Get("DD-API-KEY"))
	assert.Equal(t, "/api/v2/logs", req.URL.Path)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	assert.Equal(t, "user.create", payload["activity_code"])
}

func TestForwardRetriesOnFailure(t *testing.T) {
	mgr := NewManager(t.TempDir())
	fwd := newTestForwarder(t, mgr)

	recv := newReceiver(t, 2)
	_, err := mgr.Create(context.Background(), "acc-1", PlatformGenericHTTP, true, Config{URL: recv.srv.URL})
	require.NoError(t, err)

	fwd.Enqueue(testEvent("acc-1"))
	awaitRequest(t, recv)
	assert.Equal(t, int64(3), recv.requests.Load())
	assert.Equal(t, uint64(0), fwd.Dropped())
}

func TestForwardDropsAfterRetriesExhausted(t *testing.T) {
	mgr := NewManager(t.TempDir())
	fwd := newTestForwarder(t, mgr)

	recv := newReceiver(t, 100)
	_, err := mgr.Create(context.Background(), "acc-1", PlatformGenericHTTP, true, Config{URL: recv.srv.URL})
	require.NoError(t, err)

	fwd.Enqueue(testEvent("acc-1"))
	require.Eventually(t, func() bool { return fwd.Dropped() == 1 }, 10*time.Second, 10*time.Millisecond)
	assert.Equal(t, int64(maxAttempts), recv.requests.Load())
}

func TestEnqueueDropsWhenQueueFull(t *testing.T) {
	mgr := NewManager(t.TempDir())
	// Never started: nothing drains the queue.
	fwd := NewForwarder(mgr)
	fwd.queue = make(chan queuedEvent, 1)

	_, err := mgr.Create(context.Background(), "acc-1", PlatformGenericHTTP, true, Config{URL: "https://x"})
	require.NoError(t, err)

	fwd.Enqueue(testEvent("acc-1"))
	fwd.Enqueue(testEvent("acc-1"))
	assert.Equal(t, uint64(1), fwd.Dropped())
}

func TestEnqueueIgnoresAccountsWithoutIntegration(t *testing.T) {
	mgr := NewManager(t.TempDir())
	fwd := newTestForwarder(t, mgr)

	recv := newReceiver(t, 0)
	_, err := mgr.Create(context.Background(), "acc-1", PlatformGenericHTTP, false, Config{URL: recv.srv.URL})
	require.NoError(t, err)

	// The integration is disabled and the other account has none.
	fwd.Enqueue(testEvent("acc-1"))
	fwd.Enqueue(testEvent("acc-2"))

	assert.Never(t, func() bool { return recv.requests.Load() > 0 }, 200*time.Millisecond, 20*time.Millisecond)
}

func TestStoreSaveForwardsToQueue(t *testing.T) {
	mgr := NewManager(t.TempDir())
	fwd := newTestForwarder(t, mgr)

	recv := newReceiver(t, 0)
	_, err := mgr.Create(context.Background(), "acc-1", PlatformGenericHTTP, true, Config{URL: recv.srv.URL})
	require.NoError(t, err)

	inner := &activity.InMemoryEventStore{}
	store := NewStore(inner, fwd)

	_, err = store.Save(context.Background(), testEvent("acc-1"))
	require.NoError(t, err)

	awaitRequest(t, recv)

	events, err := store.Get(context.Background(), "acc-1", 0, 10, true)
	require.NoError(t, err)
	assert.Len(t, events, 1)
}
