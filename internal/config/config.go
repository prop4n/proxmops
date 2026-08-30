package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// EnvPrefix is prepended to environment variables bound to config keys.
const EnvPrefix = "PROXMOPS"

// they take precedence over files and are never logged.
const (
	envClusterTokenSecret = "PROXMOPS_CLUSTER_TOKENSECRET"
	envGitToken           = "PROXMOPS_GIT_TOKEN"
)

// Config is the top-level runtime configuration.
type Config struct {
	Cluster   Cluster   `mapstructure:"cluster"`
	Source    Source    `mapstructure:"source"`
	Reconcile Reconcile `mapstructure:"reconcile"`
	Server    Server    `mapstructure:"server"`
}

// Server configures the web UI and API.
type Server struct {
	DatabasePath string `mapstructure:"databasePath"`
	// KeyPath is the file holding the symmetric key used to encrypt the
	// secrets stored in the database. Defaults to the database path with a
	// ".key" suffix.
	KeyPath string `mapstructure:"keyPath"`
	// CookieSecure marks session cookies Secure; enable it when serving behind
	// HTTPS. Default false so the UI works over plain HTTP in a homelab.
	CookieSecure bool `mapstructure:"cookieSecure"`
}

// Cluster describes how to reach the Proxmox API.
//
// The token secret should not be stored in the config file. Provide it through
// the PROXMOPS_CLUSTER_TOKENSECRET environment variable or TokenSecretFile; an
// inline TokenSecret is accepted but warned about.
type Cluster struct {
	Endpoint           string `mapstructure:"endpoint"`
	TokenID            string `mapstructure:"tokenId"`
	TokenSecret        string `mapstructure:"tokenSecret"`
	TokenSecretFile    string `mapstructure:"tokenSecretFile"`
	InsecureSkipVerify bool   `mapstructure:"insecureSkipVerify"`
}

// Source describes where the desired state lives.
//
// A remote repoURL (with a scheme, or git@host:...) is cloned over Git; any
// other value (a local path, or "local") is read from the local filesystem.
// The Git token, like the cluster secret, is never stored inline: it comes from
// PROXMOPS_GIT_TOKEN or TokenFile.
type Source struct {
	RepoURL   string `mapstructure:"repoURL"`
	Path      string `mapstructure:"path"`
	Revision  string `mapstructure:"revision"`
	Username  string `mapstructure:"username"`
	TokenFile string `mapstructure:"tokenFile"`
	CacheDir  string `mapstructure:"cacheDir"`
	// Token is the resolved Git credential; it is never read from the file.
	Token string `mapstructure:"-"`
}

// Reconcile controls reconciliation behaviour.
type Reconcile struct {
	Interval time.Duration `mapstructure:"interval"`
	AutoSync bool          `mapstructure:"autoSync"`
	Prune    bool          `mapstructure:"prune"`
	DryRun   bool          `mapstructure:"dryRun"`
	// Concurrency bounds how many actions apply in parallel.
	Concurrency int `mapstructure:"concurrency"`
}

// Load reads the config file at path, overlays PROXMOPS_ environment variables,
// resolves secrets, and validates the result.
func Load(path string) (Config, error) {
	return load(path, true)
}

// LoadDraft reads the config file without requiring cluster and source
// settings: the daemon may start unconfigured and be set up later from the web
// UI. A missing file is tolerated, yielding pure defaults plus environment.
func LoadDraft(path string) (Config, error) {
	return load(path, false)
}

func load(path string, validate bool) (Config, error) {
	v := viper.New()
	setDefaults(v)

	v.SetConfigFile(path)
	v.SetEnvPrefix(EnvPrefix)
	// Map nested keys such as cluster.tokenSecret to PROXMOPS_CLUSTER_TOKENSECRET.
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		// The daemon can run with defaults alone and be configured from the
		// web UI afterwards.
		if validate || !errors.Is(err, os.ErrNotExist) {
			return Config{}, fmt.Errorf("read config: %w", err)
		}
	}

	cfg, err := unmarshal(v)
	if err != nil {
		return Config{}, err
	}
	if cfg.Server.KeyPath == "" {
		cfg.Server.KeyPath = cfg.Server.DatabasePath + ".key"
	}
	if err := cfg.resolveSecrets(); err != nil {
		return Config{}, err
	}
	if validate {
		if err := cfg.Validate(); err != nil {
			return Config{}, err
		}
	}
	return cfg, nil
}

// resolveSecrets fills secret fields from their most secure available source and
// warns when a secret is stored inline in the config file.
func (c *Config) resolveSecrets() error {
	secret, fromInline, err := resolveSecret(envClusterTokenSecret, c.Cluster.TokenSecretFile, c.Cluster.TokenSecret)
	if err != nil {
		return err
	}
	if fromInline {
		slog.Warn("cluster.tokenSecret is stored in clear text in the config file; "+
			"prefer "+envClusterTokenSecret+" or cluster.tokenSecretFile",
			"key", "cluster.tokenSecret")
	}
	c.Cluster.TokenSecret = secret

	// Git token: env or file only, never inline.
	token, _, err := resolveSecret(envGitToken, c.Source.TokenFile, "")
	if err != nil {
		return err
	}
	c.Source.Token = token
	return nil
}

// setDefaults registers the values used when a key is absent from every layer.
func setDefaults(v *viper.Viper) {
	v.SetDefault("source.revision", "main")
	v.SetDefault("reconcile.interval", time.Minute)
	v.SetDefault("reconcile.concurrency", 4)
	v.SetDefault("server.databasePath", "proxmops.db")
}

// unmarshal decodes and validates the accumulated Viper state. Environment
// variables that back nested struct fields are bound explicitly, because
// AutomaticEnv alone does not surface them to Unmarshal.
func unmarshal(v *viper.Viper) (Config, error) {
	for _, key := range envBoundKeys {
		if err := v.BindEnv(key); err != nil {
			return Config{}, fmt.Errorf("bind env %s: %w", key, err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

// envBoundKeys are the config keys that may be supplied through the environment.
// The token secret is deliberately absent: it is resolved by resolveSecrets so
// it can come from a file and never leak through Viper's key handling.
var envBoundKeys = []string{
	"cluster.endpoint",
	"cluster.tokenId",
	"cluster.insecureSkipVerify",
	"source.repoURL",
	"source.path",
	"source.revision",
	"reconcile.interval",
	"reconcile.autoSync",
	"reconcile.prune",
	"reconcile.dryRun",
	"reconcile.concurrency",
	"server.databasePath",
	"server.keyPath",
	"server.cookieSecure",
}

// Validate reports whether the configuration is usable.
func (c Config) Validate() error {
	if c.Cluster.Endpoint == "" {
		return fmt.Errorf("cluster.endpoint is required")
	}
	if c.Cluster.TokenID == "" || c.Cluster.TokenSecret == "" {
		return fmt.Errorf("cluster.tokenId and cluster.tokenSecret are required")
	}
	if c.Source.RepoURL == "" {
		return fmt.Errorf("source.repoURL is required")
	}
	if c.Reconcile.Interval <= 0 {
		return fmt.Errorf("reconcile.interval must be positive")
	}
	return nil
}

// Complete reports whether the configuration holds everything needed to reach
// the cluster and the desired state, ignoring policy settings.
func (c Config) Complete() bool {
	return c.Cluster.Endpoint != "" &&
		c.Cluster.TokenID != "" &&
		c.Cluster.TokenSecret != "" &&
		c.Source.RepoURL != ""
}
