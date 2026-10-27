package eventstream

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"text/template"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/netbirdio/netbird/management/server/activity"
)

const (
	// queueSize bounds the buffered events waiting for delivery. Events
	// beyond the bound are dropped and counted rather than blocking the
	// activity store.
	queueSize = 4096
	// workers is the number of concurrent delivery goroutines.
	workers = 4
	// maxAttempts bounds the delivery retries of a single event.
	maxAttempts = 5
	// baseBackoff is the delay before the first retry, doubled each attempt.
	baseBackoff = 200 * time.Millisecond
	// maxBackoff caps the retry delay.
	maxBackoff = 5 * time.Second
	// requestTimeout bounds a single delivery request.
	requestTimeout = 15 * time.Second
)

// Payload is the JSON document delivered for one activity event. The field
// names mirror the dashboard audit view and double as the template data for
// body_template.
type Payload struct {
	ID             uint64         `json:"id"`
	Timestamp      time.Time      `json:"timestamp"`
	Activity       string         `json:"activity"`
	ActivityCode   string         `json:"activity_code"`
	ActivityID     int            `json:"activity_id"`
	InitiatorID    string         `json:"initiator_id"`
	InitiatorName  string         `json:"initiator_name,omitempty"`
	InitiatorEmail string         `json:"initiator_email,omitempty"`
	TargetID       string         `json:"target_id"`
	AccountID      string         `json:"account_id"`
	Meta           map[string]any `json:"meta,omitempty"`
}

// newPayload maps an activity event to its delivery payload.
func newPayload(e *activity.Event) *Payload {
	return &Payload{
		ID:             e.ID,
		Timestamp:      e.Timestamp,
		Activity:       e.Activity.Message(),
		ActivityCode:   e.Activity.StringCode(),
		ActivityID:     int(e.Activity),
		InitiatorID:    e.InitiatorID,
		InitiatorName:  e.InitiatorName,
		InitiatorEmail: e.InitiatorEmail,
		TargetID:       e.TargetID,
		AccountID:      e.AccountID,
		Meta:           e.Meta,
	}
}

// queuedEvent is one event waiting for delivery.
type queuedEvent struct {
	accountID string
	event     *activity.Event
}

// Forwarder delivers activity events to the enabled integration of each
// account. Delivery happens on background workers fed by a bounded queue, so
// Enqueue never blocks the activity store path.
type Forwarder struct {
	manager *Manager
	client  *http.Client

	queue chan queuedEvent

	// retryDelay and attempts are the retry policy, overridable in tests.
	retryDelay func(attempt int) time.Duration
	attempts   int

	dropped atomic.Uint64
	sent    atomic.Uint64
	failed  atomic.Uint64

	mu      sync.Mutex
	cancel  context.CancelFunc
	running bool
	wg      sync.WaitGroup
}

// NewForwarder creates a Forwarder reading integrations from manager.
func NewForwarder(manager *Manager) *Forwarder {
	return &Forwarder{
		manager:    manager,
		client:     &http.Client{Timeout: requestTimeout},
		queue:      make(chan queuedEvent, queueSize),
		retryDelay: backoff,
		attempts:   maxAttempts,
	}
}

// Start launches the delivery workers, stopped by ctx or Stop.
func (f *Forwarder) Start(ctx context.Context) {
	f.mu.Lock()
	if f.running {
		f.mu.Unlock()
		return
	}
	f.running = true
	wctx, cancel := context.WithCancel(ctx)
	f.cancel = cancel
	f.mu.Unlock()

	for i := 0; i < workers; i++ {
		f.wg.Add(1)
		go f.worker(wctx)
	}
}

// Stop halts the delivery workers. Pending queue entries are discarded.
func (f *Forwarder) Stop() {
	f.mu.Lock()
	cancel := f.cancel
	f.cancel = nil
	f.running = false
	f.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	f.wg.Wait()
}

// Dropped reports the number of events discarded because the queue was full
// or retries were exhausted.
func (f *Forwarder) Dropped() uint64 {
	return f.dropped.Load()
}

// Enqueue offers an event for delivery. It returns immediately: events for
// accounts without an enabled integration are ignored and a full queue drops
// the event while counting it.
func (f *Forwarder) Enqueue(event *activity.Event) {
	if event == nil || f.manager.EnabledFor(event.AccountID) == nil {
		return
	}
	select {
	case f.queue <- queuedEvent{accountID: event.AccountID, event: event}:
	default:
		f.dropped.Add(1)
		log.Debugf("event stream queue full, dropped event %d for account %s", event.ID, event.AccountID)
	}
}

