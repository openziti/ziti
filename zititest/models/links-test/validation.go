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
	"context"
	"errors"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/common/pb/mgmt_pb"
	"github.com/openziti/ziti/controller/rest_client/link"
	"github.com/openziti/ziti/zitirest"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
	"google.golang.org/protobuf/proto"
	"time"
)

func sowChaos(run model.Run) error {
	controllers, err := chaos.SelectRandom(run, ".ctrl", chaos.RandomOfTotal())
	if err != nil {
		return err
	}
	time.Sleep(5 * time.Second)
	routers, err := chaos.SelectRandom(run, ".router", chaos.PercentageRange(10, 75))
	if err != nil {
		return err
	}
	toRestart := append(routers, controllers...)
	fmt.Printf("restarting %v controllers and %v routers\n", len(controllers), len(routers))
	return chaos.RestartSelected(run, 100, toRestart...)
}

func validateLinks(run model.Run) error {
	ctrls := run.GetModel().SelectComponents(".ctrl")
	errC := make(chan error, len(ctrls))
	deadline := time.Now().Add(15 * time.Minute)
	for _, ctrl := range ctrls {
		ctrlComponent := ctrl
		go validateLinksForCtrlWithChan(run, ctrlComponent, deadline, errC)
	}

	for i := 0; i < len(ctrls); i++ {
		err := <-errC
		if err != nil {
			return err
		}
	}

	return nil
}

func validateLinksForCtrlWithChan(run model.Run, c *model.Component, deadline time.Time, errC chan<- error) {
	errC <- validateLinksForCtrl(run, c, deadline)
}

func validateLinksForCtrl(run model.Run, c *model.Component, deadline time.Time) error {
	clients, err := chaos.EnsureLoggedIntoCtrl(run, c, time.Minute)
	if err != nil {
		return err
	}

	allLinksPresent := false
	start := time.Now()

	logger := pfxlog.Logger().WithField("ctrl", c.Id)
	var lastLog time.Time
	for time.Now().Before(deadline) && !allLinksPresent {
		linkCount, err := getLinkCount(clients)
		if err != nil {
			return nil
		}
		if linkCount == 79800 {
			allLinksPresent = true
		} else {
			time.Sleep(5 * time.Second)
		}
		if time.Since(lastLog) > time.Minute {
			logger.Infof("current link count: %v, elapsed time: %v", linkCount, time.Since(start))
			lastLog = time.Now()
		}
	}

	if allLinksPresent {
		logger.Infof("all links present, elapsed time: %v", time.Since(start))
	} else {
		return fmt.Errorf("fail to reach expected link count of 79800 on controller %v", c.Id)
	}

	for {
		count, err := validateRouterLinks(c.Id, clients)
		if err == nil {
			return nil
		}

		if time.Now().After(deadline) {
			return err
		}

		logger.Infof("current link errors: %v, elapsed time: %v", count, time.Since(start))
		time.Sleep(15 * time.Second)
	}
}

func getLinkCount(clients *zitirest.Clients) (int64, error) {
	ctx, cancelF := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelF()

	filter := "limit 1"
	result, err := clients.Fabric.Link.ListLinks(&link.ListLinksParams{
		Filter:  &filter,
		Context: ctx,
	})

	if err != nil {
		return 0, err
	}
	linkCount := *result.Payload.Meta.Pagination.TotalCount
	return linkCount, nil
}

func validateRouterLinks(id string, clients *zitirest.Clients) (int, error) {
	logger := pfxlog.Logger().WithField("ctrl", id)

	closeNotify := make(chan struct{})
	eventNotify := make(chan *mgmt_pb.RouterLinkDetails, 1)

	handleLinkResults := func(msg *channel.Message, _ channel.Channel) {
		detail := &mgmt_pb.RouterLinkDetails{}
		if err := proto.Unmarshal(msg.Body, detail); err != nil {
			pfxlog.Logger().WithError(err).Error("unable to unmarshal router link details")
			return
		}
		eventNotify <- detail
	}

	bindHandler := func(binding channel.Binding) error {
		binding.AddReceiveHandlerF(int32(mgmt_pb.ContentType_ValidateRouterLinksResultType), handleLinkResults)
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

	request := &mgmt_pb.ValidateRouterLinksRequest{
		Filter: "limit none",
	}
	responseMsg, err := protobufs.MarshalTyped(request).WithTimeout(10 * time.Second).SendForReply(ch)

	response := &mgmt_pb.ValidateRouterLinksResponse{}
	if err = protobufs.TypedResponse(response).Unmarshall(responseMsg, err); err != nil {
		return 0, err
	}

	if !response.Success {
		return 0, fmt.Errorf("failed to start link validation: %s", response.Message)
	}

	logger.Infof("started validation of %v routers", response.RouterCount)

	expected := response.RouterCount

	invalid := 0
	for expected > 0 {
		select {
		case <-closeNotify:
			fmt.Printf("channel closed, exiting")
			return 0, errors.New("unexpected close of mgmt channel")
		case routerDetail := <-eventNotify:
			if !routerDetail.ValidateSuccess {
				return invalid, fmt.Errorf("error: unable to validate on controller %s (%s)", routerDetail.Message, id)
			}
			for _, linkDetail := range routerDetail.LinkDetails {
				if !linkDetail.IsValid {
					invalid++
				}
			}
			expected--
		}
	}
	if invalid == 0 {
		logger.Infof("link validation of %v routers successful", response.RouterCount)
		return invalid, nil
	}
	return invalid, fmt.Errorf("invalid links found")
}
