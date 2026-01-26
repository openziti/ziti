package edge

import (
	"io"

	"github.com/spf13/cobra"
)

func newDbCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Database management operations for the Ziti Edge Controller",
	}

	cmd.AddCommand(newDbSnapshotCmd(out, errOut))
	cmd.AddCommand(newDbCheckIntegrityCmd(out, errOut))
	cmd.AddCommand(newDbCheckIntegrityStatusCmd(out, errOut))

	return cmd
}
