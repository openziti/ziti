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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/common/pb/mgmt_pb"
	"github.com/openziti/ziti/zitirest"
	"github.com/openziti/ziti/zititest/zitilab"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
	zitiLibOps "github.com/openziti/ziti/zititest/zitilab/runlevel/5_operation"
	"google.golang.org/protobuf/proto"
	"math/rand"
	"sync"
	"time"
)

func RunSimScenarios(run model.Run, services *zitiLibOps.SimServices) error {
	if err := run.GetModel().Exec(run, "startSimMetrics"); err != nil {
		return err
	}

	simControl, err := services.GetSimController(run, "sim-control")
	if err != nil {
		return err
	}

	sims := run.GetModel().FilterComponents(".loop-client", func(c *model.Component) bool {
		t, ok := c.Type.(*zitilab.Loop4SimType)
		return ok && t.Mode == zitilab.Loop4RemoteControlled
	})

	err = simControl.WaitForAllConnected(time.Second*30, sims)
	if err != nil {
		return err
	}

	results, err := simControl.StartSimScenarios()
	if err != nil {
		return err
	}

	return results.GetResults(5 * time.Minute)
}

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

func validateCircuits(run model.Run) error {
	ctrls := run.GetModel().SelectComponents(".ctrl")
	errC := make(chan error, len(ctrls))
	deadline := time.Now().Add(time.Minute)
	for _, ctrl := range ctrls {
		ctrlComponent := ctrl
		go validateCircuitsForCtrlWithChan(run, ctrlComponent, deadline, errC)
	}

	for i := 0; i < len(ctrls); i++ {
		err := <-errC
		if err != nil {
			return err
		}
	}

	return nil
}

func validateCircuitsForCtrlWithChan(run model.Run, c *model.Component, deadline time.Time, errC chan<- error) {
	errC <- validateCircuitsForCtrl(run, c, deadline)
}

func validateCircuitsForCtrl(run model.Run, c *model.Component, deadline time.Time) error {
	clients, err := chaos.EnsureLoggedIntoCtrl(run, c, time.Minute)
	if err != nil {
		return err
	}

	start := time.Now()

	logger := pfxlog.Logger().WithField("ctrl", c.Id)

	first := true
	for {
		count, err := validateCircuitsForCtrlOnce(c.Id, clients, first)
		if err == nil {
			return nil
		}

		if time.Now().After(deadline) {
			return err
		}

		logger.Infof("current count of circuit errors: %v, elapsed time: %v, current err: %v", count, time.Since(start), err)
		time.Sleep(15 * time.Second)

		clients, err = chaos.EnsureLoggedIntoCtrl(run, c, time.Minute)
		if err != nil {
			return err
		}
		first = false
	}
}

func validateCircuitsForCtrlOnce(id string, clients *zitirest.Clients, first bool) (int, error) {
	logger := pfxlog.Logger().WithField("ctrl", id)

	closeNotify := make(chan struct{})
	eventNotify := make(chan *mgmt_pb.RouterCircuitDetails, 1)

	handleResults := func(msg *channel.Message, _ channel.Channel) {
		detail := &mgmt_pb.RouterCircuitDetails{}
		if err := proto.Unmarshal(msg.Body, detail); err != nil {
			pfxlog.Logger().WithError(err).Error("unable to unmarshal circuit validation details")
			return
		}
		eventNotify <- detail
	}

	bindHandler := func(binding channel.Binding) error {
		binding.AddReceiveHandlerF(int32(mgmt_pb.ContentType_ValidateCircuitsResultType), handleResults)
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

	request := &mgmt_pb.ValidateCircuitsRequest{
		RouterFilter: "limit none",
	}
	responseMsg, err := protobufs.MarshalTyped(request).WithTimeout(10 * time.Second).SendForReply(ch)

	response := &mgmt_pb.ValidateCircuitsResponse{}
	if err = protobufs.TypedResponse(response).Unmarshall(responseMsg, err); err != nil {
		return 0, err
	}

	if !response.Success {
		return 0, fmt.Errorf("failed to start circuit validation: %s", response.Message)
	}

	logger.Infof("started validation of %v components", response.RouterCount)

	expected := response.RouterCount

	invalid := 0
	for expected > 0 {
		select {
		case <-closeNotify:
			fmt.Printf("channel closed, exiting")
			return 0, errors.New("unexpected close of mgmt channel")
		case detail := <-eventNotify:
			if !detail.ValidateSuccess {
				invalid++
				fmt.Printf("error validating router %s using ctrl %s: %s", detail.RouterId, id, detail.Message)
			}
			for _, details := range detail.Details {
				if details.IsInErrorState() {
					if !first {
						fmt.Printf("\tcircuit: %s ctrl: %v, fwd: %v, edge: %v, sdk: %v, dest: %+v\n",
							details.CircuitId, details.MissingInCtrl, details.MissingInForwarder,
							details.MissingInEdge, details.MissingInSdk, details.Destinations)
					}
					invalid++
				}
			}
			expected--
		}
	}
	if invalid == 0 {
		logger.Infof("circuit validation of %v routers successful", response.RouterCount)
		return invalid, nil
	}
	return invalid, errors.New("errors found")
}
