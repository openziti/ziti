package edge

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/zitilab"
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

func (self *raftJoin) Execute(run model.Run) error {
	ctrls := run.GetModel().SelectComponents(self.componentSpec)
	if len(ctrls) < 1 {
		return errors.Errorf("no controllers found with spec '%v'", self.componentSpec)
	}
	primary := ctrls[0]
	ctrlType, ok := primary.Type.(*zitilab.ControllerType)
	if !ok {
		return errors.Errorf("component %s is not a controller", primary.Id)
	}
	log := pfxlog.Logger().WithField("component", primary.Id)
	for _, c := range ctrls[1:] {
		tmpl := "%s agent cluster add %v --id %v"
		cmd := fmt.Sprintf(tmpl, ctrlType.GetBinaryPath(primary), "tls:"+c.Host.PublicIp+":6262", c.Id)
		log.Info(cmd)
		if err := primary.GetHost().ExecLogOnlyOnError(cmd); err != nil {
			return err
		}
	}

	return nil
}
