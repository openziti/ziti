package fabric

import (
	"context"
	"fmt"
	"github.com/openziti/fabric/rest_client/inspect"
	"github.com/openziti/fabric/rest_model"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/openziti/ziti/ziti/util"
	"github.com/spf13/cobra"
	"io"
	"os"
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
	toFiles bool
}

func (self *InspectCmd) newCobraCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Inspect runtime <application> <values>",
		RunE:  self.run,
		Args:  cobra.MinimumNArgs(2),
	}
	self.AddCommonFlags(cmd)
	cmd.Flags().BoolVarP(&self.toFiles, "file", "f", false, "Output results to a file per result, with the format <instanceId>.<ValueName>")
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
		fmt.Printf("Results: (%d)\n", len(result.Values))
		for _, value := range result.Values {
			appId := stringz.OrEmpty(value.AppID)
			name := stringz.OrEmpty(value.Name)
			var out io.Writer
			var file *os.File
			if self.toFiles {
				fmt.Printf("output result to: %v.%v\n", appId, name)
				file, err = os.Create(fmt.Sprintf("%v.%v", appId, name))
				if err != nil {
					return err
				}
				out = file
			} else {
				fmt.Printf("%v.%v\n", appId, name)
				out = os.Stdout
			}
			if err = self.prettyPrintOutput(out, value.Value, 0); err != nil {
				if closeErr := file.Close(); closeErr != nil {
					return errorz.MultipleErrors{err, closeErr}
				}
				return err
			}
			if file != nil {
				if err = file.Close(); err != nil {
					return err
				}
			}
		}
	} else {
		fmt.Printf("\nEncountered errors: (%d)\n", len(result.Errors))
		for _, err := range result.Errors {
			fmt.Printf("\t%v\n", err)
		}
	}

	return nil
}

func (self *InspectCmd) prettyPrintOutput(o io.Writer, val interface{}, indent int) error {
	if mapVal, ok := val.(map[string]interface{}); ok {
		if _, err := fmt.Fprintf(o, "\n"); err != nil {
			return err
		}
		var sortedKeys []string
		for k := range mapVal {
			sortedKeys = append(sortedKeys, k)
		}
		sort.Strings(sortedKeys)
		for _, k := range sortedKeys {
			for i := 0; i < indent; i++ {
				if _, err := fmt.Fprintf(o, " "); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintf(o, "%v: ", k); err != nil {
				return err
			}
			if err := self.prettyPrintOutput(o, mapVal[k], indent+4); err != nil {
				return err
			}
		}
	} else if sliceVal, ok := val.([]interface{}); ok {
		if _, err := fmt.Fprintf(o, "\n"); err != nil {
			return err
		}
		for _, v := range sliceVal {
			for i := 0; i < indent; i++ {
				if _, err := fmt.Fprintf(o, " "); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintf(o, "    - "); err != nil {
				return err
			}
			if err := self.prettyPrintOutput(o, v, indent+6); err != nil {
				return err
			}
		}
	} else if strVal, ok := val.(string); ok {
		if strings.IndexByte(strVal, '\n') > 0 {
			lines := strings.Split(strVal, "\n")
			if _, err := fmt.Fprintln(o, lines[0]); err != nil {
				return err
			}
			for _, line := range lines[1:] {
				for i := 0; i < indent; i++ {
					if _, err := fmt.Fprintf(o, " "); err != nil {
						return err
					}
				}
				if _, err := fmt.Fprintln(o, line); err != nil {
					return err
				}
			}
		} else {
			if _, err := fmt.Fprintf(o, "%v\n", val); err != nil {
				return err
			}
		}
	} else {
		if _, err := fmt.Fprintf(o, "%v\n", val); err != nil {
			return err
		}
	}
	return nil
}
