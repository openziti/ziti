package fabric

import (
	"github.com/openziti/ziti/v2/ziti/cmd/common"
	"github.com/spf13/cobra"
)

func newDbCmd(p common.OptionsProvider) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Database management operations for the Ziti Edge Controller",
	}

	cmd.AddCommand(NewDbSnapshotCmd(p))
	cmd.AddCommand(NewDbCheckIntegrityCmd(p))
	cmd.AddCommand(NewDbCheckIntegrityStatusCmd(p))

	return cmd
}
