// Package config loads and validates braind's immutable process configuration.
package config

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	EnvListenAddr              = "BRAIND_LISTEN_ADDR"
	EnvExternalURL             = "BRAIND_EXTERNAL_URL"
	EnvDataPath                = "BRAIND_DATA_PATH"
	EnvVaultPath               = "BRAIND_VAULT_PATH"
	EnvReadOnly                = "BRAIND_READ_ONLY"
	EnvSyncEnabled             = "BRAIND_SYNC_ENABLED"
	EnvOBPath                  = "BRAIND_OB_PATH"
	EnvMaxNoteBytes            = "BRAIND_MAX_NOTE_BYTES"
	EnvSearchMaxFiles          = "BRAIND_SEARCH_MAX_FILES"
	EnvSearchMaxBytes          = "BRAIND_SEARCH_MAX_BYTES"
	EnvSearchTimeout           = "BRAIND_SEARCH_TIMEOUT"
	EnvSearchMaxResults        = "BRAIND_SEARCH_MAX_RESULTS"
	EnvGitBackupEnabled        = "BRAIND_GIT_BACKUP_ENABLED"
	EnvGitDir                  = "BRAIND_GIT_DIR"
	EnvGitRemoteURL            = "BRAIND_GIT_REMOTE_URL"
	EnvGitBranch               = "BRAIND_GIT_BRANCH"
	EnvGitBackupInterval       = "BRAIND_GIT_BACKUP_INTERVAL"
	EnvGitQuietPeriod          = "BRAIND_GIT_QUIET_PERIOD"
	EnvGitSSHKeyPath           = "BRAIND_GIT_SSH_KEY_PATH"
	EnvGitKnownHostsPath       = "BRAIND_GIT_KNOWN_HOSTS_PATH"
	EnvLogLevel                = "BRAIND_LOG_LEVEL"
	EnvOIDCEnabled             = "OIDC_ENABLED"
	EnvOIDCConfigurationURL    = "OIDC_CONFIGURATION_URL"
	EnvOIDCClientID            = "OIDC_CLIENT_ID"
	EnvOIDCClientSecret        = "OIDC_CLIENT_SECRET"
	EnvOIDCGroupsClaim         = "OIDC_GROUPS_CLAIM"
	EnvOIDCGroupsSource        = "OIDC_GROUPS_SOURCE"
	EnvOIDCAdminGroup          = "OIDC_ADMIN_GROUP"
	EnvOIDCUserGroup           = "OIDC_USER_GROUP"
	EnvOIDCViewerGroup         = "OIDC_VIEWER_GROUP"
	EnvOIDCUserClaim           = "OIDC_USER_CLAIM"
	EnvOIDCProviderName        = "OIDC_PROVIDER_NAME"
	EnvOIDCAutoRedirect        = "OIDC_AUTO_REDIRECT"
	EnvOIDCExtraScopes         = "OIDC_EXTRA_SCOPES"
	EnvOIDCAPIBearerEnabled    = "OIDC_API_BEARER_ENABLED"
	EnvOIDCAPIAudience         = "OIDC_API_AUDIENCE"
	EnvSessionIdleTimeout      = "BRAIND_SESSION_IDLE_TIMEOUT"
	EnvSessionAbsoluteLifetime = "BRAIND_SESSION_ABSOLUTE_LIFETIME"
	EnvOIDCReauthInterval      = "OIDC_REAUTH_INTERVAL"
	EnvTrustedProxyCIDRs       = "BRAIND_TRUSTED_PROXY_CIDRS"
)

const redacted = "[redacted]"

// LookupEnv matches os.LookupEnv and allows deterministic configuration tests.
type LookupEnv func(string) (string, bool)

// Config is immutable after loading. Accessors return values or defensive
// copies, so callers cannot alter the process configuration.
type Config struct {
	server   Server
	vault    Vault
	sync     Sync
	search   Search
	backup   Backup
	oidc     OIDC
	session  Session
	logLevel string
}

type Server struct {
	ListenAddr        string
	ExternalURL       string
	TrustedProxyCIDRs []netip.Prefix
}

type Vault struct {
	DataPath     string
	VaultPath    string
	ReadOnly     bool
	MaxNoteBytes int64
}

type Sync struct {
	Enabled bool
	OBPath  string
}

type Search struct {
	MaxFiles   int
	MaxBytes   int64
	Timeout    time.Duration
	MaxResults int
}

type Backup struct {
	Enabled        bool
	GitDir         string
	RemoteURL      string
	Branch         string
	Interval       time.Duration
	QuietPeriod    time.Duration
	SSHKeyPath     string
	KnownHostsPath string
}

