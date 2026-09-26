package config

import (
	"fmt"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestLoadDefaultsWithRequiredValues(t *testing.T) {
	config, err := Load(mapLookup(validEnvironment()))
	if err != nil {
		t.Fatalf("load configuration: %v", err)
	}

	server := config.Server()
	if server.ListenAddr != "0.0.0.0:8080" {
		t.Fatalf("unexpected listen address: %q", server.ListenAddr)
	}
	if server.ExternalURL != "https://brain.lab.johnrowley.co" {
		t.Fatalf("unexpected external URL: %q", server.ExternalURL)
	}
	if len(server.TrustedProxyCIDRs) != 0 {
		t.Fatalf("unexpected trusted proxy CIDRs: %v", server.TrustedProxyCIDRs)
	}
	if got := config.Vault(); got.DataPath != "/data" || got.VaultPath != "/data/vault" || got.ReadOnly || got.MaxNoteBytes != 2*1024*1024 {
		t.Fatalf("unexpected vault defaults: %+v", got)
	}
	if got := config.Sync(); !got.Enabled || got.OBPath != "ob" {
		t.Fatalf("unexpected sync defaults: %+v", got)
	}
	if got := config.Search(); got.MaxFiles != 10_000 || got.MaxBytes != 256*1024*1024 || got.Timeout != 10*time.Second || got.MaxResults != 100 {
		t.Fatalf("unexpected search defaults: %+v", got)
	}
	if got := config.Backup(); !got.Enabled || got.GitDir != "/data/git/vault.git" || got.Branch != "main" || got.Interval != 6*time.Hour || got.QuietPeriod != 30*time.Second {
		t.Fatalf("unexpected backup defaults: %+v", got)
	}
	if got := config.OIDC(); !got.Enabled || got.GroupsClaim != "groups" || got.GroupsSource != "id_token" || got.UserClaim != "email" || got.ProviderName != "oidc.lab.johnrowley.co" {
		t.Fatalf("unexpected OIDC defaults: %+v", got)
	}
	if got := config.Session(); got.IdleTimeout != time.Hour || got.AbsoluteLifetime != 8*time.Hour || got.ReauthInterval != time.Hour {
		t.Fatalf("unexpected session defaults: %+v", got)
	}
	if config.LogLevel() != "info" {
		t.Fatalf("unexpected log level: %q", config.LogLevel())
	}
}

func TestLoadOverridesAndReturnsDefensiveCopies(t *testing.T) {
	environment := validEnvironment()
	for key, value := range map[string]string{
		EnvListenAddr:              "127.0.0.1:9090",
		EnvExternalURL:             "https://brain.example.test",
		EnvDataPath:                "/srv/brain",
		EnvVaultPath:               "/srv/brain/notes",
		EnvReadOnly:                "true",
		EnvSyncEnabled:             "false",
		EnvOBPath:                  "/usr/local/bin/ob",
		EnvMaxNoteBytes:            "3MiB",
		EnvSearchMaxFiles:          "2000",
		EnvSearchMaxBytes:          "64MiB",
		EnvSearchTimeout:           "3s",
		EnvSearchMaxResults:        "25",
		EnvGitBackupEnabled:        "false",
		EnvGitDir:                  "/srv/brain/git/vault.git",
		EnvGitRemoteURL:            "",
		EnvGitBranch:               "backup",
		EnvGitBackupInterval:       "12h",
		EnvGitQuietPeriod:          "45s",
		EnvGitSSHKeyPath:           "/secrets/key",
		EnvGitKnownHostsPath:       "/secrets/hosts",
		EnvLogLevel:                "debug",
		EnvOIDCEnabled:             "false",
		EnvOIDCConfigurationURL:    "",
		EnvOIDCClientID:            "",
		EnvOIDCClientSecret:        "",
		EnvOIDCAdminGroup:          "",
		EnvOIDCUserGroup:           "",
		EnvOIDCViewerGroup:         "",
		EnvOIDCGroupsClaim:         "roles",
		EnvOIDCGroupsSource:        "userinfo",
		EnvOIDCUserClaim:           "preferred_username",
		EnvOIDCProviderName:        "Test ID",
		EnvOIDCAutoRedirect:        "true",
		EnvOIDCExtraScopes:         "groups groups custom",
		EnvOIDCAPIBearerEnabled:    "true",
		EnvOIDCAPIAudience:         "https://brain.example.test/api",
		EnvSessionIdleTimeout:      "30m",
		EnvSessionAbsoluteLifetime: "4h",
		EnvOIDCReauthInterval:      "2h",
		EnvTrustedProxyCIDRs:       "10.0.0.7/8, 2001:db8::1/32,10.0.0.0/8",
	} {
		environment[key] = value
	}

	config, err := Load(mapLookup(environment))
	if err != nil {
		t.Fatalf("load configuration: %v", err)
	}
	if got := config.Vault().MaxNoteBytes; got != 3*1024*1024 {
		t.Fatalf("unexpected note byte limit: %d", got)
	}
	if got := config.Search(); got.MaxFiles != 2000 || got.MaxBytes != 64*1024*1024 || got.Timeout != 3*time.Second || got.MaxResults != 25 {
		t.Fatalf("unexpected search overrides: %+v", got)
	}
	if got := config.OIDC().ExtraScopes; len(got) != 2 || got[0] != "groups" || got[1] != "custom" {
		t.Fatalf("unexpected extra scopes: %v", got)
	}
	wantPrefixes := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("2001:db8::/32")}
	server := config.Server()
	if fmt.Sprint(server.TrustedProxyCIDRs) != fmt.Sprint(wantPrefixes) {
		t.Fatalf("unexpected trusted proxies: %v", server.TrustedProxyCIDRs)
	}
	server.TrustedProxyCIDRs[0] = netip.MustParsePrefix("192.0.2.0/24")
	if got := config.Server().TrustedProxyCIDRs[0]; got != wantPrefixes[0] {
		t.Fatalf("server accessor did not return a defensive copy: %v", got)
	}
	oidc := config.OIDC()
	oidc.ExtraScopes[0] = "changed"
	if got := config.OIDC().ExtraScopes[0]; got != "groups" {
		t.Fatalf("OIDC accessor did not return a defensive copy: %q", got)
	}
}

