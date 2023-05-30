package edge

import (
	"fmt"
	"github.com/openziti/fablab/kernel/lib"
	"github.com/openziti/fablab/kernel/lib/actions/host"
	"github.com/openziti/fablab/kernel/model"
	"github.com/pkg/errors"
)

func RaftJoin(componentSpec string) model.Action {
	return &raftJoin{
		componentSpec: componentSpec,
	}
}

type raftJoin struct {
	componentSpec string
}

func (self *raftJoin) Execute(m *model.Model) error {
	ctrls := m.SelectComponents(self.componentSpec)
	if len(ctrls) < 1 {
		return errors.Errorf("no controllers found with spec '%v'", self.componentSpec)
	}
	primary := ctrls[0]
	sshConfigFactory := lib.NewSshConfigFactory(primary.GetHost())
	for _, c := range ctrls[1:] {
		tmpl := "/home/%s/fablab/bin/ziti agent cluster add %v --id %v"
		if err := host.Exec(primary.GetHost(), fmt.Sprintf(tmpl, sshConfigFactory.User(), "tls:"+c.Host.PublicIp+":6262", c.PublicIdentity)).Execute(m); err != nil {
			return err
		}
	}

	return nil
}
