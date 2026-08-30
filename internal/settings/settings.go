// Package settings persists the daemon configuration edited from the web UI:
// the cluster connection, the desired-state source, and the reconcile policy.
// Settings live in a single encrypted-at-rest row of the application database
// and take precedence over the file configuration once saved, so changes apply
// without restarting the daemon.
package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/prop4n/proxmops/internal/config"
	"github.com/prop4n/proxmops/internal/crypt"
	"github.com/prop4n/proxmops/internal/store"
)

// ErrNotConfigured is returned when no settings have been saved yet.
var ErrNotConfigured = errors.New("settings: not configured")

// Repository is the persistence the service needs. LoadSettings returns
// store.ErrNotFound when nothing has been saved yet.
type Repository interface {
	LoadSettings(ctx context.Context) ([]byte, error)
	SaveSettings(ctx context.Context, data []byte) error
}

// Settings holds the user-editable daemon configuration. Secret fields are
// plain text in memory only; they are encrypted before hitting the database.
type Settings struct {
	Cluster   ClusterSettings   `json:"cluster"`
	Source    SourceSettings    `json:"source"`
	Reconcile ReconcileSettings `json:"reconcile"`
	UpdatedAt time.Time         `json:"updatedAt"`
}

// ClusterSettings describes how to reach the Proxmox API.
type ClusterSettings struct {
	Endpoint           string `json:"endpoint"`
	TokenID            string `json:"tokenId"`
	TokenSecret        string `json:"tokenSecret,omitempty"`
	InsecureSkipVerify bool   `json:"insecureSkipVerify"`
}

// SourceSettings describes where the desired state lives.
type SourceSettings struct {
	RepoURL  string `json:"repoURL"`
	Path     string `json:"path"`
	Revision string `json:"revision"`
	Username string `json:"username"`
	Token    string `json:"token,omitempty"`
}

// ReconcileSettings controls reconciliation behaviour.
type ReconcileSettings struct {
	IntervalSeconds int  `json:"intervalSeconds"`
	AutoSync        bool `json:"autoSync"`
	Prune           bool `json:"prune"`
	DryRun          bool `json:"dryRun"`
	Concurrency     int  `json:"concurrency"`
}

// Duration converts the interval to a time.Duration, defaulting to one minute.
func (r ReconcileSettings) Duration() time.Duration {
	if r.IntervalSeconds <= 0 {
		return time.Minute
	}
	return time.Duration(r.IntervalSeconds) * time.Second
}

// blob is the on-disk shape of Settings with encrypted secret fields.
type blob struct {
	Cluster   blobCluster       `json:"cluster"`
	Source    blobSource        `json:"source"`
	Reconcile ReconcileSettings `json:"reconcile"`
	UpdatedAt time.Time         `json:"updatedAt"`
}

type blobCluster struct {
	Endpoint           string `json:"endpoint"`
	TokenID            string `json:"tokenId"`
	TokenSecretEnc     string `json:"tokenSecretEnc,omitempty"`
	InsecureSkipVerify bool   `json:"insecureSkipVerify"`
}

type blobSource struct {
	RepoURL  string `json:"repoURL"`
	Path     string `json:"path"`
	Revision string `json:"revision"`
	Username string `json:"username"`
	TokenEnc string `json:"tokenEnc,omitempty"`
}

// Service reads and writes settings, encrypting secrets on the way through.
type Service struct {
	repo Repository
	key  crypt.Key
}

// New returns a Service over the given repository.
func New(repo Repository, key crypt.Key) *Service {
	return &Service{repo: repo, key: key}
}

// Get returns the current settings, or ErrNotConfigured when none were saved.
func (s *Service) Get(ctx context.Context) (Settings, error) {
	raw, err := s.repo.LoadSettings(ctx)
	if errors.Is(err, store.ErrNotFound) {
		return Settings{}, ErrNotConfigured
	}
	if err != nil {
		return Settings{}, err
	}

	var b blob
	if err := json.Unmarshal(raw, &b); err != nil {
		return Settings{}, fmt.Errorf("settings: decode: %w", err)
	}

	st := Settings{
		Cluster: ClusterSettings{
			Endpoint:           b.Cluster.Endpoint,
			TokenID:            b.Cluster.TokenID,
			InsecureSkipVerify: b.Cluster.InsecureSkipVerify,
		},
		Source: SourceSettings{
			RepoURL:  b.Source.RepoURL,
			Path:     b.Source.Path,
			Revision: b.Source.Revision,
			Username: b.Source.Username,
		},
		Reconcile: b.Reconcile,
		UpdatedAt: b.UpdatedAt,
	}
	if st.Cluster.TokenSecret, err = s.key.Decrypt(b.Cluster.TokenSecretEnc); err != nil {
		return Settings{}, fmt.Errorf("settings: cluster token secret: %w", err)
	}
	if st.Source.Token, err = s.key.Decrypt(b.Source.TokenEnc); err != nil {
		return Settings{}, fmt.Errorf("settings: git token: %w", err)
	}
	return st, nil
}

// Save validates and stores the settings, encrypting the secrets.
func (s *Service) Save(ctx context.Context, st Settings) error {
	if err := st.Config().Validate(); err != nil {
		return fmt.Errorf("settings: %w", err)
	}

	clusterSecret, err := s.key.Encrypt(st.Cluster.TokenSecret)
	if err != nil {
		return err
	}
	gitToken, err := s.key.Encrypt(st.Source.Token)
	if err != nil {
		return err
	}

	raw, err := json.Marshal(blob{
		Cluster: blobCluster{
			Endpoint:           st.Cluster.Endpoint,
			TokenID:            st.Cluster.TokenID,
			TokenSecretEnc:     clusterSecret,
			InsecureSkipVerify: st.Cluster.InsecureSkipVerify,
		},
		Source: blobSource{
			RepoURL:  st.Source.RepoURL,
			Path:     st.Source.Path,
			Revision: st.Source.Revision,
			Username: st.Source.Username,
			TokenEnc: gitToken,
		},
		Reconcile: st.Reconcile,
		UpdatedAt: time.Now(),
	})
	if err != nil {
		return fmt.Errorf("settings: encode: %w", err)
	}
	return s.repo.SaveSettings(ctx, raw)
}

// Config converts the settings to the runtime configuration shape, filling in
// the same defaults as the file loader.
func (st Settings) Config() config.Config {
	revision := st.Source.Revision
	if revision == "" {
		revision = "main"
	}
	return config.Config{
		Cluster: config.Cluster{
			Endpoint:           st.Cluster.Endpoint,
			TokenID:            st.Cluster.TokenID,
			TokenSecret:        st.Cluster.TokenSecret,
			InsecureSkipVerify: st.Cluster.InsecureSkipVerify,
		},
		Source: config.Source{
			RepoURL:  st.Source.RepoURL,
			Path:     st.Source.Path,
			Revision: revision,
			Username: st.Source.Username,
			Token:    st.Source.Token,
		},
		Reconcile: config.Reconcile{
			Interval:    st.Reconcile.Duration(),
			AutoSync:    st.Reconcile.AutoSync,
			Prune:       st.Reconcile.Prune,
			DryRun:      st.Reconcile.DryRun,
			Concurrency: st.Reconcile.Concurrency,
		},
	}
}
