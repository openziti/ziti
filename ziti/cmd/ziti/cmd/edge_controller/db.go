package edge_controller

import (
	cmdutil "github.com/openziti/ziti/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/spf13/cobra"
	"io"
)

// newListCmd creates a command object for the "controller list" command
func newDbCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Database management operations for the Ziti Edge Controller",
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			cmdhelper.CheckErr(err)
		},
	}

	cmd.AddCommand(newDbSnapshotCmd(f, out, errOut))
	cmd.AddCommand(newDbCheckIntegrityCmd(f, out, errOut))

	return cmd
}
