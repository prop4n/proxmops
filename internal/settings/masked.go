package settings

import "time"

// Masked is the settings view exchanged with the web UI. Secret values never
// round-trip: the GET response leaves them empty and reports whether they are
// set; on save, an empty secret keeps the stored value.
type Masked struct {
	Configured bool              `json:"configured"`
	Cluster    MaskedCluster     `json:"cluster"`
	Source     MaskedSource      `json:"source"`
	Reconcile  ReconcileSettings `json:"reconcile"`
	UpdatedAt  *time.Time        `json:"updatedAt"`
}

// MaskedCluster is the cluster section as seen by the UI.
type MaskedCluster struct {
	Endpoint           string `json:"endpoint"`
	TokenID            string `json:"tokenId"`
	TokenSecret        string `json:"tokenSecret"`
	TokenSecretSet     bool   `json:"tokenSecretSet"`
	InsecureSkipVerify bool   `json:"insecureSkipVerify"`
}

// MaskedSource is the source section as seen by the UI.
type MaskedSource struct {
	RepoURL  string `json:"repoURL"`
	Path     string `json:"path"`
	Revision string `json:"revision"`
	Username string `json:"username"`
	Token    string `json:"token"`
	TokenSet bool   `json:"tokenSet"`
}

// NotConfigured returns the masked view of an unconfigured daemon.
func NotConfigured() Masked {
	return Masked{}
}

// Masked projects the settings onto the UI shape.
func (st Settings) Masked() Masked {
	updatedAt := st.UpdatedAt
	return Masked{
		Configured: true,
		Cluster: MaskedCluster{
			Endpoint:           st.Cluster.Endpoint,
			TokenID:            st.Cluster.TokenID,
			TokenSecretSet:     st.Cluster.TokenSecret != "",
			InsecureSkipVerify: st.Cluster.InsecureSkipVerify,
		},
		Source: MaskedSource{
			RepoURL:  st.Source.RepoURL,
			Path:     st.Source.Path,
			Revision: st.Source.Revision,
			Username: st.Source.Username,
			TokenSet: st.Source.Token != "",
		},
		Reconcile: st.Reconcile,
		UpdatedAt: &updatedAt,
	}
}

// Settings turns a masked update into settings, keeping the secrets of current
// when the update leaves them empty.
func (m Masked) Settings(current Settings) Settings {
	st := Settings{
		Cluster: ClusterSettings{
			Endpoint:           m.Cluster.Endpoint,
			TokenID:            m.Cluster.TokenID,
			TokenSecret:        m.Cluster.TokenSecret,
			InsecureSkipVerify: m.Cluster.InsecureSkipVerify,
		},
		Source: SourceSettings{
			RepoURL:  m.Source.RepoURL,
			Path:     m.Source.Path,
			Revision: m.Source.Revision,
			Username: m.Source.Username,
			Token:    m.Source.Token,
		},
		Reconcile: m.Reconcile,
		UpdatedAt: time.Now(),
	}
	if st.Cluster.TokenSecret == "" {
		st.Cluster.TokenSecret = current.Cluster.TokenSecret
	}
	if st.Source.Token == "" {
		st.Source.Token = current.Source.Token
	}
	return st
}
