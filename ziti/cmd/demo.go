//go:build !production

package cmd

import (
	"github.com/spf13/cobra"

	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/openziti/ziti/ziti/cmd/demo"
	"github.com/openziti/ziti/ziti/cmd/templates"
)

func DemoCommandGroup(p common.OptionsProvider) templates.CommandGroups {
	demoCmd := demo.NewDemoCmd(p)
	return templates.CommandGroups{
		{
			Message: "Learning Ziti",
			Commands: []*cobra.Command{
				demoCmd,
			},
		},
	}
}
