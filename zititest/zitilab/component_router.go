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

package zitilab

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/fablab/kernel/lib"
	"github.com/openziti/fablab/kernel/lib/parallel"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/foundation/v2/util"
	"github.com/openziti/ziti/v2/zitirest"
	zitilib_actions "github.com/openziti/ziti/zititest/zitilab/actions"
	"github.com/openziti/ziti/zititest/zitilab/models"
	"github.com/openziti/ziti/zititest/zitilab/stageziti"
)

var _ model.ComponentType = (*RouterType)(nil)

const (
	RouterActionsCreateAndEnroll = "createAndEnroll"
	RouterActionsReEnroll        = "reEnroll"
)

type RouterType struct {
	ConfigSourceFS fs.FS
	ConfigSource   string
	ConfigName     string
	Version        string
	LocalPath      string
	Debug          bool
}

func (self *RouterType) Label() string {
	return "ziti-router"
}

func (self *RouterType) GetVersion() string {
	return self.Version
}

func (self *RouterType) SetVersion(version string) {
	self.Version = version
}

func (self *RouterType) InitType(*model.Component) {
	canonicalizeGoAppVersion(&self.Version)
}

func (self *RouterType) GetActions() map[string]model.ComponentAction {
	return map[string]model.ComponentAction{
		RouterActionsCreateAndEnroll: model.ComponentActionF(self.CreateAndEnroll),
		RouterActionsReEnroll:        model.ComponentActionF(self.ReEnroll),
	}
}

func (self *RouterType) Dump() any {
	return map[string]string{
		"type_id":       "router",
		"config_source": self.ConfigSource,
		"config_name":   self.ConfigName,
		"version":       self.Version,
		"local_path":    self.LocalPath,
	}
}

func (self *RouterType) InitializeHost(run model.Run, c *model.Component) error {
	if self.isTunneler(c) {
		return setupDnsForTunneler(c)
	}
	return nil
}

func (self *RouterType) StageFiles(r model.Run, c *model.Component) error {
	configSource := self.ConfigSource
	if configSource == "" {
		configSource = "router.yml.tmpl"
	}

	configName := self.GetConfigName(c)

	if err := lib.GenerateConfigForComponent(c, self.ConfigSourceFS, configSource, configName, r); err != nil {
		return err
	}

	return stageziti.StageZitiOnce(r, c, self.Version, self.LocalPath)
}

func (self *RouterType) isTunneler(c *model.Component) bool {
	return c.HasLocalOrAncestralTag("tunneler")
}

func (self *RouterType) GetConfigName(c *model.Component) string {
	configName := self.ConfigName
	if configName == "" {
		configName = c.Id + ".yml"
	}
	return configName
}

func (self *RouterType) getProcessFilter(c *model.Component) func(string) bool {
	return getZitiProcessFilter(c, "router")
}

func (self *RouterType) IsRunning(_ model.Run, c *model.Component) (bool, error) {
	pids, err := c.GetHost().FindProcesses(self.getProcessFilter(c))
	if err != nil {
		return false, err
	}
	return len(pids) > 0, nil
}

func (self *RouterType) Start(r model.Run, c *model.Component) error {
	isRunninng, err := self.IsRunning(r, c)
	if err != nil {
		return err
	}
	if isRunninng {
		fmt.Printf("router %s already started\n", c.Id)
		return nil
	}
	extraArgs := ""
	if self.Debug {
		extraArgs = " -v "
	}
	return startZitiComponent(c, "router", self.Version, self.GetConfigName(c), extraArgs)
}

