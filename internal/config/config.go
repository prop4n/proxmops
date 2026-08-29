package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// EnvPrefix is prepended to environment variables bound to config keys.
const EnvPrefix = "PROXMOPS"

// Config is the top-level runtime configuration.
type Config struct {
	Cluster   Cluster   `mapstructure:"cluster"`
	Source    Source    `mapstructure:"source"`
	Reconcile Reconcile `mapstructure:"reconcile"`
}

// Cluster describes how to reach the Proxmox API.
type Cluster struct {
	Endpoint           string `mapstructure:"endpoint"`
	TokenID            string `mapstructure:"tokenId"`
	TokenSecret        string `mapstructure:"tokenSecret"`
	InsecureSkipVerify bool   `mapstructure:"insecureSkipVerify"`
}

// Source describes where the desired state lives.
type Source struct {
	RepoURL  string `mapstructure:"repoURL"`
	Path     string `mapstructure:"path"`
	Revision string `mapstructure:"revision"`
}

// Reconcile controls reconciliation behaviour.
type Reconcile struct {
	Interval time.Duration `mapstructure:"interval"`
	AutoSync bool          `mapstructure:"autoSync"`
	Prune    bool          `mapstructure:"prune"`
	DryRun   bool          `mapstructure:"dryRun"`
}

// Load reads the config file at path, overlays PROXMOPS_ environment variables,
// and validates the result.
func Load(path string) (Config, error) {
	v := viper.New()
	setDefaults(v)

	v.SetConfigFile(path)
	v.SetEnvPrefix(EnvPrefix)
	// Map nested keys such as cluster.tokenSecret to PROXMOPS_CLUSTER_TOKENSECRET.
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	return unmarshal(v)
}

// setDefaults registers the values used when a key is absent from every layer.
func setDefaults(v *viper.Viper) {
	v.SetDefault("source.revision", "main")
	v.SetDefault("reconcile.interval", time.Minute)
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
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// envBoundKeys are the config keys that may be supplied through the environment.
var envBoundKeys = []string{
	"cluster.endpoint",
	"cluster.tokenId",
	"cluster.tokenSecret",
	"cluster.insecureSkipVerify",
	"source.repoURL",
	"source.path",
	"source.revision",
	"reconcile.interval",
	"reconcile.autoSync",
	"reconcile.prune",
	"reconcile.dryRun",
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
