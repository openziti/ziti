package fabric

import (
	"context"
	"fmt"
	"github.com/openziti/fabric/rest_client/inspect"
	"github.com/openziti/fabric/rest_model"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/common"
	"github.com/openziti/ziti/ziti/cmd/ziti/util"
	"github.com/spf13/cobra"
	"sort"
	"strings"
)

// newListCmd creates a command object for the "controller list" command
func newInspectCmd(p common.OptionsProvider) *cobra.Command {
	listCmd := &InspectCmd{Options: api.Options{CommonOptions: p()}}
	return listCmd.newCobraCmd()
}

type InspectCmd struct {
	api.Options
	prettyPrint bool
}

func (self *InspectCmd) newCobraCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Inspect runtime application values",
		RunE:  self.run,
		Args:  cobra.MinimumNArgs(2),
	}
	self.AddCommonFlags(cmd)
	return cmd
}

func (self *InspectCmd) run(_ *cobra.Command, args []string) error {
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

	if self.OutputResponseJson() {
		return nil
	}

	result := inspectOk.Payload
	if *result.Success {
		fmt.Printf("\nResults: (%d)\n", len(result.Values))
		for _, value := range result.Values {
			fmt.Printf("%v.%v\n", stringz.OrEmpty(value.AppID), stringz.OrEmpty(value.Name))
			self.prettyPrintOutput(value.Value, 0)
			fmt.Printf("\n")
		}
	} else {
		fmt.Printf("\nEncountered errors: (%d)\n", len(result.Errors))
		for _, err := range result.Errors {
			fmt.Printf("\t%v\n", err)
		}
	}

	return nil
}

func (self *InspectCmd) prettyPrintOutput(val interface{}, indent int) {
	if mapVal, ok := val.(map[string]interface{}); ok {
		fmt.Printf("\n")
		var sortedKeys []string
		for k := range mapVal {
			sortedKeys = append(sortedKeys, k)
		}
		sort.Strings(sortedKeys)
		for _, k := range sortedKeys {
			for i := 0; i < indent; i++ {
				fmt.Printf(" ")
			}
			fmt.Printf("%v: ", k)
			self.prettyPrintOutput(mapVal[k], indent+4)
		}
	} else if sliceVal, ok := val.([]interface{}); ok {
		fmt.Println()
		for _, v := range sliceVal {
			for i := 0; i < indent; i++ {
				fmt.Printf(" ")
			}
			fmt.Printf("    - ")
			self.prettyPrintOutput(v, indent+6)
		}
	} else if strVal, ok := val.(string); ok {
		if strings.IndexByte(strVal, '\n') > 0 {
			lines := strings.Split(strVal, "\n")
			fmt.Println(lines[0])
			for _, line := range lines[1:] {
				for i := 0; i < indent; i++ {
					fmt.Printf(" ")
				}
				fmt.Println(line)
			}
		} else {
			fmt.Printf("%v\n", val)
		}
	} else {
		fmt.Printf("%v\n", val)
	}
}
