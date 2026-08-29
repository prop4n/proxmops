package cli

import (
	"github.com/prop4n/proxmops/internal/app"
	"github.com/spf13/cobra"
)

func newDaemonCmd(st *rootState) *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run the reconciliation loop and web UI",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := st.loadConfigDraft(); err != nil {
				return err
			}
			return app.New(st.cfg, st.log).Run(cmd.Context(), addr)
		},
	}
	cmd.Flags().StringVar(&addr, "addr", ":8080", "address for the web UI and API")
	return cmd
}