type OIDC struct {
	Enabled          bool
	ConfigurationURL string
	ClientID         string
	ClientSecret     string
	GroupsClaim      string
	GroupsSource     string
	AdminGroup       string
	UserGroup        string
	ViewerGroup      string
	UserClaim        string
	ProviderName     string
	AutoRedirect     bool
	ExtraScopes      []string
	APIBearerEnabled bool
	APIAudience      string
}

type Session struct {
	IdleTimeout      time.Duration
	AbsoluteLifetime time.Duration
	ReauthInterval   time.Duration
}

func (c Config) Server() Server {
	result := c.server
	result.TrustedProxyCIDRs = append([]netip.Prefix(nil), c.server.TrustedProxyCIDRs...)
	return result
}

func (c Config) Vault() Vault     { return c.vault }
func (c Config) Sync() Sync       { return c.sync }
func (c Config) Search() Search   { return c.search }
func (c Config) Backup() Backup   { return c.backup }
func (c Config) Session() Session { return c.session }
func (c Config) LogLevel() string { return c.logLevel }

func (c Config) OIDC() OIDC {
	result := c.oidc
	result.ExtraScopes = append([]string(nil), c.oidc.ExtraScopes...)
	return result
}

// Load reads, parses, and validates all documented environment settings.
func Load(lookup LookupEnv) (Config, error) {
	if lookup == nil {
		return Config{}, errors.New("environment lookup is required")
	}

	var errs []error
	read := func(name, fallback string) string {
		if value, ok := lookup(name); ok {
			return strings.TrimSpace(value)
		}
		return fallback
	}
	readBool := func(name string, fallback bool) bool {
		raw := read(name, strconv.FormatBool(fallback))
		value, err := strconv.ParseBool(raw)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: must be a boolean", name))
			return fallback
		}
		return value
	}
	readPositiveInt := func(name string, fallback int) int {
		raw := read(name, strconv.Itoa(fallback))
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			errs = append(errs, fmt.Errorf("%s: must be a positive integer", name))
			return fallback
		}
		return value
	}
	readPositiveDuration := func(name string, fallback time.Duration) time.Duration {
		raw := read(name, fallback.String())
		value, err := time.ParseDuration(raw)
		if err != nil || value <= 0 {
			errs = append(errs, fmt.Errorf("%s: must be a positive duration", name))
			return fallback
		}
		return value
	}
	readPositiveBytes := func(name, fallback string) int64 {
		raw := read(name, fallback)
		value, err := parseByteSize(raw)
		if err != nil || value <= 0 {
			errs = append(errs, fmt.Errorf("%s: must be a positive byte size", name))
			fallbackValue, _ := parseByteSize(fallback)
			return fallbackValue
		}
		return value
	}

	config := Config{
		server: Server{
			ListenAddr:  read(EnvListenAddr, "0.0.0.0:8080"),
			ExternalURL: read(EnvExternalURL, "https://brain.lab.johnrowley.co"),
		},
		vault: Vault{
			DataPath:     read(EnvDataPath, "/data"),
			VaultPath:    read(EnvVaultPath, "/data/vault"),
			ReadOnly:     readBool(EnvReadOnly, false),
			MaxNoteBytes: readPositiveBytes(EnvMaxNoteBytes, "2MiB"),
		},
		sync: Sync{
			Enabled: readBool(EnvSyncEnabled, true),
			OBPath:  read(EnvOBPath, "ob"),
		},
		search: Search{
			MaxFiles:   readPositiveInt(EnvSearchMaxFiles, 10_000),
			MaxBytes:   readPositiveBytes(EnvSearchMaxBytes, "256MiB"),
			Timeout:    readPositiveDuration(EnvSearchTimeout, 10*time.Second),
			MaxResults: readPositiveInt(EnvSearchMaxResults, 100),
		},
		backup: Backup{
			Enabled:        readBool(EnvGitBackupEnabled, true),
			GitDir:         read(EnvGitDir, "/data/git/vault.git"),
			RemoteURL:      read(EnvGitRemoteURL, ""),
			Branch:         read(EnvGitBranch, "main"),
			Interval:       readPositiveDuration(EnvGitBackupInterval, 6*time.Hour),
			QuietPeriod:    readPositiveDuration(EnvGitQuietPeriod, 30*time.Second),
			SSHKeyPath:     read(EnvGitSSHKeyPath, "/run/secrets/git/ssh-privatekey"),
			KnownHostsPath: read(EnvGitKnownHostsPath, "/run/secrets/git/known_hosts"),
		},
		oidc: OIDC{
			Enabled:          readBool(EnvOIDCEnabled, true),
			ConfigurationURL: read(EnvOIDCConfigurationURL, ""),
			ClientID:         read(EnvOIDCClientID, ""),
			ClientSecret:     read(EnvOIDCClientSecret, ""),
			GroupsClaim:      read(EnvOIDCGroupsClaim, "groups"),
			GroupsSource:     read(EnvOIDCGroupsSource, "id_token"),
			AdminGroup:       read(EnvOIDCAdminGroup, ""),
			UserGroup:        read(EnvOIDCUserGroup, ""),
			ViewerGroup:      read(EnvOIDCViewerGroup, ""),
			UserClaim:        read(EnvOIDCUserClaim, "email"),
			ProviderName:     read(EnvOIDCProviderName, ""),
			AutoRedirect:     readBool(EnvOIDCAutoRedirect, false),
			ExtraScopes:      uniqueFields(read(EnvOIDCExtraScopes, "groups")),
			APIBearerEnabled: readBool(EnvOIDCAPIBearerEnabled, false),
			APIAudience:      read(EnvOIDCAPIAudience, ""),
		},
		session: Session{
			IdleTimeout:      readPositiveDuration(EnvSessionIdleTimeout, time.Hour),
			AbsoluteLifetime: readPositiveDuration(EnvSessionAbsoluteLifetime, 8*time.Hour),
			ReauthInterval:   readPositiveDuration(EnvOIDCReauthInterval, time.Hour),
		},
		logLevel: read(EnvLogLevel, "info"),
	}

	config.server.TrustedProxyCIDRs = parseCIDRs(read(EnvTrustedProxyCIDRs, ""), &errs)
	validate(&config, &errs)
	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}
	return config, nil
}

