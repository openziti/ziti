package edge

import (
	"fmt"
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/actions/host"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/zitilab"
	"github.com/pkg/errors"
)

func InitController(componentSpec string) model.Action {
	return &edgeInit{
		componentSpec: componentSpec,
	}
}

func (init *edgeInit) Execute(run model.Run) error {
	return component.Exec(init.componentSpec, zitilab.ControllerActionInitStandalone).Execute(run)
}

type edgeInit struct {
	componentSpec string
}

func InitRaftController(componentSpec string) model.Action {
	return &raftInit{
		componentSpec: componentSpec,
	}
}

func (init *raftInit) Execute(run model.Run) error {
	m := run.GetModel()
	username := m.MustStringVariable("credentials.edge.username")
	password := m.MustStringVariable("credentials.edge.password")

	if username == "" {
		return errors.New("variable credentials/edge/username must be a string")
	}

	if password == "" {
		return errors.New("variable credentials/edge/password must be a string")
	}

	for _, c := range m.SelectComponents(init.componentSpec) {
		sshConfigFactory := c.GetHost().NewSshConfigFactory()

		tmpl := "set -o pipefail; /home/%s/fablab/bin/ziti agent controller init %s %s default.admin 2>&1 | tee logs/controller.edge.init.log"
		if err := host.Exec(c.GetHost(), fmt.Sprintf(tmpl, sshConfigFactory.User(), username, password)).Execute(run); err != nil {
			return err
		}
	}

	return nil
}

type raftInit struct {
	componentSpec string
}
