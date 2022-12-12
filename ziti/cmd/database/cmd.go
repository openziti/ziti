package database

import (
	"fmt"
	"github.com/openziti/ziti-db-explorer/cmd/ziti-db-explorer/zdecli"
	"github.com/openziti/ziti/ziti/util"
	"github.com/spf13/cobra"
	"io"
)

func NewCmdDb(_ io.Writer, errOut io.Writer) *cobra.Command {
	cmd := util.NewEmptyParentCmd("db", "Interact with Ziti database files")

	exploreCmd := &cobra.Command{
		Use:   "explore <ctrl.db>|help|version",
		Short: "Interactive CLI to explore Ziti database files",
		Run: func(cmd *cobra.Command, args []string) {
			if err := zdecli.Run("ziti db explore", args[0]); err != nil {
				_, _ = errOut.Write([]byte(fmt.Sprintf("Error: %s", err)))
			}
		},
	}

	cmd.AddCommand(exploreCmd)
	return cmd
}
