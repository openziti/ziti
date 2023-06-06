package edge

import (
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/foundation/v2/netz"
	"github.com/pkg/errors"
	"time"
)

func ControllerAvailable(componentSpec string, timeout time.Duration) model.Action {
	return &edgeAvailable{
		componentSpec: componentSpec,
		timeout:       timeout,
	}
}

func (self *edgeAvailable) Execute(run model.Run) error {
	for _, c := range run.GetModel().SelectComponents(self.componentSpec) {
		if err := netz.WaitForPortActive(c.Host.PublicIp+":1280", self.timeout); err != nil {
			return errors.Wrap(err, "controller didn't start in time")
		}
	}

	return nil
}

type edgeAvailable struct {
	componentSpec string
	timeout       time.Duration
}
