package fabric

import (
	"github.com/spf13/cobra"
)

type InspectCircuitsAction struct {
	InspectAction
	includeStacks bool
}

func (self *InspectCircuitsAction) addFlags(cmd *cobra.Command) *cobra.Command {
	self.InspectAction.addFlags(cmd)
	cmd.Flags().BoolVar(&self.includeStacks, "include-stacks", false, "Include stack information")
	return cmd
}

func (self *InspectCircuitsAction) newCobraCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "circuit",
		Short: "query routers to get diagnostic information for a circuit",
		RunE:  self.runInspectCircuit,
		Args:  cobra.RangeArgs(1, 2),
	}
	return self.addFlags(cmd)
}

func (self *InspectCircuitsAction) runInspectCircuit(_ *cobra.Command, args []string) error {
	requestedValue := "circuit:"
	if self.includeStacks {
		requestedValue = "circuitAndStacks:"
	}
	requestedValue += args[0]

	appRegex := ".*"
	if len(args) > 1 {
		appRegex = args[1]
	}
	return self.inspect(appRegex, requestedValue)
}