// worker drains the queue until the context is cancelled.
func (f *Forwarder) worker(ctx context.Context) {
	defer f.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case q := <-f.queue:
			f.deliver(ctx, q)
		}
	}
}

// deliver resolves the current enabled integration at send time, so config
// updates take effect for already queued events, and retries the request.
func (f *Forwarder) deliver(ctx context.Context, q queuedEvent) {
	in := f.manager.EnabledFor(q.accountID)
	if in == nil {
		return
	}

	body, err := renderBody(in, q.event)
	if err != nil {
		// A broken template never gets better on retry.
		f.failed.Add(1)
		log.WithContext(ctx).Warnf("event stream integration %d for account %s: %v", in.ID, in.AccountID, err)
		return
	}

	var lastErr error
	for attempt := 1; attempt <= f.attempts; attempt++ {
		lastErr = f.sendOnce(ctx, in, body)
		if lastErr == nil {
			f.sent.Add(1)
			return
		}
		if !sleep(ctx, f.retryDelay(attempt)) {
			f.failed.Add(1)
			return
		}
	}
	f.dropped.Add(1)
	f.failed.Add(1)
	log.WithContext(ctx).Warnf("event stream integration %d for account %s dropped event after %d attempts: %v",
		in.ID, in.AccountID, f.attempts, sanitizeErr(lastErr))
}

// renderBody produces the request body: the event JSON by default, or the
// body template rendered over the event JSON fields for generic_http.
func renderBody(in *Integration, event *activity.Event) ([]byte, error) {
	payload := newPayload(event)

	if in.Platform != PlatformGenericHTTP || in.Config.BodyTemplate == "" {
		return json.Marshal(payload)
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}

	// Template errors are reduced to fixed messages: the error text can embed
	// fragments of the configured template, a per-account config value that
	// must not land in logs.
	tpl, err := template.New("body").Parse(in.Config.BodyTemplate)
	if err != nil {
		return nil, errors.New("failed to parse body template")
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, fields); err != nil {
		return nil, errors.New("failed to render body template")
	}
	return buf.Bytes(), nil
}

// sendOnce performs a single delivery attempt.
func (f *Forwarder) sendOnce(ctx context.Context, in *Integration, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint(in), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range requestHeaders(in) {
		req.Header.Set(k, v)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("endpoint returned status %d", resp.StatusCode)
}

// endpoint resolves the destination URL of the integration.
func endpoint(in *Integration) string {
	if in.Platform == PlatformDatadog {
		return in.Config.APIURL
	}
	return in.Config.URL
}

// requestHeaders resolves the per-request headers: configured headers for
// generic_http, the API key header for datadog.
func requestHeaders(in *Integration) map[string]string {
	if in.Platform == PlatformDatadog {
		return map[string]string{"DD-API-KEY": in.Config.APIKey}
	}
	headers, err := parseHeaders(in.Config.Headers)
	if err != nil {
		return nil
	}
	return headers
}

// backoff computes the retry delay for the given attempt (1-based).
func backoff(attempt int) time.Duration {
	d := baseBackoff << (attempt - 1)
	if d > maxBackoff || d < 0 {
		return maxBackoff
	}
	return d
}

// sleep waits for d or ctx cancellation, reporting whether the wait
// completed.
func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// sanitizeErr strips the request URL from url errors so log lines never carry
// endpoint details configured per account.
func sanitizeErr(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}

var _ activity.Store = (*Store)(nil)

// Store decorates an activity.Store, enqueueing every saved event for
// delivery. Event streaming failures never propagate to the store.
type Store struct {
	inner activity.Store
	fwd   *Forwarder
}

// NewStore wraps inner so saved events reach the forwarder.
func NewStore(inner activity.Store, fwd *Forwarder) *Store {
	return &Store{inner: inner, fwd: fwd}
}

// Save delegates to the inner store and enqueues the saved event.
func (s *Store) Save(ctx context.Context, event *activity.Event) (*activity.Event, error) {
	saved, err := s.inner.Save(ctx, event)
	if err != nil {
		return saved, err
	}
	// Copy so a caller mutating the event after Save cannot race delivery.
	s.fwd.Enqueue(saved.Copy())
	return saved, nil
}

// Get delegates to the inner store.
func (s *Store) Get(ctx context.Context, accountID string, offset, limit int, descending bool) ([]*activity.Event, error) {
	return s.inner.Get(ctx, accountID, offset, limit, descending)
}

// Close delegates to the inner store.
func (s *Store) Close(ctx context.Context) error {
	return s.inner.Close(ctx)
}
