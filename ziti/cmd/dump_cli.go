package cmd

import (
	"github.com/spf13/cobra"
)

func NewDumpCliCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "dump-cli",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			current := cmd
			for current.HasParent() {
				current = current.Parent()
			}
			return DumpCli(current)
		},
	}
	return cmd
}

func DumpCli(cmd *cobra.Command) error {
	if err := cmd.Help(); err != nil {
		return err
	}

	for _, childCmd := range cmd.Commands() {
		if err := DumpCli(childCmd); err != nil {
			return err
		}
	}
	return nil
}
