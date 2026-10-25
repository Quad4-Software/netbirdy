package security

import (
	"context"
	"strings"
	"time"

	"github.com/netbirdio/netbird/management/server/activity"
)

// AuditLine is the stable jsonl schema mirrored to the audit log file.
type AuditLine struct {
	Timestamp      time.Time      `json:"ts"`
	Type           string         `json:"type"` // activity code string
	TypeID         int            `json:"type_id"`
	Severity       Severity       `json:"severity"`
	AccountID      string         `json:"account_id"`
	InitiatorID    string         `json:"initiator_id"`
	InitiatorName  string         `json:"initiator_name,omitempty"`
	InitiatorEmail string         `json:"initiator_email,omitempty"`
	TargetID       string         `json:"target_id"`
	Message        string         `json:"message"`
	Meta           map[string]any `json:"meta,omitempty"`
	Source         string         `json:"source"`
}

// securityActivities maps activity ids that deserve an elevated severity.
var securityActivities = map[activity.Activity]Severity{
	activity.PeerNewIPDetected:        SeverityHigh,
	activity.PeerConcurrentConnection: SeverityHigh,
	activity.PeerIPFlapping:           SeverityMedium,
	activity.PeerMetaMismatch:         SeverityMedium,
	activity.AuthFailureBurst:         SeverityHigh,
	activity.SetupKeyMisuseDetected:   SeverityHigh,
	activity.UserLoginAnomaly:         SeverityMedium,
	activity.PATAnomaly:               SeverityMedium,
}

// sensitiveMetaKeys are dropped from the mirrored meta.
var sensitiveMetaKeys = []string{"token", "secret", "password", "key"}

func sanitizeMeta(meta map[string]any) map[string]any {
	out := make(map[string]any, len(meta))
	for k, v := range meta {
		lk := strings.ToLower(k)
		sensitive := false
		for _, s := range sensitiveMetaKeys {
			if strings.Contains(lk, s) {
				sensitive = true
				break
			}
		}
		if sensitive {
			continue
		}
		if str, ok := v.(string); ok && len(str) > 256 {
			out[k] = str[:256]
			continue
		}
		out[k] = v
	}
	return out
}

// AuditStore decorates an activity.Store, mirroring each saved event as a
// jsonl line through a FileSink. Errors are logged, never returned.
type AuditStore struct {
	inner activity.Store
	sink  *FileSink
}

// NewAuditStore wraps inner with the jsonl mirror at path.
func NewAuditStore(inner activity.Store, path string) (*AuditStore, error) {
	fs, err := NewFileSink(path, 0, 0)
	if err != nil {
		return nil, err
	}
	return &AuditStore{inner: inner, sink: fs}, nil
}

// Save forwards to the inner store then mirrors the event.
func (a *AuditStore) Save(ctx context.Context, event *activity.Event) (*activity.Event, error) {
	saved, err := a.inner.Save(ctx, event)
	if err != nil {
		return saved, err
	}
	line := AuditLine{
		Timestamp:      saved.Timestamp,
		Type:           activity.Activity(saved.Activity).StringCode(),
		TypeID:         int(saved.Activity),
		Severity:       SeverityInfo,
		AccountID:      saved.AccountID,
		InitiatorID:    saved.InitiatorID,
		InitiatorName:  saved.InitiatorName,
		InitiatorEmail: saved.InitiatorEmail,
		TargetID:       saved.TargetID,
		Source:         "audit",
	}
	if sev, ok := securityActivities[saved.Activity]; ok {
		line.Severity = sev
	}
	line.Message = activity.Activity(saved.Activity).Message()
	if len(saved.Meta) > 0 {
		line.Meta = sanitizeMeta(saved.Meta)
	}
	if a.sink != nil {
		a.sink.EmitJSON(line)
	}
	return saved, nil
}

// Get delegates.
func (a *AuditStore) Get(ctx context.Context, accountID string, offset, limit int, descending bool) ([]*activity.Event, error) {
	return a.inner.Get(ctx, accountID, offset, limit, descending)
}

// Close delegates and flushes.
func (a *AuditStore) Close(ctx context.Context) error {
	return a.inner.Close(ctx)
}
