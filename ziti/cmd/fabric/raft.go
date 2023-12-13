package fabric

import (
	"context"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/openziti/ziti/controller/rest_client/raft"
	"github.com/openziti/ziti/controller/rest_model"
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
	cmd.AddCommand(newRaftAddMemberCmd(p))
	cmd.AddCommand(newRaftRemoveMemberCmd(p))
	cmd.AddCommand(newRaftTransferLeadershipCmd(p))

	return cmd
}

func newRaftListMembersCmd(p common.OptionsProvider) *cobra.Command {
	action := &raftListMembersAction{
		Options: api.Options{CommonOptions: p()},
	}

	cmd := &cobra.Command{
		Use:   "list-members",
		Short: "list cluster members and their status",
		Args:  cobra.ExactArgs(0),
		RunE:  action.run,
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	action.AddCommonFlags(cmd)

	return cmd
}

type raftListMembersAction struct {
	api.Options
}

func (self *raftListMembersAction) run(cmd *cobra.Command, _ []string) error {
	self.Cmd = cmd
	client, err := util.NewFabricManagementClient(self)
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
		CommonOptions: self.CommonOptions,
	}, t, nil)
	return nil
}

func newRaftAddMemberCmd(p common.OptionsProvider) *cobra.Command {
	action := &raftAddMemberAction{
		Options: api.Options{CommonOptions: p()},
	}

	cmd := &cobra.Command{
		Use:   "add-member <address>",
		Short: "add cluster member",
		Args:  cobra.ExactArgs(1),
		RunE:  action.run,
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	action.AddCommonFlags(cmd)
	cmd.Flags().BoolVar(&action.nonVoting, "non-voting", false, "Allows adding a non-voting member to the cluster")

	return cmd
}

type raftAddMemberAction struct {
	api.Options
	nonVoting bool
}

func (self *raftAddMemberAction) run(cmd *cobra.Command, args []string) error {
	self.Cmd = cmd
	client, err := util.NewFabricManagementClient(self)
	if err != nil {
		return err
	}

	isVoter := !self.nonVoting

	_, err = client.Raft.RaftMemberAdd(&raft.RaftMemberAddParams{
		Context: context.Background(),
		Member: &rest_model.RaftMemberAdd{
			Address: &args[0],
			IsVoter: &isVoter,
		},
	})

	return err
}

func newRaftRemoveMemberCmd(p common.OptionsProvider) *cobra.Command {
	action := &raftRemoveMemberAction{
		Options: api.Options{CommonOptions: p()},
	}

	cmd := &cobra.Command{
		Use:   "remove-member <cluster member id>",
		Short: "remove cluster member",
		Args:  cobra.ExactArgs(1),
		RunE:  action.run,
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	action.AddCommonFlags(cmd)

	return cmd
}

type raftRemoveMemberAction struct {
	api.Options
}

func (self *raftRemoveMemberAction) run(cmd *cobra.Command, args []string) error {
	self.Cmd = cmd

	client, err := util.NewFabricManagementClient(self)
	if err != nil {
		return err
	}

	_, err = client.Raft.RaftMemberRemove(&raft.RaftMemberRemoveParams{
		Context: context.Background(),
		Member: &rest_model.RaftMemberRemove{
			ID: &args[0],
		},
	})

	return err
}

func newRaftTransferLeadershipCmd(p common.OptionsProvider) *cobra.Command {
	action := &raftTransferLeadershipAction{
		Options: api.Options{CommonOptions: p()},
	}

	cmd := &cobra.Command{
		Use:   "transfer-leadership [cluster member id]?",
		Short: "transfer cluster leadership to another member",
		Long:  "transfer cluster leadership to another member. If a node id is specified, leadership will be transferred to that node",
		Args:  cobra.RangeArgs(0, 1),
		RunE:  action.run,
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	action.AddCommonFlags(cmd)

	return cmd
}

type raftTransferLeadershipAction struct {
	api.Options
}

func (self *raftTransferLeadershipAction) run(cmd *cobra.Command, args []string) error {
	self.Cmd = cmd

	client, err := util.NewFabricManagementClient(self)
	if err != nil {
		return err
	}

	newLeader := ""

	if len(args) > 0 {
		newLeader = args[0]
	}

	_, err = client.Raft.RaftTranferLeadership(&raft.RaftTranferLeadershipParams{
		Context: context.Background(),
		Member: &rest_model.RaftTransferLeadership{
			NewLeaderID: newLeader,
		},
	})

	return err
}
