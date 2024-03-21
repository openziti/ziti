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

package main

import (
	"github.com/openziti/fablab/kernel/lib/parallel"
	"github.com/openziti/fablab/kernel/model"
	zitilib_actions "github.com/openziti/ziti/zititest/zitilab/actions"
	"path/filepath"
)

func provisionIdentities(identities []string, run model.Run) error {
	var tasks []parallel.Task

	identitiesDir := model.MakeBuildPath("identities")
	for _, v := range identities {
		id := v
		task := func() error {
			jwtFileName := filepath.Join(identitiesDir, id+".jwt")
			args := []string{"create", "enrollment", "ott", "--jwt-output-file", jwtFileName, "--", id}

			if err := zitilib_actions.EdgeExec(run.GetModel(), args...); err != nil {
				return err
			}

			args = []string{"enroll", jwtFileName}
			if err := zitilib_actions.EdgeExec(run.GetModel(), args...); err != nil {
				return err
			}
			return nil
		}
		tasks = append(tasks, task)
	}

	if err := parallel.Execute(tasks, 10); err != nil {
		return err
	}

	return nil
}
