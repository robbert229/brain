// Package status publishes immutable snapshots of braind's operational state.
package status

import (
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/johnrowl/brain/internal/braind/config"
	"github.com/johnrowl/brain/internal/braind/lifecycle"
)

type OIDCDiscoveryState string
type VaultState string
type SyncState string
type BackupState string

const (
	OIDCDisabled OIDCDiscoveryState = "disabled"
	OIDCPending  OIDCDiscoveryState = "pending"
	OIDCReady    OIDCDiscoveryState = "ready"
	OIDCDegraded OIDCDiscoveryState = "degraded"

	VaultUnavailable VaultState = "unavailable"
	VaultScanning    VaultState = "scanning"
	VaultReady       VaultState = "ready"
	VaultReadOnly    VaultState = "read-only"
	VaultDegraded    VaultState = "degraded"

	SyncDisabled            SyncState = "disabled"
	SyncChecking            SyncState = "checking"
	SyncNeedsAuthentication SyncState = "needs-authentication"
	SyncNeedsSetup          SyncState = "needs-setup"
	SyncStarting            SyncState = "starting"
	SyncRunning             SyncState = "running"
	SyncBackoff             SyncState = "backoff"
	SyncStopped             SyncState = "stopped"

	BackupDisabled BackupState = "disabled"
	BackupChecking BackupState = "checking"
	BackupReady    BackupState = "ready"
	BackupRunning  BackupState = "running"
	BackupBackoff  BackupState = "backoff"
	BackupBlocked  BackupState = "blocked"
	BackupDegraded BackupState = "degraded"
)

// Snapshot is the internal point-in-time status model. It deliberately has no
// fields for client secrets, tokens, raw claims, credentials, or remote URLs.
type Snapshot struct {
	Build     Build
	Lifecycle lifecycle.Snapshot
	Vault     Vault
	OIDC      OIDC
	Search    Search
	Sync      Sync
	Backup    Backup
}

type Build struct {
	Version   string
	Commit    string
	StartedAt time.Time
}

type Vault struct {
	State    VaultState
	ReadOnly bool
}

type OIDC struct {
	Enabled        bool
	Discovery      OIDCDiscoveryState
	Issuer         string
	ClientID       string
	ProviderName   string
	GroupsClaim    string
	GroupsSource   string
	AdminGroup     string
	UserGroup      string
	ViewerGroup    string
	ActiveSessions int
}

type Search struct {
	CompletedAt time.Time
	Duration    time.Duration
	Files       int
	Bytes       int64
	Results     int
	Truncated   bool
}

type Sync struct {
	State        SyncState
	OBVersion    string
	ChildPID     int
	ChildStarted time.Time
	LastExitCode *int
	RestartCount uint64
	LastError    string   // Sanitized before publication.
	OutputTail   []string // Sanitized before publication.
}

type Backup struct {
	State               BackupState
	LastCommitID        string
	LastCommitAt        time.Time
	LastPushAt          time.Time
	PendingLocalCommits int
	NextAttemptAt       time.Time
	LastError           string // Sanitized before publication.
}

// Initial returns the safe placeholder state used before subsystem discovery.
func Initial(version, commit string, processConfig config.Config, processLifecycle lifecycle.Snapshot, startedAt time.Time) Snapshot {
	oidcConfig := processConfig.OIDC()
	syncConfig := processConfig.Sync()
	backupConfig := processConfig.Backup()

	oidcState := OIDCDisabled
	if oidcConfig.Enabled {
		oidcState = OIDCPending
	}
	syncState := SyncDisabled
	if syncConfig.Enabled {
		syncState = SyncChecking
	}
	backupState := BackupDisabled
	if backupConfig.Enabled {
		backupState = BackupChecking
	}

	return Snapshot{
		Build: Build{
			Version:   version,
			Commit:    commit,
			StartedAt: startedAt.UTC(),
		},
		Lifecycle: processLifecycle,
		Vault: Vault{
			State:    VaultScanning,
			ReadOnly: processConfig.Vault().ReadOnly,
		},
		OIDC: OIDC{
			Enabled:      oidcConfig.Enabled,
			Discovery:    oidcState,
			ClientID:     oidcConfig.ClientID,
			ProviderName: oidcConfig.ProviderName,
			GroupsClaim:  oidcConfig.GroupsClaim,
			GroupsSource: oidcConfig.GroupsSource,
			AdminGroup:   oidcConfig.AdminGroup,
			UserGroup:    oidcConfig.UserGroup,
			ViewerGroup:  oidcConfig.ViewerGroup,
		},
		Sync:   Sync{State: syncState},
		Backup: Backup{State: backupState},
	}
}

