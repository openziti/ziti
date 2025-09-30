//go:build production

package cmd

import (
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/openziti/ziti/ziti/cmd/templates"
)

func DemoCommandGroup(p common.OptionsProvider) templates.CommandGroups {
	return nil
}