func TestLoadReportsRequiredSettings(t *testing.T) {
	_, err := Load(mapLookup(map[string]string{}))
	if err == nil {
		t.Fatal("expected missing required settings to fail")
	}
	message := err.Error()
	for _, name := range []string{
		EnvGitRemoteURL,
		EnvOIDCConfigurationURL,
		EnvOIDCClientID,
		EnvOIDCClientSecret,
		EnvOIDCAdminGroup,
		EnvOIDCUserGroup,
		EnvOIDCViewerGroup,
	} {
		if !strings.Contains(message, name+": required") {
			t.Errorf("error does not identify %s: %v", name, err)
		}
	}
}

func TestLoadRejectsMalformedSettings(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "listen address", key: EnvListenAddr, value: "localhost"},
		{name: "listen port", key: EnvListenAddr, value: "localhost:65536"},
		{name: "external URL", key: EnvExternalURL, value: "http://brain.example.test"},
		{name: "boolean", key: EnvReadOnly, value: "sometimes"},
		{name: "byte size", key: EnvMaxNoteBytes, value: "2MB"},
		{name: "positive integer", key: EnvSearchMaxFiles, value: "0"},
		{name: "duration", key: EnvSearchTimeout, value: "soon"},
		{name: "CIDR", key: EnvTrustedProxyCIDRs, value: "10.0.0.1"},
		{name: "log level", key: EnvLogLevel, value: "trace"},
		{name: "groups source", key: EnvOIDCGroupsSource, value: "both"},
		{name: "OIDC URL", key: EnvOIDCConfigurationURL, value: "http://oidc.example.test/.well-known/openid-configuration"},
		{name: "Git branch", key: EnvGitBranch, value: "bad branch"},
		{name: "Git remote line break", key: EnvGitRemoteURL, value: "git@example.test:owner/repo.git\nother"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			environment := validEnvironment()
			environment[test.key] = test.value
			_, err := Load(mapLookup(environment))
			if err == nil {
				t.Fatalf("expected %s to be rejected", test.key)
			}
			if !strings.Contains(err.Error(), test.key) {
				t.Fatalf("error does not identify %s: %v", test.key, err)
			}
		})
	}
}

