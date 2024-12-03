/*
	Copyright NetFoundry Inc.

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
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v3"
	"github.com/openziti/channel/v3/protobufs"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/fablab/kernel/lib/parallel"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/common/pb/mgmt_pb"
	"github.com/openziti/ziti/ziti/util"
	"github.com/openziti/ziti/zitirest"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
	"github.com/openziti/ziti/zititest/zitilab/models"
	"google.golang.org/protobuf/proto"
	"math/rand"
	"strings"
	"sync"
	"time"
)

type CtrlClients struct {
	ctrls   []*zitirest.Clients
	ctrlMap map[string]*zitirest.Clients
	sync.Mutex
}

func (self *CtrlClients) init(run model.Run, selector string) error {
	self.ctrlMap = map[string]*zitirest.Clients{}
	ctrls := run.GetModel().SelectComponents(selector)
	resultC := make(chan struct {
		err     error
		id      string
		clients *zitirest.Clients
	}, len(ctrls))

	for _, ctrl := range ctrls {
		go func() {
			clients, err := chaos.EnsureLoggedIntoCtrl(run, ctrl, time.Minute)
			resultC <- struct {
				err     error
				id      string
				clients *zitirest.Clients
			}{
				err:     err,
				id:      ctrl.Id,
				clients: clients,
			}
		}()
	}

	for i := 0; i < len(ctrls); i++ {
		result := <-resultC
		if result.err != nil {
			return result.err
		}
		self.ctrls = append(self.ctrls, result.clients)
		self.ctrlMap[result.id] = result.clients
	}
	return nil
}

func (self *CtrlClients) getRandomCtrl() *zitirest.Clients {
	return self.ctrls[rand.Intn(len(self.ctrls))]
}

func (self *CtrlClients) getCtrl(id string) *zitirest.Clients {
	return self.ctrlMap[id]
}

// start with a random scenario then cycle through them
var scenarioCounter = rand.Intn(7)

func sowChaos(run model.Run) error {
	ctrls := &CtrlClients{}
	if err := ctrls.init(run, ".ctrl"); err != nil {
		return err
	}

	var tasks []parallel.LabeledTask
	var err error

	applyTasks := func(f func(run model.Run, ctrls *CtrlClients) ([]parallel.LabeledTask, error)) {
		var t []parallel.LabeledTask
		if err == nil {
			t, err = f(run, ctrls)
			if err == nil {
				tasks = append(tasks, t...)
			}
		}
	}

	applyTasks(getRestartTasks)
	applyTasks(getServiceChaosTasks)
	applyTasks(getIdentityChaosTasks)
	applyTasks(getServicePolicyChaosTasks)

	if err != nil {
		return err
	}

	chaos.Randomize(tasks)

	retryPolicy := func(task parallel.LabeledTask, attempt int, err error) parallel.ErrorAction {
		if strings.HasPrefix(task.Type(), "delete.") {
			var apiErr util.ApiErrorPayload
			if errors.As(err, &apiErr) {
				if apiErr.GetPayload().Error.Code == errorz.NotFoundCode {
					return parallel.ErrActionIgnore
				}
			}
		}
		if attempt > 3 {
			return parallel.ErrActionReport
		}
		pfxlog.Logger().WithField("attempt", attempt).WithError(err).WithField("task", task.Label()).Error("action failed, retrying")
		time.Sleep(time.Second)
		return parallel.ErrActionRetry
	}
	return parallel.ExecuteLabeled(tasks, 2, retryPolicy)
}

func getRestartTasks(run model.Run, _ *CtrlClients) ([]parallel.LabeledTask, error) {
	var controllers []*model.Component
	var err error

	scenarioCounter = (scenarioCounter + 1) % 7
	scenario := scenarioCounter + 1

	var result []parallel.LabeledTask

	if scenario&0b001 > 0 {
		controllers, err = chaos.SelectRandom(run, ".ctrl", chaos.RandomOfTotal())
		if err != nil {
			return nil, err
		}
		for _, controller := range controllers {
			result = append(result, parallel.TaskWithLabel("restart.ctrl", fmt.Sprintf("restart controller %s", controller.Id), func() error {
				return chaos.RestartSelected(run, 1, controller)
			}))
		}
	}

	var routers []*model.Component
	if scenario&0b010 > 0 {
		routers, err = chaos.SelectRandom(run, ".router", chaos.PercentageRange(10, 75))
		if err != nil {
			return nil, err
		}
		for _, router := range routers {
			result = append(result, parallel.TaskWithLabel("restart.router", fmt.Sprintf("restart router %s", router.Id), func() error {
				return chaos.RestartSelected(run, 1, router)
			}))
		}
	}

	return result, nil
}

func getRoles(n int) []string {
	roles := getRoleAttributes(n)
	for i, role := range roles {
		roles[i] = "#" + role
	}
	return roles
}

func getRoleAttributes(n int) []string {
	attr := map[string]struct{}{}
	count := rand.Intn(n) + 1
	for i := 0; i < count; i++ {
		attr[fmt.Sprintf("role-%v", rand.Intn(10))] = struct{}{}
	}

	var result []string
	for k := range attr {
		result = append(result, k)
	}
	return result
}

func getRoleAttributesAsAttrPtr(n int) *rest_model.Attributes {
	result := getRoleAttributes(n)
	return (*rest_model.Attributes)(&result)
}

func newId() *string {
	id := uuid.NewString()
	return &id
}

func newBoolPtr() *bool {
	b := rand.Int()%2 == 0
	return &b
}

func getServiceChaosTasks(_ model.Run, ctrls *CtrlClients) ([]parallel.LabeledTask, error) {
	svcs, err := models.ListServices(ctrls.getRandomCtrl(), "limit none", 15*time.Second)
	if err != nil {
		return nil, err
	}
	chaos.Randomize(svcs)

	var result []parallel.LabeledTask

	for i := 0; i < 5; i++ {
		result = append(result, parallel.TaskWithLabel("delete.service", fmt.Sprintf("delete service %s", *svcs[i].ID), func() error {
			return models.DeleteService(ctrls.getRandomCtrl(), *svcs[i].ID, 15*time.Second)
		}))
	}

	for i := 5; i < 10; i++ {
		result = append(result, parallel.TaskWithLabel("modify.service", fmt.Sprintf("modify service %s", *svcs[i].ID), func() error {
			svc := svcs[i]
			svc.RoleAttributes = getRoleAttributesAsAttrPtr(3)
			svc.Name = newId()
			return models.UpdateServiceFromDetail(ctrls.getRandomCtrl(), svc, 15*time.Second)
		}))
	}

	for i := 0; i < 5; i++ {
		result = append(result, createNewService(ctrls.getRandomCtrl()))
	}

	return result, nil
}

func getIdentityChaosTasks(_ model.Run, ctrls *CtrlClients) ([]parallel.LabeledTask, error) {
	entities, err := models.ListIdentities(ctrls.getRandomCtrl(), "not isAdmin limit none", 15*time.Second)
	if err != nil {
		return nil, err
	}
	chaos.Randomize(entities)

	var result []parallel.LabeledTask

	for i := 0; i < 5; i++ {
		result = append(result, parallel.TaskWithLabel("delete.identity", fmt.Sprintf("delete identity %s", *entities[i].ID), func() error {
			return models.DeleteIdentity(ctrls.getRandomCtrl(), *entities[i].ID, 15*time.Second)
		}))
	}

	for i := 5; i < 10; i++ {
		result = append(result, parallel.TaskWithLabel("modify.identity", fmt.Sprintf("modify identity %s", *entities[i].ID), func() error {
			entity := entities[i]
			entity.RoleAttributes = getRoleAttributesAsAttrPtr(3)
			entity.Name = newId()
			return models.UpdateIdentityFromDetail(ctrls.getRandomCtrl(), entity, 15*time.Second)
		}))
	}

	for i := 0; i < 5; i++ {
		result = append(result, createNewIdentity(ctrls.getRandomCtrl()))
	}

	return result, nil
}

func getServicePolicyChaosTasks(_ model.Run, ctrls *CtrlClients) ([]parallel.LabeledTask, error) {
	entities, err := models.ListServicePolicies(ctrls.getRandomCtrl(), "limit none", 15*time.Second)
	if err != nil {
		return nil, err
	}
	chaos.Randomize(entities)

	var result []parallel.LabeledTask

	for i := 0; i < 5; i++ {
		result = append(result, parallel.TaskWithLabel("delete.service-policy", fmt.Sprintf("delete service policy %s", *entities[i].ID), func() error {
			return models.DeleteServicePolicy(ctrls.getRandomCtrl(), *entities[i].ID, 15*time.Second)
		}))
	}

	for i := 5; i < 10; i++ {
		result = append(result, parallel.TaskWithLabel("modify.service-policy", fmt.Sprintf("modify service policy %s", *entities[i].ID), func() error {
			entity := entities[i]
			entity.IdentityRoles = getRoles(3)
			entity.ServiceRoles = getRoles(3)
			entity.PostureCheckRoles = getRoles(3)
			entity.Name = newId()
			return models.UpdateServicePolicyFromDetail(ctrls.getRandomCtrl(), entity, 15*time.Second)
		}))
	}

	for i := 0; i < 5; i++ {
		result = append(result, createNewServicePolicy(ctrls.getRandomCtrl()))
	}

	return result, nil
}

func createNewService(ctrl *zitirest.Clients) parallel.LabeledTask {
	return parallel.TaskWithLabel("create.service", "create new service", func() error {
		svc := &rest_model.ServiceCreate{
			Configs:            nil,
			EncryptionRequired: newBoolPtr(),
			Name:               newId(),
			RoleAttributes:     getRoleAttributes(3),
			TerminatorStrategy: "smartrouting",
		}
		return models.CreateService(ctrl, svc, 15*time.Second)
	})
}

func createNewIdentity(ctrl *zitirest.Clients) parallel.LabeledTask {
	isAdmin := false
	identityType := rest_model.IdentityTypeDefault
	return parallel.TaskWithLabel("create.identity", "create new identity", func() error {
		svc := &rest_model.IdentityCreate{
			DefaultHostingCost:        nil,
			DefaultHostingPrecedence:  "",
			IsAdmin:                   &isAdmin,
			Name:                      newId(),
			RoleAttributes:            getRoleAttributesAsAttrPtr(3),
			ServiceHostingCosts:       nil,
			ServiceHostingPrecedences: nil,
			Tags:                      nil,
			Type:                      &identityType,
		}
		return models.CreateIdentity(ctrl, svc, 15*time.Second)
	})
}

func createNewServicePolicy(ctrl *zitirest.Clients) parallel.LabeledTask {
	return parallel.TaskWithLabel("create.service-policy", "create new service policy", func() error {
		anyOf := rest_model.SemanticAnyOf
		policyType := rest_model.DialBindDial
		if rand.Int()%2 == 0 {
			policyType = rest_model.DialBindBind
		}
		entity := &rest_model.ServicePolicyCreate{
			Name:              newId(),
			IdentityRoles:     getRoles(3),
			PostureCheckRoles: getRoles(3),
			Semantic:          &anyOf,
			ServiceRoles:      getRoles(3),
			Type:              &policyType,
		}
		return models.CreateServicePolicy(ctrl, entity, 15*time.Second)
	})
}

func validateRouterDataModel(run model.Run) error {
	ctrls := run.GetModel().SelectComponents(".ctrl")
	errC := make(chan error, len(ctrls))
	deadline := time.Now().Add(15 * time.Minute)
	for _, ctrl := range ctrls {
		ctrlComponent := ctrl
		go validateRouterDataModelForCtrlWithChan(run, ctrlComponent, deadline, errC)
	}

	for i := 0; i < len(ctrls); i++ {
		err := <-errC
		if err != nil {
			return err
		}
	}

	return nil
}

func validateRouterDataModelForCtrlWithChan(run model.Run, c *model.Component, deadline time.Time, errC chan<- error) {
	errC <- validateRouterDataModelForCtrl(run, c, deadline)
}

func validateRouterDataModelForCtrl(run model.Run, c *model.Component, deadline time.Time) error {
	clients, err := chaos.EnsureLoggedIntoCtrl(run, c, time.Minute)
	if err != nil {
		return err
	}

	start := time.Now()

	logger := pfxlog.Logger().WithField("ctrl", c.Id)

	for {
		count, err := validateRouterDataModelForCtrlOnce(c.Id, clients)
		if err == nil {
			return nil
		}

		if time.Now().After(deadline) {
			return err
		}

		logger.Infof("current count of router data model errors: %v, elapsed time: %v", count, time.Since(start))
		time.Sleep(15 * time.Second)

		clients, err = chaos.EnsureLoggedIntoCtrl(run, c, time.Minute)
		if err != nil {
			return err
		}
	}
}

func validateRouterDataModelForCtrlOnce(id string, clients *zitirest.Clients) (int, error) {
	logger := pfxlog.Logger().WithField("ctrl", id)

	closeNotify := make(chan struct{})
	eventNotify := make(chan *mgmt_pb.RouterDataModelDetails, 1)

	handleSdkTerminatorResults := func(msg *channel.Message, _ channel.Channel) {
		detail := &mgmt_pb.RouterDataModelDetails{}
		if err := proto.Unmarshal(msg.Body, detail); err != nil {
			pfxlog.Logger().WithError(err).Error("unable to unmarshal router data model details")
			return
		}
		eventNotify <- detail
	}

	bindHandler := func(binding channel.Binding) error {
		binding.AddReceiveHandlerF(int32(mgmt_pb.ContentType_ValidateRouterDataModelResultType), handleSdkTerminatorResults)
		binding.AddCloseHandler(channel.CloseHandlerF(func(ch channel.Channel) {
			close(closeNotify)
		}))
		return nil
	}

	ch, err := clients.NewWsMgmtChannel(channel.BindHandlerF(bindHandler))
	if err != nil {
		return 0, err
	}

	defer func() {
		_ = ch.Close()
	}()

	request := &mgmt_pb.ValidateRouterDataModelRequest{
		RouterFilter: "limit none",
		ValidateCtrl: true,
	}
	responseMsg, err := protobufs.MarshalTyped(request).WithTimeout(10 * time.Second).SendForReply(ch)

	response := &mgmt_pb.ValidateRouterDataModelResponse{}
	if err = protobufs.TypedResponse(response).Unmarshall(responseMsg, err); err != nil {
		return 0, err
	}

	if !response.Success {
		return 0, fmt.Errorf("failed to start router data model validation: %s", response.Message)
	}

	logger.Infof("started validation of %v components", response.ComponentCount)

	expected := response.ComponentCount

	invalid := 0
	for expected > 0 {
		select {
		case <-closeNotify:
			fmt.Printf("channel closed, exiting")
			return 0, errors.New("unexpected close of mgmt channel")
		case detail := <-eventNotify:
			if !detail.ValidateSuccess {
				invalid++
			}
			for _, errorDetails := range detail.Errors {
				fmt.Printf("\tdetail: %s\n", errorDetails)
			}
			expected--
		}
	}
	if invalid == 0 {
		logger.Infof("router data model validation of %v components successful", response.ComponentCount)
		return invalid, nil
	}
	return invalid, errors.New("errors found")
}
