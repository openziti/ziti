package fabric

import (
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/spf13/cobra"
)

func newDbCmd(p common.OptionsProvider) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Database management operations for the Ziti Edge Controller",
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			cmdhelper.CheckErr(err)
		},
	}

	cmd.AddCommand(newDbSnapshotCmd(p))
	cmd.AddCommand(newDbCheckIntegrityCmd(p))
	cmd.AddCommand(newDbCheckIntegrityStatusCmd(p))

	return cmd
}
