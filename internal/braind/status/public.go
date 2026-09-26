package status

import (
	"time"

	"github.com/johnrowl/brain/internal/braind/lifecycle"
)

type PublicSnapshot struct {
	Build     PublicBuild     `json:"build"`
	Lifecycle PublicLifecycle `json:"lifecycle"`
	Vault     PublicVault     `json:"vault"`
	OIDC      PublicOIDC      `json:"oidc"`
	Search    PublicSearch    `json:"search"`
	Sync      PublicSync      `json:"sync"`
	Backup    PublicBackup    `json:"backup"`
}

type PublicVault struct {
	State    VaultState `json:"state"`
	ReadOnly bool       `json:"read_only"`
}

type PublicOIDC struct {
	Enabled        bool               `json:"enabled"`
	Discovery      OIDCDiscoveryState `json:"discovery"`
	Issuer         string             `json:"issuer,omitempty"`
	ClientID       string             `json:"client_id,omitempty"`
	ProviderName   string             `json:"provider_name,omitempty"`
	GroupsClaim    string             `json:"groups_claim"`
	GroupsSource   string             `json:"groups_source"`
	AdminGroup     string             `json:"admin_group,omitempty"`
	UserGroup      string             `json:"user_group,omitempty"`
	ViewerGroup    string             `json:"viewer_group,omitempty"`
	ActiveSessions int                `json:"active_sessions"`
}

type PublicBuild struct {
	Version       string    `json:"version"`
	Commit        string    `json:"commit,omitempty"`
	StartedAt     time.Time `json:"started_at"`
	UptimeSeconds int64     `json:"uptime_seconds"`
}

type PublicLifecycle struct {
	State     lifecycle.State `json:"state"`
	Ready     bool            `json:"ready"`
	ChangedAt time.Time       `json:"changed_at"`
}

type PublicSearch struct {
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	DurationMillis int64      `json:"duration_millis"`
	Files          int        `json:"files"`
	Bytes          int64      `json:"bytes"`
	Results        int        `json:"results"`
	Truncated      bool       `json:"truncated"`
}

type PublicSync struct {
	State          SyncState  `json:"state"`
	OBVersion      string     `json:"ob_version,omitempty"`
	ChildPID       int        `json:"child_pid,omitempty"`
	ChildStartedAt *time.Time `json:"child_started_at,omitempty"`
	LastExitCode   *int       `json:"last_exit_code,omitempty"`
	RestartCount   uint64     `json:"restart_count"`
	LastError      string     `json:"last_error,omitempty"`
	OutputTail     []string   `json:"output_tail,omitempty"`
}

type PublicBackup struct {
	State               BackupState `json:"state"`
	LastCommitID        string      `json:"last_commit_id,omitempty"`
	LastCommitAt        *time.Time  `json:"last_commit_at,omitempty"`
	LastPushAt          *time.Time  `json:"last_push_at,omitempty"`
	PendingLocalCommits int         `json:"pending_local_commits"`
	NextAttemptAt       *time.Time  `json:"next_attempt_at,omitempty"`
	LastError           string      `json:"last_error,omitempty"`
}

// Public converts a snapshot into the stable external status contract.
func (s Snapshot) Public(now time.Time) PublicSnapshot {
	uptime := time.Duration(0)
	if !s.Build.StartedAt.IsZero() {
		uptime = now.Sub(s.Build.StartedAt)
		if uptime < 0 {
			uptime = 0
		}
	}
	return PublicSnapshot{
		Build: PublicBuild{
			Version:       s.Build.Version,
			Commit:        s.Build.Commit,
			StartedAt:     s.Build.StartedAt,
			UptimeSeconds: int64(uptime / time.Second),
		},
		Lifecycle: PublicLifecycle{
			State:     s.Lifecycle.State,
			Ready:     s.Lifecycle.Ready,
			ChangedAt: s.Lifecycle.ChangedAt,
		},
		Vault: PublicVault{
			State:    s.Vault.State,
			ReadOnly: s.Vault.ReadOnly,
		},
		OIDC: PublicOIDC{
			Enabled:        s.OIDC.Enabled,
			Discovery:      s.OIDC.Discovery,
			Issuer:         s.OIDC.Issuer,
			ClientID:       s.OIDC.ClientID,
			ProviderName:   s.OIDC.ProviderName,
			GroupsClaim:    s.OIDC.GroupsClaim,
			GroupsSource:   s.OIDC.GroupsSource,
			AdminGroup:     s.OIDC.AdminGroup,
			UserGroup:      s.OIDC.UserGroup,
			ViewerGroup:    s.OIDC.ViewerGroup,
			ActiveSessions: s.OIDC.ActiveSessions,
		},
		Search: publicSearch(s.Search),
		Sync:   publicSync(s.Sync),
		Backup: publicBackup(s.Backup),
	}
}

func publicSearch(search Search) PublicSearch {
	return PublicSearch{
		CompletedAt:    optionalTime(search.CompletedAt),
		DurationMillis: search.Duration.Milliseconds(),
		Files:          search.Files,
		Bytes:          search.Bytes,
		Results:        search.Results,
		Truncated:      search.Truncated,
	}
}

func publicSync(syncStatus Sync) PublicSync {
	result := PublicSync{
		State:          syncStatus.State,
		OBVersion:      syncStatus.OBVersion,
		ChildPID:       syncStatus.ChildPID,
		ChildStartedAt: optionalTime(syncStatus.ChildStarted),
		RestartCount:   syncStatus.RestartCount,
		LastError:      syncStatus.LastError,
		OutputTail:     append([]string(nil), syncStatus.OutputTail...),
	}
	if syncStatus.LastExitCode != nil {
		exitCode := *syncStatus.LastExitCode
		result.LastExitCode = &exitCode
	}
	return result
}

func publicBackup(backup Backup) PublicBackup {
	return PublicBackup{
		State:               backup.State,
		LastCommitID:        backup.LastCommitID,
		LastCommitAt:        optionalTime(backup.LastCommitAt),
		LastPushAt:          optionalTime(backup.LastPushAt),
		PendingLocalCommits: backup.PendingLocalCommits,
		NextAttemptAt:       optionalTime(backup.NextAttemptAt),
		LastError:           backup.LastError,
	}
}

func optionalTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	copy := value.UTC()
	return &copy
}