func TestLoadRejectsIncompatibleSettings(t *testing.T) {
	tests := []struct {
		name    string
		changes map[string]string
		want    string
	}{
		{
			name:    "relative data path",
			changes: map[string]string{EnvDataPath: "data"},
			want:    EnvDataPath,
		},
		{
			name:    "vault outside data",
			changes: map[string]string{EnvVaultPath: "/other/vault"},
			want:    EnvVaultPath,
		},
		{
			name:    "Git metadata in vault",
			changes: map[string]string{EnvGitDir: "/data/vault/.git"},
			want:    EnvGitDir,
		},
		{
			name:    "duplicate role groups",
			changes: map[string]string{EnvOIDCUserGroup: "access-brain-admin"},
			want:    "OIDC role groups must be distinct",
		},
		{
			name:    "bearer without audience",
			changes: map[string]string{EnvOIDCAPIBearerEnabled: "true"},
			want:    EnvOIDCAPIAudience,
		},
		{
			name:    "idle exceeds absolute lifetime",
			changes: map[string]string{EnvSessionIdleTimeout: "9h"},
			want:    EnvSessionIdleTimeout,
		},
		{
			name:    "reauth exceeds absolute lifetime",
			changes: map[string]string{EnvOIDCReauthInterval: "9h"},
			want:    EnvOIDCReauthInterval,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			environment := validEnvironment()
			for key, value := range test.changes {
				environment[key] = value
			}
			_, err := Load(mapLookup(environment))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected error containing %q, got %v", test.want, err)
			}
		})
	}
}

func TestLoadAllowsDisabledSubsystemsWithoutCredentials(t *testing.T) {
	environment := map[string]string{
		EnvOIDCEnabled:      "false",
		EnvGitBackupEnabled: "false",
	}
	if _, err := Load(mapLookup(environment)); err != nil {
		t.Fatalf("load disabled subsystems: %v", err)
	}
}

func TestDiagnosticsAndFormattingRedactSecrets(t *testing.T) {
	environment := validEnvironment()
	environment[EnvOIDCClientSecret] = "unique-oidc-secret"
	environment[EnvGitRemoteURL] = "git@example.test:private/repository.git"
	config, err := Load(mapLookup(environment))
	if err != nil {
		t.Fatalf("load configuration: %v", err)
	}

	diagnostics := config.Diagnostics()
	if got := diagnostics[EnvOIDCClientSecret]; got != redacted {
		t.Fatalf("client secret was not redacted: %q", got)
	}
	if got := diagnostics[EnvGitRemoteURL]; got != "[configured]" {
		t.Fatalf("Git remote was not redacted: %q", got)
	}
	for name, formatted := range map[string]string{
		"String": config.String(),
		"v":      fmt.Sprintf("%v", config),
		"plus-v": fmt.Sprintf("%+v", config),
		"hash-v": fmt.Sprintf("%#v", config),
	} {
		if strings.Contains(formatted, environment[EnvOIDCClientSecret]) || strings.Contains(formatted, environment[EnvGitRemoteURL]) {
			t.Fatalf("%s formatting disclosed a secret: %s", name, formatted)
		}
	}

	diagnostics[EnvListenAddr] = "changed"
	if got := config.Diagnostics()[EnvListenAddr]; got == "changed" {
		t.Fatal("Diagnostics did not return a fresh map")
	}
}

func TestErrorsDoNotIncludeSecretValues(t *testing.T) {
	environment := validEnvironment()
	environment[EnvOIDCClientSecret] = "secret-that-must-not-appear"
	environment[EnvGitRemoteURL] = "https://user:embedded-password@example.test/repo.git"

	_, err := Load(mapLookup(environment))
	if err == nil {
		t.Fatal("expected embedded credentials to fail")
	}
	if strings.Contains(err.Error(), "secret-that-must-not-appear") || strings.Contains(err.Error(), "embedded-password") {
		t.Fatalf("validation error disclosed a secret: %v", err)
	}
}

func TestLoadRequiresLookup(t *testing.T) {
	if _, err := Load(nil); err == nil {
		t.Fatal("expected a nil lookup to fail")
	}
}

func validEnvironment() map[string]string {
	return map[string]string{
		EnvGitRemoteURL:         "git@github.com:robbert229/obsidian-vault-backup.git",
		EnvOIDCConfigurationURL: "https://oidc.lab.johnrowley.co/.well-known/openid-configuration",
		EnvOIDCClientID:         "brain-client",
		EnvOIDCClientSecret:     "client-secret",
		EnvOIDCAdminGroup:       "access-brain-admin",
		EnvOIDCUserGroup:        "access-brain-user",
		EnvOIDCViewerGroup:      "access-brain-viewer",
	}
}

func mapLookup(values map[string]string) LookupEnv {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}
