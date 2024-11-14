package edge

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/zitilab"
	"github.com/pkg/errors"
	"time"
)

func RaftJoin(primaryId string, componentSpec string) model.Action {
	return RaftJoinWithTimeout(primaryId, componentSpec, time.Minute)
}

func RaftJoinWithTimeout(primaryId string, componentSpec string, timeout time.Duration) model.Action {
	return &raftJoin{
		primaryId:     primaryId,
		componentSpec: componentSpec,
		timeout:       timeout,
	}
}

type raftJoin struct {
	primaryId     string
	componentSpec string
	timeout       time.Duration
}

func (self *raftJoin) Execute(run model.Run) error {
	primary, err := run.GetModel().SelectComponent(self.primaryId)
	if err != nil {
		return fmt.Errorf("could not find primary controller component with id '%s'", self.primaryId)
	}

	ctrls := run.GetModel().SelectComponents(self.componentSpec)
	if len(ctrls) < 1 {
		return errors.Errorf("no controllers found with spec '%v'", self.componentSpec)
	}
	ctrlType, ok := primary.Type.(*zitilab.ControllerType)
	if !ok {
		return errors.Errorf("component %s is not a controller", primary.Id)
	}
	log := pfxlog.Logger().WithField("component", primary.Id)
	start := time.Now()
	for _, c := range ctrls {
		if c.Id == primary.Id {
			continue
		}

		for {
			tmpl := "%s agent cluster add %v --id %v"
			cmd := fmt.Sprintf(tmpl, ctrlType.GetBinaryPath(primary), "tls:"+c.Host.PublicIp+":6262", c.Id)
			log.Info(cmd)
			if err = primary.GetHost().ExecLogOnlyOnError(cmd); err != nil {
				if time.Since(start) > self.timeout {
					return err
				}
				time.Sleep(2 * time.Second)
			} else {
				break
			}
		}
	}

	return nil
}
