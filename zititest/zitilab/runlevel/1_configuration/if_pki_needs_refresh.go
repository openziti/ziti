/*
	Copyright 2019 NetFoundry Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package zitilib_runlevel_1_configuration

import (
	"fmt"
	"github.com/openziti/fablab/kernel/model"
	"os"
)

func IfPkiNeedsRefresh(stages ...model.ConfigurationStage) model.ConfigurationStage {
	return &ifPkiNeedsRefresh{stages: stages}
}

func (self *ifPkiNeedsRefresh) Configure(run model.Run) error {
	if changed, err := haveIpsChanged(run.GetModel()); err != nil || !changed {
		return err
	}

	if existing, err := hasExisitingPki(); err == nil && existing {
		if err = os.RemoveAll(model.PkiBuild()); err != nil {
			return err
		}
	}

	for _, stage := range self.stages {
		if err := stage.Configure(run); err != nil {
			return fmt.Errorf("error running configuration stage (%w)", err)
		}
	}

	return nil
}

type ifPkiNeedsRefresh struct {
	stages []model.ConfigurationStage
}
