package status

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/johnrowl/brain/internal/braind/config"
	"github.com/johnrowl/brain/internal/braind/lifecycle"
)

func TestInitialStatusAndPublicSerializationExcludeSecrets(t *testing.T) {
	const (
		clientSecret = "status-client-secret-marker"
		remoteURL    = "ssh://git@example.test/private-status-repository.git"
	)
	processConfig := loadConfig(t, map[string]string{
		config.EnvOIDCClientSecret: clientSecret,
		config.EnvGitRemoteURL:     remoteURL,
	})
	startedAt := time.Date(2026, time.September, 26, 12, 0, 0, 0, time.UTC)
	lifecycleSnapshot := lifecycle.Snapshot{
		State:     lifecycle.Starting,
		Ready:     false,
		ChangedAt: startedAt,
	}
	initial := Initial("v1.2.3", "abc123", processConfig, lifecycleSnapshot, startedAt)
	store := New(initial, clientSecret, remoteURL)
	store.Update(func(snapshot *Snapshot) {
		snapshot.OIDC.Issuer = "https://issuer.example.test/" + clientSecret
		snapshot.Sync.LastError = "sync rejected " + clientSecret
		snapshot.Sync.OutputTail = []string{"remote=" + remoteURL, "safe line"}
		snapshot.Backup.LastError = "push failed for " + remoteURL
	})

	public := store.Load().Public(startedAt.Add(90 * time.Second))
	if public.Build.UptimeSeconds != 90 || public.Build.Version != "v1.2.3" || public.Build.Commit != "abc123" {
		t.Fatalf("unexpected public build status: %+v", public.Build)
	}
	if public.OIDC.Discovery != OIDCPending || public.OIDC.ClientID != "brain-client" || public.OIDC.GroupsClaim != "groups" {
		t.Fatalf("unexpected public OIDC status: %+v", public.OIDC)
	}
	if public.Vault.State != VaultScanning || public.Sync.State != SyncChecking || public.Backup.State != BackupChecking {
		t.Fatalf("unexpected placeholder states: vault=%+v sync=%+v backup=%+v", public.Vault, public.Sync, public.Backup)
	}

	serialized, err := json.Marshal(store)
	if err != nil {
		t.Fatalf("marshal public status: %v", err)
	}
	output := string(serialized)
	for _, forbidden := range []string{clientSecret, remoteURL, "client_secret", "remote_url", "access_token", "raw_claims"} {
		if strings.Contains(output, forbidden) {
			t.Errorf("public status disclosed %q: %s", forbidden, output)
		}
	}
	for _, required := range []string{`"client_id":"brain-client"`, `"groups_claim":"groups"`, `"last_error":"sync rejected [redacted]"`, `"remote=[redacted]"`} {
		if !strings.Contains(output, required) {
			t.Errorf("public status does not contain %q: %s", required, output)
		}
	}
}

func TestStoreDefensivelyCopiesMutableFields(t *testing.T) {
	const secret = "copy-secret"
	exitCode := 7
	input := Snapshot{Sync: Sync{OutputTail: []string{"original " + secret}, LastExitCode: &exitCode}}
	store := New(input, secret)
	if input.Sync.OutputTail[0] != "original "+secret {
		t.Fatalf("publication mutated caller input: %q", input.Sync.OutputTail[0])
	}
	input.Sync.OutputTail[0] = "changed input"
	exitCode = 8

	first := store.Load()
	if first.Sync.OutputTail[0] != "original [redacted]" || *first.Sync.LastExitCode != 7 {
		t.Fatalf("published snapshot changed through input: %+v", first.Sync)
	}
	first.Sync.OutputTail[0] = "changed load"
	*first.Sync.LastExitCode = 9
	second := store.Load()
	if second.Sync.OutputTail[0] != "original [redacted]" || *second.Sync.LastExitCode != 7 {
		t.Fatalf("published snapshot changed through load: %+v", second.Sync)
	}

	var retained *Snapshot
	store.Update(func(snapshot *Snapshot) {
		retained = snapshot
		snapshot.Sync.OutputTail = append(snapshot.Sync.OutputTail, "published")
	})
	retained.Sync.OutputTail[0] = "changed callback"
	if got := store.Load().Sync.OutputTail[0]; got != "original [redacted]" {
		t.Fatalf("published snapshot changed through retained callback: %q", got)
	}
}