func validate(config *Config, errs *[]error) {
	if err := validateListenAddress(config.server.ListenAddr); err != nil {
		*errs = append(*errs, fmt.Errorf("%s: %w", EnvListenAddr, err))
	}
	if err := validateHTTPSURL(config.server.ExternalURL); err != nil {
		*errs = append(*errs, fmt.Errorf("%s: %w", EnvExternalURL, err))
	}
	if !filepath.IsAbs(config.vault.DataPath) {
		*errs = append(*errs, fmt.Errorf("%s: must be an absolute path", EnvDataPath))
	}
	if !isPathWithin(config.vault.DataPath, config.vault.VaultPath) {
		*errs = append(*errs, fmt.Errorf("%s: must be an absolute path beneath %s", EnvVaultPath, EnvDataPath))
	}
	if strings.TrimSpace(config.sync.OBPath) == "" {
		*errs = append(*errs, fmt.Errorf("%s: must not be empty", EnvOBPath))
	}
	if !filepath.IsAbs(config.backup.GitDir) || !isPathWithin(config.vault.DataPath, config.backup.GitDir) {
		*errs = append(*errs, fmt.Errorf("%s: must be an absolute path beneath %s", EnvGitDir, EnvDataPath))
	}
	if isPathWithin(config.vault.VaultPath, config.backup.GitDir) {
		*errs = append(*errs, fmt.Errorf("%s: must be outside %s", EnvGitDir, EnvVaultPath))
	}
	if config.backup.Enabled {
		require(EnvGitRemoteURL, config.backup.RemoteURL, errs)
		require(EnvGitBranch, config.backup.Branch, errs)
		require(EnvGitSSHKeyPath, config.backup.SSHKeyPath, errs)
		require(EnvGitKnownHostsPath, config.backup.KnownHostsPath, errs)
		if hasURLCredentials(config.backup.RemoteURL) {
			*errs = append(*errs, fmt.Errorf("%s: credentials must not be embedded in the remote URL", EnvGitRemoteURL))
		}
	}
	if strings.ContainsAny(config.backup.RemoteURL, "\r\n") {
		*errs = append(*errs, fmt.Errorf("%s: must not contain line breaks", EnvGitRemoteURL))
	}
	if strings.ContainsAny(config.backup.Branch, " \t\r\n") {
		*errs = append(*errs, fmt.Errorf("%s: must not contain whitespace", EnvGitBranch))
	}

	switch config.logLevel {
	case "debug", "info", "warn", "error":
	default:
		*errs = append(*errs, fmt.Errorf("%s: must be one of debug, info, warn, error", EnvLogLevel))
	}

	switch config.oidc.GroupsSource {
	case "id_token", "userinfo":
	default:
		*errs = append(*errs, fmt.Errorf("%s: must be id_token or userinfo", EnvOIDCGroupsSource))
	}
	if config.oidc.Enabled {
		require(EnvOIDCConfigurationURL, config.oidc.ConfigurationURL, errs)
		require(EnvOIDCClientID, config.oidc.ClientID, errs)
		require(EnvOIDCClientSecret, config.oidc.ClientSecret, errs)
		require(EnvOIDCAdminGroup, config.oidc.AdminGroup, errs)
		require(EnvOIDCUserGroup, config.oidc.UserGroup, errs)
		require(EnvOIDCViewerGroup, config.oidc.ViewerGroup, errs)
		if config.oidc.ConfigurationURL != "" {
			if err := validateHTTPSURL(config.oidc.ConfigurationURL); err != nil {
				*errs = append(*errs, fmt.Errorf("%s: %w", EnvOIDCConfigurationURL, err))
			}
		}
		groups := []string{config.oidc.AdminGroup, config.oidc.UserGroup, config.oidc.ViewerGroup}
		if allNonEmpty(groups) && uniqueCount(groups) != len(groups) {
			*errs = append(*errs, errors.New("OIDC role groups must be distinct"))
		}
		if config.oidc.ProviderName == "" && config.oidc.ConfigurationURL != "" {
			parsed, err := url.Parse(config.oidc.ConfigurationURL)
			if err == nil {
				config.oidc.ProviderName = parsed.Hostname()
			}
		}
	}
	if config.oidc.APIBearerEnabled && config.oidc.APIAudience == "" {
		*errs = append(*errs, fmt.Errorf("%s: required when %s is true", EnvOIDCAPIAudience, EnvOIDCAPIBearerEnabled))
	}
	if config.session.IdleTimeout > config.session.AbsoluteLifetime {
		*errs = append(*errs, fmt.Errorf("%s: must not exceed %s", EnvSessionIdleTimeout, EnvSessionAbsoluteLifetime))
	}
	if config.session.ReauthInterval > config.session.AbsoluteLifetime {
		*errs = append(*errs, fmt.Errorf("%s: must not exceed %s", EnvOIDCReauthInterval, EnvSessionAbsoluteLifetime))
	}
}

