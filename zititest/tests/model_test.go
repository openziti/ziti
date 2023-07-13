/*
	(c) Copyright NetFoundry Inc.

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

package tests

import (
	"github.com/openziti/fablab"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/models/simple"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var run model.Run

func init() {
	cfg := model.GetConfig()
	instance, found := cfg.Instances[cfg.GetSelectedInstanceId()]
	if !found {
		panic(errors.Errorf("no instance found for current instance id %v", cfg.GetSelectedInstanceId()))
	}

	if instance.Model == simple.Model.Id {
		simple.InitBootstrapExtensions()
		fablab.InitModel(simple.Model)
	} else {
		panic(errors.Errorf("unsupported model for network tests [%v]", instance.Model))
	}

	if err := model.Bootstrap(); err != nil {
		logrus.Fatalf("unable to bootstrap (%s)", err)
	}

	var err error
	run, err = model.NewRun()
	if err != nil {
		logrus.WithError(err).Fatal("error initializing run")
	}
}
