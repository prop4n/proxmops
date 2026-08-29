package cli

import (
	"log/slog"
	"os"

	"github.com/prop4n/proxmops/internal/config"
	"github.com/spf13/cobra"
)

type rootState struct {
	configPath string
	cfg        config.Config
	log        *slog.Logger
}

func Execute() int {
	if err := newRootCmd().Execute(); err != nil {
		// Cobra has already printed the error.
		return 1
	}
	return 0
}

func newRootCmd() *cobra.Command {
	st := &rootState{log: slog.New(slog.NewTextHandler(os.Stdout, nil))}

	cmd := &cobra.Command{
		Use:           "proxmops",
		Short:         "Declarative GitOps for Proxmox VE",
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	cmd.PersistentFlags().StringVar(&st.configPath, "config", "proxmops.yaml", "path to the configuration file")

	cmd.AddCommand(
		newDaemonCmd(st),
		newPlanCmd(st),
		newInitCmd(),
		newVersionCmd(),
	)
	return cmd
}

func (st *rootState) loadConfig() error {
	cfg, err := config.Load(st.configPath)
	if err != nil {
		return err
	}
	st.cfg = cfg
	return nil
}

// loadConfigDraft reads the config without requiring cluster and source
// settings, allowing the daemon to start unconfigured and be set up from the
// web UI.
func (st *rootState) loadConfigDraft() error {
	cfg, err := config.LoadDraft(st.configPath)
	if err != nil {
		return err
	}
	st.cfg = cfg
	return nil
}