func parseByteSize(raw string) (int64, error) {
	units := []struct {
		suffix     string
		multiplier int64
	}{
		{suffix: "GiB", multiplier: 1024 * 1024 * 1024},
		{suffix: "MiB", multiplier: 1024 * 1024},
		{suffix: "KiB", multiplier: 1024},
		{suffix: "B", multiplier: 1},
	}
	multiplier := int64(1)
	number := raw
	for _, unit := range units {
		if strings.HasSuffix(raw, unit.suffix) {
			multiplier = unit.multiplier
			number = strings.TrimSuffix(raw, unit.suffix)
			break
		}
	}
	value, err := strconv.ParseInt(number, 10, 64)
	if err != nil || value <= 0 || value > (1<<63-1)/multiplier {
		return 0, errors.New("invalid byte size")
	}
	return value * multiplier, nil
}

func parseCIDRs(raw string, errs *[]error) []netip.Prefix {
	if raw == "" {
		return nil
	}
	seen := make(map[netip.Prefix]struct{})
	result := make([]netip.Prefix, 0)
	for _, item := range strings.Split(raw, ",") {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(item))
		if err != nil {
			*errs = append(*errs, fmt.Errorf("%s: must contain only comma-separated CIDRs", EnvTrustedProxyCIDRs))
			return nil
		}
		prefix = prefix.Masked()
		if _, ok := seen[prefix]; ok {
			continue
		}
		seen[prefix] = struct{}{}
		result = append(result, prefix)
	}
	return result
}

func validateListenAddress(address string) error {
	_, rawPort, err := net.SplitHostPort(address)
	if err != nil {
		return errors.New("must be a host:port address")
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil || port < 0 || port > 65535 {
		return errors.New("port must be between 0 and 65535")
	}
	return nil
}

func validateHTTPSURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return errors.New("must be an absolute HTTPS URL")
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return errors.New("must not contain credentials or a fragment")
	}
	return nil
}

func hasURLCredentials(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && parsed.User != nil && parsed.User.String() != "git"
}

