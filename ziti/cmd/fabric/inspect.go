package fabric

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/fatih/color"
	"github.com/openziti/ziti/controller/rest_client/inspect"
	"github.com/openziti/ziti/controller/rest_model"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/openziti/ziti/ziti/util"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	"io"
	"os"
	"strings"
)

// newListCmd creates a command object for the "controller list" command
func newInspectCmd(p common.OptionsProvider) *cobra.Command {
	action := newInspectAction(p)
	cmd := action.newCobraCmd()
	cmd.AddCommand(action.newInspectSubCmd(p, "stackdump", "gets stackdumps from the requested nodes"))
	cmd.AddCommand(action.newInspectSubCmd(p, "metrics", "gets current metrics from the requested nodes"))
	cmd.AddCommand(action.newInspectSubCmd(p, "config", "gets configuration from the requested nodes"))
	cmd.AddCommand(action.newInspectSubCmd(p, "cluster-config", "gets a subset of cluster configuration from the requested nodes"))
	cmd.AddCommand(action.newInspectSubCmd(p, "connected-routers", "gets information about which routers are connected to which controllers"))
	cmd.AddCommand(action.newInspectSubCmd(p, "links", "gets information from routers about their view of links"))

	inspectCircuitsAction := &InspectCircuitsAction{InspectAction: *newInspectAction(p)}
	cmd.AddCommand(inspectCircuitsAction.newCobraCmd())

	return cmd
}

func newInspectAction(p common.OptionsProvider) *InspectAction {
	return &InspectAction{Options: api.Options{CommonOptions: p()}}
}

type InspectAction struct {
	api.Options
	toFiles bool
	format  string
}

func (self *InspectAction) addFlags(cmd *cobra.Command) *cobra.Command {
	self.AddCommonFlags(cmd)
	cmd.Flags().BoolVarP(&self.toFiles, "file", "f", false, "Output results to a file per result, with the format <instanceId>.<ValueName>")
	cmd.Flags().StringVar(&self.format, "format", "yaml", "Output format. One of yaml|json")
	return cmd
}

func (self *InspectAction) newCobraCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Inspect runtime <application> <values>",
		RunE:  self.run,
		Args:  cobra.MinimumNArgs(2),
	}
	return self.addFlags(cmd)
}

func (self *InspectAction) newInspectSubCmd(p common.OptionsProvider, value string, desc string) *cobra.Command {
	inspectAction := newInspectAction(p)

	cmd := &cobra.Command{
		Use:   value + " [optional node id regex]",
		Short: desc,
		RunE: func(cmd *cobra.Command, args []string) error {
			appRegex := ".*"
			if len(args) > 0 {
				appRegex = args[0]
			}
			return inspectAction.inspect(appRegex, value)
		},
		Args: cobra.RangeArgs(0, 1),
	}
	return inspectAction.addFlags(cmd)
}

func (self *InspectAction) run(_ *cobra.Command, args []string) error {
	return self.inspect(args[0], args[1:]...)
}

func (self *InspectAction) inspect(appRegex string, requestValues ...string) error {
	client, err := util.NewFabricManagementClient(self)
	if err != nil {
		return err
	}

	inspectOk, err := client.Inspect.Inspect(&inspect.InspectParams{
		Request: &rest_model.InspectRequest{
			AppRegex:        &appRegex,
			RequestedValues: requestValues,
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
		for idx, value := range result.Values {
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
				if idx > 0 {
					fmt.Println()
				}
				fmt.Print(color.New(color.FgGreen, color.Bold).Sprintf("%v.%v\n", appId, name))
				out = os.Stdout
			}
			if err = self.prettyPrint(out, value.Value, 0); err != nil {
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

func (self *InspectAction) prettyPrint(o io.Writer, val interface{}, indent uint) error {
	if strVal, ok := val.(string); ok {
		if strings.IndexByte(strVal, '\n') > 0 {
			lines := strings.Split(strVal, "\n")
			if _, err := fmt.Fprintln(o, lines[0]); err != nil {
				return err
			}
			for _, line := range lines[1:] {
				for i := uint(0); i < indent; i++ {
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
		return nil
	}

	if self.format == "yaml" {
		return yaml.NewEncoder(o).Encode(val)
	}

	if self.format == "json" {
		enc := json.NewEncoder(o)
		enc.SetIndent("", "    ")
		return enc.Encode(val)
	}
	return errors.Errorf("unsupported format %v", self.format)
}
