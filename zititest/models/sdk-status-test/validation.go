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
	"github.com/openziti/ziti/zititest/zitilab/chaos"
	"google.golang.org/protobuf/proto"
	"math/rand"
	"time"
)

// start with a random scenario then cycle through them
var scenarioCounter = rand.Intn(7)

func sowChaos(run model.Run) error {
	var controllers []*model.Component
	var err error

	scenarioCounter = (scenarioCounter + 1) % 7
	scenario := scenarioCounter + 1

	if scenario&0b001 > 0 {
		controllers, err = chaos.SelectRandom(run, ".ctrl", chaos.RandomInRange(1, 2))
		if err != nil {
			return err
		}
		time.Sleep(5 * time.Second)
	}

	var routers []*model.Component
	if scenario&0b010 > 0 {
		routers, err = chaos.SelectRandom(run, ".router", chaos.PercentageRange(10, 75))
		if err != nil {
			return err
		}
	}

	var hosts []*model.Component
	if scenario&0b100 > 0 {
		hosts, err = chaos.SelectRandom(run, ".host", chaos.PercentageRange(10, 75))
		if err != nil {
			return err
		}
	}

	fmt.Printf("stopping %d controllers,  %d routers and %d hosts\n", len(controllers), len(routers), len(hosts))
	if err = chaos.RestartSelected(run, 3, controllers...); err != nil {
		return err
	}
	var toStop []*model.Component
	toStop = append(toStop, routers...)
	toStop = append(toStop, hosts...)
	return chaos.StopSelected(run, toStop, 100)
}

func validateSdkStatus(run model.Run) error {
	ctrls := run.GetModel().SelectComponents(".ctrl")
	errC := make(chan error, len(ctrls))
	deadline := time.Now().Add(8 * time.Minute)
	for _, ctrl := range ctrls {
		ctrlComponent := ctrl
		go validateSdkStatusForCtrlWithChan(run, ctrlComponent, deadline, errC)
	}

	for i := 0; i < len(ctrls); i++ {
		err := <-errC
		if err != nil {
			return err
		}
	}

	return nil
}

func validateSdkStatusForCtrlWithChan(run model.Run, c *model.Component, deadline time.Time, errC chan<- error) {
	errC <- validateSdkStatusForCtrl(run, c, deadline)
}

func validateSdkStatusForCtrl(run model.Run, c *model.Component, deadline time.Time) error {
	clients, err := chaos.EnsureLoggedIntoCtrl(run, c, time.Minute)
	if err != nil {
		return err
	}

	start := time.Now()
	logger := pfxlog.Logger().WithField("ctrl", c.Id)

	for {
		count, err := validateSdkStatuses(c.Id, clients)
		if err == nil {
			return nil
		}

		if time.Now().After(deadline) {
			return err
		}

		logger.Infof("current count of invalid sdk statuses: %v, elapsed time: %v", count, time.Since(start))
		beforeLogin := time.Now()
		clients, err = chaos.EnsureLoggedIntoCtrl(run, c, time.Minute)
		if err != nil {
			return err
		}
		elapsed := time.Since(beforeLogin)
		if elapsed < 5*time.Second {
			time.Sleep((5 * time.Second) - elapsed)
		}
	}
}

func validateSdkStatuses(id string, clients *zitirest.Clients) (int, error) {
	logger := pfxlog.Logger().WithField("ctrl", id)

	closeNotify := make(chan struct{})
	eventNotify := make(chan *mgmt_pb.RouterIdentityConnectionStatusesDetails, 1)

	handleSdkStatusResults := func(msg *channel.Message, _ channel.Channel) {
		detail := &mgmt_pb.RouterIdentityConnectionStatusesDetails{}
		if err := proto.Unmarshal(msg.Body, detail); err != nil {
			pfxlog.Logger().WithError(err).Error("unable to unmarshal router sdk status details")
			return
		}
		eventNotify <- detail
	}

	bindHandler := func(binding channel.Binding) error {
		binding.AddReceiveHandlerF(int32(mgmt_pb.ContentType_ValidateIdentityConnectionStatusesResultType), handleSdkStatusResults)
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

	request := &mgmt_pb.ValidateIdentityConnectionStatusesRequest{
		RouterFilter: "limit none",
	}
	responseMsg, err := protobufs.MarshalTyped(request).WithTimeout(10 * time.Second).SendForReply(ch)

	response := &mgmt_pb.ValidateIdentityConnectionStatusesResponse{}
	if err = protobufs.TypedResponse(response).Unmarshall(responseMsg, err); err != nil {
		return 0, err
	}

	if !response.Success {
		return 0, fmt.Errorf("failed to start sdk statuses validation: %s", response.Message)
	}

	logger.Infof("started validation of %v routers", response.ComponentCount)

	expected := response.ComponentCount

	invalid := 0
	for expected > 0 {
		select {
		case <-closeNotify:
			fmt.Printf("channel closed, exiting")
			return 0, errors.New("unexpected close of mgmt channel")
		case routerDetail := <-eventNotify:
			if len(routerDetail.Errors) > 0 {
				for _, errMsg := range routerDetail.Errors {
					logger.Infof("router %s (%s) reported error: %s", routerDetail.ComponentId, routerDetail.ComponentName, errMsg)
					invalid++
				}
			}
			expected--
		}
	}
	if invalid == 0 {
		logger.Infof("sdk status validation of %v routers successful", response.ComponentCount)
		return invalid, nil
	}
	return invalid, fmt.Errorf("invalid sdk statuses found")
}