func (self *RouterType) Stop(run model.Run, c *model.Component) error {
	if err := c.GetHost().KillProcesses("-TERM", self.getProcessFilter(c)); err != nil {
		return err
	}
	for i := 0; i < 10; i++ {
		if isRunning, err := self.IsRunning(run, c); err == nil && !isRunning {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return c.GetHost().KillProcesses("-KILL", self.getProcessFilter(c))
}

func (self *RouterType) CreateAndEnroll(run model.Run, c *model.Component) error {
	jwtFileName := filepath.Join(run.GetTmpDir(), c.Id+".jwt")

	attributes := strings.Join(c.Tags, ",")

	args := []string{"create", "edge-router", c.Id, "--timeout", strconv.Itoa(60), "-j", "--jwt-output-file", jwtFileName, "-a", attributes}

	isTunneler := c.HasLocalOrAncestralTag("tunneler")
	if isTunneler {
		args = append(args, "--tunneler-enabled")
	}

	if c.HasLocalOrAncestralTag("no-traversal") {
		args = append(args, "--no-traversal")
	}

	if err := zitilib_actions.EdgeExec(c.GetModel(), args...); err != nil {
		return err
	}

	if isTunneler {
		attempt := 1
		for {
			if err := zitilib_actions.EdgeExec(c.GetModel(), "update", "identity", c.Id, "-a", attributes, "--timeout", strconv.Itoa(60)); err != nil {
				if attempt > 4 {
					return err
				}
				attempt++
				time.Sleep(time.Duration(attempt) * time.Second)
			} else {
				break
			}
		}
	}

	remoteJwt := "/home/ubuntu/fablab/cfg/" + c.Id + ".jwt"
	if err := c.GetHost().SendFile(jwtFileName, remoteJwt); err != nil {
		return err
	}

	tmpl := "set -o pipefail; %s router enroll /home/ubuntu/fablab/cfg/%s -j %s 2>&1 | tee /home/ubuntu/logs/%s.router.enroll.log "
	cmd := fmt.Sprintf(tmpl, GetZitiBinaryPath(c, self.Version), self.GetConfigName(c), remoteJwt, c.Id)

	return c.GetHost().ExecLogOnlyOnError(cmd)
}

func (self *RouterType) CreateAndEnrollTasks(run model.Run, c *model.Component, client *zitirest.Clients) []parallel.LabeledTask {
	var tasks []parallel.LabeledTask

	createDoneNotify := make(chan struct{})
	jwtRetrievalDone := make(chan struct{})
	jwtSendDone := make(chan struct{})

	var idHolder concurrenz.AtomicValue[string]
	isTunneler := c.HasLocalOrAncestralTag("tunneler")

	createEdgeRouterTask := parallel.Task(func() error {
		createRequest := &rest_model.EdgeRouterCreate{
			Disabled:          util.Ptr(false),
			IsTunnelerEnabled: isTunneler,
			Name:              util.Ptr(c.Id),
			NoTraversal:       util.Ptr(c.HasLocalOrAncestralTag("no-traversal")),
			RoleAttributes:    util.Ptr(rest_model.Attributes(c.Tags)),
			Tags:              nil,
		}
		id, err := models.CreateEdgeRouter(client, createRequest, 15*time.Second)
		if err != nil {
			return err
		}
		idHolder.Store(id)
		close(createDoneNotify)
		return nil
	})

	jwtFileName := filepath.Join(run.GetTmpDir(), c.Id+".jwt")

	getJwtTask := parallel.Task(func() error {
		select {
		case <-createDoneNotify:
		case <-time.After(time.Minute):
			return fmt.Errorf("timed out waiting for create of edge router %s", c.Id)
		}

		id := idHolder.Load()

		l, err := models.ListEdgeRouters(client, fmt.Sprintf(`id="%s"`, id), 5*time.Second)
		if err != nil {
			return err
		}
		if len(l) != 1 {
			return fmt.Errorf("expected 1 edge router, got %d", len(l))
		}
		er := l[0]

		if err = os.WriteFile(jwtFileName, []byte(*er.EnrollmentJWT), 0600); err != nil {
			fmt.Printf("Failed to write JWT to file(%v)\n", jwtFileName)
			return err
		}

		close(jwtRetrievalDone)
		return nil
	})

	remoteJwt := "/home/ubuntu/fablab/cfg/" + c.Id + ".jwt"

	sendJwtTask := parallel.Task(func() error {
		select {
		case <-jwtRetrievalDone:
		case <-time.After(time.Minute):
			return fmt.Errorf("timed out waiting for create of edge router %s", c.Id)
		}

		if err := c.GetHost().SendFile(jwtFileName, remoteJwt); err != nil {
			return err
		}

		close(jwtSendDone)
		return nil
	})

	enrollEdgeRouterTask := parallel.Task(func() error {
		select {
		case <-jwtSendDone:
		case <-time.After(time.Minute):
			return fmt.Errorf("timed out waiting for create of edge router %s", c.Id)
		}

		tmpl := "set -o pipefail; %s router enroll /home/ubuntu/fablab/cfg/%s -j %s 2>&1 | tee /home/ubuntu/logs/%s.router.enroll.log "
		cmd := fmt.Sprintf(tmpl, GetZitiBinaryPath(c, self.Version), self.GetConfigName(c), remoteJwt, c.Id)
		return c.GetHost().ExecLogOnlyOnError(cmd)
	})

	var updateTunnelerIdentity parallel.Task

	if isTunneler {
		updateTunnelerIdentity = func() error {
			select {
			case <-createDoneNotify:
			case <-time.After(time.Minute):
				return fmt.Errorf("timed out waiting for create of edge router %s", c.Id)
			}
			id := idHolder.Load()
			identityPatchRequest := &rest_model.IdentityPatch{
				RoleAttributes: util.Ptr(rest_model.Attributes(c.Tags)),
			}
			return models.PatchIdentity(client, id, identityPatchRequest, 15*time.Second)
		}
	}

	tasks = append(tasks, parallel.TaskWithLabel("edge-router.create", fmt.Sprintf("create edge router %s", c.Id), createEdgeRouterTask))
	tasks = append(tasks, parallel.TaskWithLabel("edge-router.get-jwt", fmt.Sprintf("get edge router %s JWT", c.Id), getJwtTask))
	tasks = append(tasks, parallel.TaskWithLabel("edge-router.send-jwt", fmt.Sprintf("send edge router %s JWT", c.Id), sendJwtTask))
	tasks = append(tasks, parallel.TaskWithLabel("edge-router.enroll", fmt.Sprintf("enroll edge router %s", c.Id), enrollEdgeRouterTask))

	if isTunneler {
		tasks = append(tasks, parallel.TaskWithLabel("edge-router.update-identity", fmt.Sprintf("update edge router identity %s", c.Id), updateTunnelerIdentity))
	}

	return tasks
}

func (self *RouterType) ReEnroll(run model.Run, c *model.Component) error {
	jwtFileName := filepath.Join(model.ConfigBuild(), c.Id+".jwt")

	args := []string{"re-enroll", "edge-router", "-j", "--jwt-output-file", jwtFileName, "--", c.Id}

	if err := zitilib_actions.EdgeExec(c.GetModel(), args...); err != nil {
		return err
	}

	remoteJwt := "/home/ubuntu/fablab/cfg/" + c.Id + ".jwt"
	if err := c.GetHost().SendFile(jwtFileName, remoteJwt); err != nil {
		return err
	}

	zitiVersion := self.Version

	ctrls := run.GetModel().SelectComponents(".ctrl")
	for _, ctrl := range ctrls {
		if ctrl.Type != nil {
			zitiVersion = ctrl.Type.GetVersion()
			break
		}
	}

	tmpl := "set -o pipefail; %s router enroll /home/ubuntu/fablab/cfg/%s -j %s 2>&1 | tee /home/ubuntu/logs/%s.router.enroll.log "
	cmd := fmt.Sprintf(tmpl, GetZitiBinaryPath(c, zitiVersion), self.GetConfigName(c), remoteJwt, c.Id)

	return c.GetHost().ExecLogOnlyOnError(cmd)
}
