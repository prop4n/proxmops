package cli

import (
	"fmt"

	"github.com/prop4n/proxmops/internal/app"
	"github.com/spf13/cobra"
)

func newPlanCmd(st *rootState) *cobra.Command {
	return &cobra.Command{
		Use:   "plan",
		Short: "Print the diff between the repository and the cluster",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := st.loadConfig(); err != nil {
				return err
			}
			plan, err := app.New(st.cfg, st.log).Plan(cmd.Context())
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if plan.Empty() {
				fmt.Fprintln(out, "in sync, no changes")
				return nil
			}
			for _, a := range plan.Actions {
				fmt.Fprintf(out, "%s\t%s\n", a, a.Reason)
			}
			return nil
		},
	}
}
