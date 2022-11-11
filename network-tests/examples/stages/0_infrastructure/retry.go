package runlevel_0_infrastructure

import (
	"fmt"

	"github.com/openziti/fablab/kernel/model"
	"github.com/sirupsen/logrus"
)

type retry struct {
	stage   model.InfrastructureStage
	retries int
}

func RetryInfra(stage model.InfrastructureStage, retries int) model.InfrastructureStage {
	return &retry{
		stage:   stage,
		retries: retries,
	}
}

func (r *retry) Express(run model.Run) error {
	var err error
	for i := 0; i < r.retries; i++ {
		if e := r.stage.Express(run); e == nil {
			continue
		} else {
			err = fmt.Errorf("%w", e)
		}
	}
	logrus.Warn("Infra stage successful after retries. Earlier errors: [%w]", err)
	return nil
}
