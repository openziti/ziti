package fabric

import (
	"context"
	"fmt"
	"github.com/openziti/fabric/rest_client/inspect"
	"github.com/openziti/fabric/rest_model"
	"github.com/openziti/foundation/util/stringz"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/common"
	"github.com/openziti/ziti/ziti/cmd/ziti/util"
	"github.com/spf13/cobra"
)

// newListCmd creates a command object for the "controller list" command
func newInspectCmd(p common.OptionsProvider) *cobra.Command {
	listCmd := &ListCmd{Options: api.Options{CommonOptions: p()}}
	return listCmd.newCobraCmd()
}

type ListCmd struct {
	api.Options
}

func (self *ListCmd) newCobraCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Inspect runtime application values",
		RunE:  self.run,
		Args:  cobra.MinimumNArgs(2),
	}
	self.AddCommonFlags(cmd)
	return cmd
}

func (self *ListCmd) run(cmd *cobra.Command, args []string) error {
	client, err := util.NewFabricManagementClient(self)
	if err != nil {
		return err
	}

	inspectOk, err := client.Inspect.Inspect(&inspect.InspectParams{
		Request: &rest_model.InspectRequest{
			AppRegex:        &args[0],
			RequestedValues: args[1:],
		},
		Context: context.Background(),
	})

	if err != nil {
		return err
	}

	result := inspectOk.Payload
	if *result.Success {
		fmt.Printf("\nResults: (%d)\n", len(result.Values))
		for _, value := range result.Values {
			fmt.Printf("%v.%v\n", stringz.OrEmpty(value.AppID), stringz.OrEmpty(value.Name))
			fmt.Printf("%v\n\n", stringz.OrEmpty(value.Value))
		}
	} else {
		fmt.Printf("\nEncountered errors: (%d)\n", len(result.Errors))
		for _, err := range result.Errors {
			fmt.Printf("\t%v\n", err)
		}
	}

	return nil
}