func isPathWithin(parent, child string) bool {
	if !filepath.IsAbs(parent) || !filepath.IsAbs(child) {
		return false
	}
	relative, err := filepath.Rel(filepath.Clean(parent), filepath.Clean(child))
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func require(name, value string, errs *[]error) {
	if strings.TrimSpace(value) == "" {
		*errs = append(*errs, fmt.Errorf("%s: required", name))
	}
}

func uniqueFields(raw string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0)
	for _, item := range strings.Fields(raw) {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func uniqueCount(values []string) int {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	return len(seen)
}

func allNonEmpty(values []string) bool {
	for _, value := range values {
		if value == "" {
			return false
		}
	}
	return true
}

// Diagnostics returns a fresh map containing effective configuration with
// secrets and the Git remote redacted.
func (c Config) Diagnostics() map[string]string {
	result := map[string]string{
		EnvListenAddr:              c.server.ListenAddr,
		EnvExternalURL:             c.server.ExternalURL,
		EnvDataPath:                c.vault.DataPath,
		EnvVaultPath:               c.vault.VaultPath,
		EnvReadOnly:                strconv.FormatBool(c.vault.ReadOnly),
		EnvSyncEnabled:             strconv.FormatBool(c.sync.Enabled),
		EnvOBPath:                  c.sync.OBPath,
		EnvMaxNoteBytes:            strconv.FormatInt(c.vault.MaxNoteBytes, 10),
		EnvSearchMaxFiles:          strconv.Itoa(c.search.MaxFiles),
		EnvSearchMaxBytes:          strconv.FormatInt(c.search.MaxBytes, 10),
		EnvSearchTimeout:           c.search.Timeout.String(),
		EnvSearchMaxResults:        strconv.Itoa(c.search.MaxResults),
		EnvGitBackupEnabled:        strconv.FormatBool(c.backup.Enabled),
		EnvGitDir:                  c.backup.GitDir,
		EnvGitRemoteURL:            configuredOrEmpty(c.backup.RemoteURL),
		EnvGitBranch:               c.backup.Branch,
		EnvGitBackupInterval:       c.backup.Interval.String(),
		EnvGitQuietPeriod:          c.backup.QuietPeriod.String(),
		EnvGitSSHKeyPath:           c.backup.SSHKeyPath,
		EnvGitKnownHostsPath:       c.backup.KnownHostsPath,
		EnvLogLevel:                c.logLevel,
		EnvOIDCEnabled:             strconv.FormatBool(c.oidc.Enabled),
		EnvOIDCConfigurationURL:    c.oidc.ConfigurationURL,
		EnvOIDCClientID:            c.oidc.ClientID,
		EnvOIDCClientSecret:        configuredOrEmptySecret(c.oidc.ClientSecret),
		EnvOIDCGroupsClaim:         c.oidc.GroupsClaim,
		EnvOIDCGroupsSource:        c.oidc.GroupsSource,
		EnvOIDCAdminGroup:          c.oidc.AdminGroup,
		EnvOIDCUserGroup:           c.oidc.UserGroup,
		EnvOIDCViewerGroup:         c.oidc.ViewerGroup,
		EnvOIDCUserClaim:           c.oidc.UserClaim,
		EnvOIDCProviderName:        c.oidc.ProviderName,
		EnvOIDCAutoRedirect:        strconv.FormatBool(c.oidc.AutoRedirect),
		EnvOIDCExtraScopes:         strings.Join(c.oidc.ExtraScopes, " "),
		EnvOIDCAPIBearerEnabled:    strconv.FormatBool(c.oidc.APIBearerEnabled),
		EnvOIDCAPIAudience:         c.oidc.APIAudience,
		EnvSessionIdleTimeout:      c.session.IdleTimeout.String(),
		EnvSessionAbsoluteLifetime: c.session.AbsoluteLifetime.String(),
		EnvOIDCReauthInterval:      c.session.ReauthInterval.String(),
		EnvTrustedProxyCIDRs:       joinPrefixes(c.server.TrustedProxyCIDRs),
	}
	return result
}

func configuredOrEmpty(value string) string {
	if value == "" {
		return ""
	}
	return "[configured]"
}

func configuredOrEmptySecret(value string) string {
	if value == "" {
		return ""
	}
	return redacted
}

func joinPrefixes(prefixes []netip.Prefix) string {
	values := make([]string, len(prefixes))
	for index, prefix := range prefixes {
		values[index] = prefix.String()
	}
	return strings.Join(values, ",")
}

func (c Config) String() string {
	diagnostics := c.Diagnostics()
	keys := make([]string, 0, len(diagnostics))
	for key := range diagnostics {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var builder strings.Builder
	for index, key := range keys {
		if index > 0 {
			builder.WriteByte(' ')
		}
		fmt.Fprintf(&builder, "%s=%q", key, diagnostics[key])
	}
	return builder.String()
}

// Format prevents fmt's alternate struct formatting from revealing private
// secret fields. Every formatting verb receives the redacted String form.
func (c Config) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, c.String())
}
