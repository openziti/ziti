package fabric

import (
	"context"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/openziti/ziti/controller/rest_client/cluster"
	"github.com/openziti/ziti/controller/rest_model"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/util"
	"github.com/spf13/cobra"
)

func NewClusterCmd(p common.OptionsProvider) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cluster",
		Short: "Controller cluster operations",
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			cmdhelper.CheckErr(err)
		},
	}

	cmd.AddCommand(newClusterListMembersCmd(p))
	cmd.AddCommand(newClusterAddMemberCmd(p))
	cmd.AddCommand(newClusterRemoveMemberCmd(p))
	cmd.AddCommand(newClusterTransferLeadershipCmd(p))

	return cmd
}

func newClusterListMembersCmd(p common.OptionsProvider) *cobra.Command {
	action := &clusterListMembersAction{
		Options: api.Options{CommonOptions: p()},
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "list cluster members and their status",
		Args:  cobra.ExactArgs(0),
		RunE:  action.run,
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	action.AddCommonFlags(cmd)

	return cmd
}

type clusterListMembersAction struct {
	api.Options
}

func (self *clusterListMembersAction) run(cmd *cobra.Command, _ []string) error {
	self.Cmd = cmd
	client, err := util.NewFabricManagementClient(self)
	if err != nil {
		return err
	}
	members, err := client.Cluster.ClusterListMembers(&cluster.ClusterListMembersParams{
		Context: context.Background(),
	})
	if err != nil {
		return err
	}

	t := table.NewWriter()
	t.SetStyle(table.StyleRounded)
	t.AppendHeader(table.Row{"Id", "Address", "Voter", "Leader", "Version", "Connected", "ReadOnly"})
	for _, m := range members.Payload.Data {
		t.AppendRow(table.Row{*m.ID, *m.Address, *m.Voter, *m.Leader, *m.Version, *m.Connected, m.ReadOnly != nil && *m.ReadOnly})
	}
	api.RenderTable(&api.Options{
		CommonOptions: self.CommonOptions,
	}, t, nil)
	return nil
}

func newClusterAddMemberCmd(p common.OptionsProvider) *cobra.Command {
	action := &clusterAddMemberAction{
		Options: api.Options{CommonOptions: p()},
	}

	cmd := &cobra.Command{
		Use:   "add <address>",
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

type clusterAddMemberAction struct {
	api.Options
	nonVoting bool
}

func (self *clusterAddMemberAction) run(cmd *cobra.Command, args []string) error {
	self.Cmd = cmd
	client, err := util.NewFabricManagementClient(self)
	if err != nil {
		return err
	}

	isVoter := !self.nonVoting

	_, err = client.Cluster.ClusterMemberAdd(&cluster.ClusterMemberAddParams{
		Context: context.Background(),
		Member: &rest_model.ClusterMemberAdd{
			Address: &args[0],
			IsVoter: &isVoter,
		},
	})

	return err
}

func newClusterRemoveMemberCmd(p common.OptionsProvider) *cobra.Command {
	action := &clusterRemoveMemberAction{
		Options: api.Options{CommonOptions: p()},
	}

	cmd := &cobra.Command{
		Use:   "remove <cluster member id>",
		Short: "remove cluster member",
		Args:  cobra.ExactArgs(1),
		RunE:  action.run,
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	action.AddCommonFlags(cmd)

	return cmd
}

type clusterRemoveMemberAction struct {
	api.Options
}

func (self *clusterRemoveMemberAction) run(cmd *cobra.Command, args []string) error {
	self.Cmd = cmd

	client, err := util.NewFabricManagementClient(self)
	if err != nil {
		return err
	}

	_, err = client.Cluster.ClusterMemberRemove(&cluster.ClusterMemberRemoveParams{
		Context: context.Background(),
		Member: &rest_model.ClusterMemberRemove{
			ID: &args[0],
		},
	})

	return err
}

func newClusterTransferLeadershipCmd(p common.OptionsProvider) *cobra.Command {
	action := &clusterTransferLeadershipAction{
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

type clusterTransferLeadershipAction struct {
	api.Options
}

func (self *clusterTransferLeadershipAction) run(cmd *cobra.Command, args []string) error {
	self.Cmd = cmd

	client, err := util.NewFabricManagementClient(self)
	if err != nil {
		return err
	}

	newLeader := ""

	if len(args) > 0 {
		newLeader = args[0]
	}

	_, err = client.Cluster.ClusterTransferLeadership(&cluster.ClusterTransferLeadershipParams{
		Context: context.Background(),
		Member: &rest_model.ClusterTransferLeadership{
			NewLeaderID: newLeader,
		},
	})

	return err
}