// Store serializes writers and atomically publishes immutable snapshots.
type Store struct {
	writers sync.Mutex
	value   atomic.Pointer[Snapshot]
	secrets []string
}

// New returns a store containing a cloned initial snapshot. Each supplied
// configured secret is scrubbed from every subsequently published string.
func New(initial Snapshot, secrets ...string) *Store {
	store := &Store{secrets: uniqueSecrets(secrets)}
	store.store(initial)
	return store
}

// Load returns a defensive copy of the current snapshot.
func (s *Store) Load() Snapshot {
	current := s.value.Load()
	if current == nil {
		return Snapshot{}
	}
	return clone(*current)
}

// Publish replaces the current snapshot with a defensive copy.
func (s *Store) Publish(snapshot Snapshot) {
	s.writers.Lock()
	defer s.writers.Unlock()
	s.store(snapshot)
}

// Update serializes a read-modify-publish operation. The callback receives a
// private copy and cannot mutate either the old or newly published snapshot.
func (s *Store) Update(update func(*Snapshot)) {
	if update == nil {
		return
	}
	s.writers.Lock()
	defer s.writers.Unlock()
	next := s.Load()
	update(&next)
	s.store(next)
}

// SetLifecycle publishes a new process lifecycle view without disturbing
// subsystem status.
func (s *Store) SetLifecycle(snapshot lifecycle.Snapshot) {
	s.Update(func(status *Snapshot) {
		status.Lifecycle = snapshot
	})
}

func (s *Store) store(snapshot Snapshot) {
	copy := clone(snapshot)
	sanitize(&copy, s.secrets)
	s.value.Store(&copy)
}

func uniqueSecrets(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool {
		return len(result[left]) > len(result[right])
	})
	return result
}

func sanitize(snapshot *Snapshot, secrets []string) {
	redact := func(value string) string {
		for _, secret := range secrets {
			value = strings.ReplaceAll(value, secret, "[redacted]")
		}
		return value
	}
	snapshot.Build.Version = redact(snapshot.Build.Version)
	snapshot.Build.Commit = redact(snapshot.Build.Commit)
	snapshot.OIDC.Issuer = redact(snapshot.OIDC.Issuer)
	snapshot.OIDC.ClientID = redact(snapshot.OIDC.ClientID)
	snapshot.OIDC.ProviderName = redact(snapshot.OIDC.ProviderName)
	snapshot.OIDC.GroupsClaim = redact(snapshot.OIDC.GroupsClaim)
	snapshot.OIDC.GroupsSource = redact(snapshot.OIDC.GroupsSource)
	snapshot.OIDC.AdminGroup = redact(snapshot.OIDC.AdminGroup)
	snapshot.OIDC.UserGroup = redact(snapshot.OIDC.UserGroup)
	snapshot.OIDC.ViewerGroup = redact(snapshot.OIDC.ViewerGroup)
	snapshot.Sync.OBVersion = redact(snapshot.Sync.OBVersion)
	snapshot.Sync.LastError = redact(snapshot.Sync.LastError)
	for index := range snapshot.Sync.OutputTail {
		snapshot.Sync.OutputTail[index] = redact(snapshot.Sync.OutputTail[index])
	}
	snapshot.Backup.LastCommitID = redact(snapshot.Backup.LastCommitID)
	snapshot.Backup.LastError = redact(snapshot.Backup.LastError)
}

func clone(snapshot Snapshot) Snapshot {
	snapshot.Sync.OutputTail = append([]string(nil), snapshot.Sync.OutputTail...)
	if snapshot.Sync.LastExitCode != nil {
		exitCode := *snapshot.Sync.LastExitCode
		snapshot.Sync.LastExitCode = &exitCode
	}
	return snapshot
}

// MarshalJSON exposes only the explicit public status allowlist.
func (s Snapshot) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.Public(time.Now()))
}

// MarshalJSON serializes the store's current public allowlisted view.
func (s *Store) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.Load().Public(time.Now()))
}
