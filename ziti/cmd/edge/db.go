package edge

import (
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/spf13/cobra"
	"io"
)

func newDbCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Database management operations for the Ziti Edge Controller",
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			cmdhelper.CheckErr(err)
		},
	}

	cmd.AddCommand(newDbSnapshotCmd(out, errOut))
	cmd.AddCommand(newDbCheckIntegrityCmd(out, errOut))
	cmd.AddCommand(newDbCheckIntegrityStatusCmd(out, errOut))

	return cmd
}
