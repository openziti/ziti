package fabric

import (
	"context"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/openziti/fabric/rest_client/raft"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/util"
	"github.com/spf13/cobra"
)

// newRaftCmd creates a command object for the "controller raft" command
func newRaftCmd(p common.OptionsProvider) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "raft",
		Short: "Raft operations",
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			cmdhelper.CheckErr(err)
		},
	}

	cmd.AddCommand(newRaftListMembersCmd(p))

	return cmd
}

type raftListMembersOptions struct {
	api.Options
}

func newRaftListMembersCmd(p common.OptionsProvider) *cobra.Command {
	options := &raftListMembersOptions{
		Options: api.Options{CommonOptions: p()},
	}

	cmd := &cobra.Command{
		Use:   "list-members",
		Short: "list cluster members and their status",
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := raftListMembers(options)
			cmdhelper.CheckErr(err)
		},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	options.AddCommonFlags(cmd)

	return cmd
}

func raftListMembers(o *raftListMembersOptions) error {
	client, err := util.NewFabricManagementClient(o)
	if err != nil {
		return err
	}
	members, err := client.Raft.RaftListMembers(&raft.RaftListMembersParams{
		Context: context.Background(),
	})
	if err != nil {
		return err
	}

	t := table.NewWriter()
	t.SetStyle(table.StyleRounded)
	t.AppendHeader(table.Row{"Id", "Address", "Voter", "Leader", "Version", "Connected"})
	for _, m := range members.Payload.Values {
		t.AppendRow(table.Row{*m.ID, *m.Address, *m.Voter, *m.Leader, *m.Version, *m.Connected})
	}
	api.RenderTable(&api.Options{
		CommonOptions: o.CommonOptions,
	}, t, nil)
	return nil
}
