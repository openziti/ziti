package edge

import (
	"github.com/openziti/fablab/kernel/lib"
	"github.com/openziti/fablab/kernel/model"
	zitilib_actions "github.com/openziti/ziti/zititest/zitilab/actions"
	"github.com/openziti/ziti/zititest/zitilab/cli"
	"path/filepath"
	"strings"
)

func InitIdentities(componentSpec string, concurrency int) model.Action {
	return &initIdentitiesAction{
		componentSpec: componentSpec,
		concurrency:   concurrency,
	}
}

func (action *initIdentitiesAction) Execute(m *model.Model) error {
	return m.ForEachComponent(action.componentSpec, action.concurrency, func(c *model.Component) error {
		if err := zitilib_actions.EdgeExec(m, "delete", "identity", c.PublicIdentity); err != nil {
			return err
		}

		return action.createAndEnrollIdentity(c)
	})
}

func (action *initIdentitiesAction) createAndEnrollIdentity(c *model.Component) error {
	ssh := lib.NewSshConfigFactory(c.GetHost())

	jwtFileName := filepath.Join(model.ConfigBuild(), c.PublicIdentity+".jwt")

	err := zitilib_actions.EdgeExec(c.GetModel(), "create", "identity", "service", c.PublicIdentity,
		"--jwt-output-file", jwtFileName,
		"-a", strings.Join(c.Tags, ","))

	if err != nil {
		return err
	}

	configFileName := filepath.Join(model.ConfigBuild(), c.PublicIdentity+".json")

	_, err = cli.Exec(c.GetModel(), "edge", "enroll", "--jwt", jwtFileName, "--out", configFileName)

	if err != nil {
		return err
	}

	remoteConfigFile := "/home/ubuntu/fablab/cfg/" + c.PublicIdentity + ".json"
	return lib.SendFile(ssh, configFileName, remoteConfigFile)
}

type initIdentitiesAction struct {
	componentSpec string
	concurrency   int
}
