package eventstream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/netbirdio/netbird/util"
)

// configFileName is where the integrations are persisted, inside the datadir.
// The file holds api keys, so it is written with restricted permissions.
const configFileName = "eventstream.json"

// storage is the on-disk shape of eventstream.json.
type storage struct {
	// NextID is the global identifier counter shared by all accounts.
	NextID int64 `json:"next_id"`
	// Integrations maps an account ID to its configured integrations.
	Integrations map[string][]*Integration `json:"integrations"`
}

// Manager stores per-account event streaming integrations and persists them
// to eventstream.json in the datadir.
type Manager struct {
	dataDir string
	now     func() time.Time

	mu    sync.Mutex
	state storage
}

// NewManager creates a Manager persisting to eventstream.json in dataDir. A
// missing file starts empty. A corrupt file is logged and ignored.
func NewManager(dataDir string) *Manager {
	m := &Manager{
		dataDir: dataDir,
		now:     time.Now,
		state: storage{
			NextID:       1,
			Integrations: map[string][]*Integration{},
		},
	}
	if err := m.load(); err != nil {
		log.Warnf("ignoring persisted event streaming config %s: %v", m.configPath(), err)
	}
	return m
}

// configPath is the persisted state file location.
func (m *Manager) configPath() string {
	return filepath.Join(m.dataDir, configFileName)
}

// load reads eventstream.json. A missing file is not an error.
func (m *Manager) load() error {
	data, err := os.ReadFile(m.configPath())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	var st storage
	if err := json.Unmarshal(data, &st); err != nil {
		return err
	}
	if st.Integrations == nil {
		st.Integrations = map[string][]*Integration{}
	}
	if st.NextID < 1 {
		st.NextID = 1
	}
	m.state = st
	return nil
}

// persistLocked writes the state file. The caller must hold m.mu.
func (m *Manager) persistLocked(ctx context.Context) error {
	if err := util.WriteJsonWithRestrictedPermission(ctx, m.configPath(), m.state); err != nil {
		return fmt.Errorf("failed to persist event streaming config: %w", err)
	}
	return nil
}

// List returns the account's integrations with secrets masked for API
// responses.
func (m *Manager) List(accountID string) []*Integration {
	m.mu.Lock()
	defer m.mu.Unlock()

	list := m.state.Integrations[accountID]
	out := make([]*Integration, 0, len(list))
	for _, i := range list {
		out = append(out, i.Masked())
	}
	return out
}

// EnabledFor returns a copy of the account's enabled integration, or nil when
// none is enabled. The returned copy carries unmasked secrets for delivery.
func (m *Manager) EnabledFor(accountID string) *Integration {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, i := range m.state.Integrations[accountID] {
		if i.Enabled {
			out := *i
			return &out
		}
	}
	return nil
}

// Create validates and stores a new integration for the account and returns
// it with secrets masked. Enabling it disables the account's other
// integrations, keeping at most one enabled destination per account.
func (m *Manager) Create(ctx context.Context, accountID, platform string, enabled bool, cfg Config) (*Integration, error) {
	if err := validatePlatform(platform); err != nil {
		return nil, err
	}
	if err := validateConfig(platform, cfg); err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now().UTC()
	in := &Integration{
		ID:        m.state.NextID,
		AccountID: accountID,
		Enabled:   enabled,
		Platform:  platform,
		CreatedAt: now,
		UpdatedAt: now,
		Config:    cfg,
	}
	m.state.NextID++
	if enabled {
		m.disableOthersLocked(accountID, in.ID)
	}
	m.state.Integrations[accountID] = append(m.state.Integrations[accountID], in)

	if err := m.persistLocked(ctx); err != nil {
		return nil, err
	}
	return in.Masked(), nil
}

// Update modifies an existing integration. A nil enabled keeps the current
// state, a nil cfg keeps the current config. Empty or masked secret fields in
// cfg keep the stored values. Enabling the integration disables the account's
// other integrations.
func (m *Manager) Update(ctx context.Context, accountID string, id int64, enabled *bool, cfg *Config) (*Integration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	in := m.findLocked(accountID, id)
	if in == nil {
		return nil, ErrNotFound
	}

	merged := in.Config
	if cfg != nil {
		var err error
		merged, err = cfg.mergeSecrets(in.Config)
		if err != nil {
			return nil, err
		}
	}
	if err := validateConfig(in.Platform, merged); err != nil {
		return nil, err
	}

	in.Config = merged
	if enabled != nil {
		in.Enabled = *enabled
	}
	if in.Enabled {
		m.disableOthersLocked(accountID, in.ID)
	}
	in.UpdatedAt = m.now().UTC()

	if err := m.persistLocked(ctx); err != nil {
		return nil, err
	}
	return in.Masked(), nil
}

// Delete removes an integration from the account.
func (m *Manager) Delete(ctx context.Context, accountID string, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	list := m.state.Integrations[accountID]
	for idx, i := range list {
		if i.ID != id {
			continue
		}
		m.state.Integrations[accountID] = append(list[:idx], list[idx+1:]...)
		if len(m.state.Integrations[accountID]) == 0 {
			delete(m.state.Integrations, accountID)
		}
		return m.persistLocked(ctx)
	}
	return ErrNotFound
}

// findLocked locates an integration by ID on the account. The caller must
// hold m.mu.
func (m *Manager) findLocked(accountID string, id int64) *Integration {
	for _, i := range m.state.Integrations[accountID] {
		if i.ID == id {
			return i
		}
	}
	return nil
}

// disableOthersLocked clears Enabled on every account integration except
// keepID. The caller must hold m.mu.
func (m *Manager) disableOthersLocked(accountID string, keepID int64) {
	for _, i := range m.state.Integrations[accountID] {
		if i.ID != keepID && i.Enabled {
			i.Enabled = false
			i.UpdatedAt = m.now().UTC()
		}
	}
}