func TestPublicSnapshotIncludesOperationalFields(t *testing.T) {
	startedAt := time.Date(2026, time.September, 26, 12, 0, 0, 0, time.UTC)
	completedAt := startedAt.Add(time.Minute)
	childStartedAt := startedAt.Add(2 * time.Minute)
	commitAt := startedAt.Add(3 * time.Minute)
	pushAt := startedAt.Add(4 * time.Minute)
	nextAttemptAt := startedAt.Add(5 * time.Minute)
	exitCode := 23
	snapshot := Snapshot{
		Build:     Build{Version: "v1", Commit: "deadbeef", StartedAt: startedAt},
		Lifecycle: lifecycle.Snapshot{State: lifecycle.Degraded, Ready: true, ChangedAt: completedAt},
		Vault:     Vault{State: VaultReadOnly, ReadOnly: true},
		OIDC: OIDC{
			Enabled: true, Discovery: OIDCReady, Issuer: "https://issuer.example.test", ClientID: "brain",
			ProviderName: "Test ID", GroupsClaim: "roles", GroupsSource: "userinfo",
			AdminGroup: "admins", UserGroup: "users", ViewerGroup: "viewers", ActiveSessions: 4,
		},
		Search: Search{CompletedAt: completedAt, Duration: 1500 * time.Millisecond, Files: 20, Bytes: 4096, Results: 3, Truncated: true},
		Sync: Sync{
			State: SyncBackoff, OBVersion: "1.2.3", ChildPID: 123, ChildStarted: childStartedAt,
			LastExitCode: &exitCode, RestartCount: 5, LastError: "network unavailable", OutputTail: []string{"retrying"},
		},
		Backup: Backup{
			State: BackupBackoff, LastCommitID: "abc123", LastCommitAt: commitAt, LastPushAt: pushAt,
			PendingLocalCommits: 2, NextAttemptAt: nextAttemptAt, LastError: "push unavailable",
		},
	}

	public := snapshot.Public(startedAt.Add(10 * time.Minute))
	if public.Build.UptimeSeconds != 600 || public.Lifecycle.State != lifecycle.Degraded || !public.Lifecycle.Ready {
		t.Fatalf("unexpected build/lifecycle view: build=%+v lifecycle=%+v", public.Build, public.Lifecycle)
	}
	if public.Search.DurationMillis != 1500 || public.Search.Files != 20 || public.Search.Bytes != 4096 || public.Search.Results != 3 || !public.Search.Truncated || public.Search.CompletedAt == nil {
		t.Fatalf("unexpected search view: %+v", public.Search)
	}
	if public.Sync.ChildStartedAt == nil || public.Sync.LastExitCode == nil || *public.Sync.LastExitCode != 23 || public.Sync.RestartCount != 5 || len(public.Sync.OutputTail) != 1 {
		t.Fatalf("unexpected sync view: %+v", public.Sync)
	}
	if public.Backup.LastCommitAt == nil || public.Backup.LastPushAt == nil || public.Backup.NextAttemptAt == nil || public.Backup.PendingLocalCommits != 2 {
		t.Fatalf("unexpected backup view: %+v", public.Backup)
	}
	public.Sync.OutputTail[0] = "changed"
	if snapshot.Sync.OutputTail[0] != "retrying" {
		t.Fatal("public conversion did not copy the sync output tail")
	}
}

func TestStoreSupportsConcurrentReadersAndWriters(t *testing.T) {
	store := New(Snapshot{Sync: Sync{State: SyncChecking}})
	const writers = 8
	const updatesPerWriter = 100
	const readers = 16

	start := make(chan struct{})
	done := make(chan struct{})
	var writerWait sync.WaitGroup
	for range writers {
		writerWait.Add(1)
		go func() {
			defer writerWait.Done()
			<-start
			for range updatesPerWriter {
				store.Update(func(snapshot *Snapshot) {
					snapshot.Sync.RestartCount++
					snapshot.Sync.OutputTail = []string{"sanitized"}
				})
			}
		}()
	}
	var readerWait sync.WaitGroup
	for range readers {
		readerWait.Add(1)
		go func() {
			defer readerWait.Done()
			<-start
			for {
				select {
				case <-done:
					return
				default:
					snapshot := store.Load()
					if _, err := json.Marshal(snapshot); err != nil {
						t.Errorf("marshal concurrent snapshot: %v", err)
						return
					}
				}
			}
		}()
	}
	close(start)
	writerWait.Wait()
	close(done)
	readerWait.Wait()

	if got, want := store.Load().Sync.RestartCount, uint64(writers*updatesPerWriter); got != want {
		t.Fatalf("lost concurrent update: got %d, want %d", got, want)
	}
}

func TestInitialStatusReflectsDisabledSubsystems(t *testing.T) {
	processConfig := loadConfig(t, map[string]string{
		config.EnvOIDCEnabled:      "false",
		config.EnvGitBackupEnabled: "false",
		config.EnvSyncEnabled:      "false",
	})
	snapshot := Initial("dev", "", processConfig, lifecycle.New().Snapshot(), time.Now())
	if snapshot.OIDC.Discovery != OIDCDisabled || snapshot.Sync.State != SyncDisabled || snapshot.Backup.State != BackupDisabled {
		t.Fatalf("unexpected disabled placeholder states: %+v", snapshot)
	}
}

func loadConfig(t *testing.T, overrides map[string]string) config.Config {
	t.Helper()
	values := map[string]string{
		config.EnvGitRemoteURL:         "git@github.com:example/vault.git",
		config.EnvOIDCConfigurationURL: "https://oidc.example.test/.well-known/openid-configuration",
		config.EnvOIDCClientID:         "brain-client",
		config.EnvOIDCClientSecret:     "client-secret",
		config.EnvOIDCAdminGroup:       "access-brain-admin",
		config.EnvOIDCUserGroup:        "access-brain-user",
		config.EnvOIDCViewerGroup:      "access-brain-viewer",
	}
	for key, value := range overrides {
		values[key] = value
	}
	loaded, err := config.Load(func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	})
	if err != nil {
		t.Fatalf("load test config: %v", err)
	}
	return loaded
}
