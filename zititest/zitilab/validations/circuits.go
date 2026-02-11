package validations

import (
	"errors"
	"fmt"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/fablab/kernel/lib/tui"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/v2/common/pb/mgmt_pb"
	"github.com/openziti/ziti/v2/zitirest"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
	"google.golang.org/protobuf/proto"
)

func ValidateCircuits(run model.Run, deadline time.Duration, routerFilter string) error {
	ctrls := run.GetModel().SelectComponents(".ctrl")
	errC := make(chan error, len(ctrls))
	dl := time.Now().Add(deadline)
	for _, ctrl := range ctrls {
		ctrlComponent := ctrl
		go func() {
			errC <- ValidateCircuitsForCtrl(run, ctrlComponent, dl, routerFilter)
		}()
	}

	for range len(ctrls) {
		err := <-errC
		if err != nil {
			return err
		}
	}

	return nil
}

func ValidateCircuitsForCtrl(run model.Run, c *model.Component, deadline time.Time, routerFilter string) error {
	clients, err := chaos.EnsureLoggedIntoCtrl(run, c, time.Minute)
	if err != nil {
		return err
	}

	start := time.Now()
	logger := tui.ValidationLogger().WithField("ctrl", c.Id)

	first := true
	for {
		count, err := validateCircuitsOnce(c.Id, clients, routerFilter, first)
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

func validateCircuitsOnce(id string, clients *zitirest.Clients, routerFilter string, first bool) (int, error) {
	logger := tui.ValidationLogger().WithField("ctrl", id)

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
		RouterFilter: routerFilter,
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
			logger.Info("channel closed, exiting")
			return 0, errors.New("unexpected close of mgmt channel")
		case detail := <-eventNotify:
			if !detail.ValidateSuccess {
				invalid++
				logger.Infof("error validating router %s using ctrl %s: %s", detail.RouterId, id, detail.Message)
			}
			for _, details := range detail.Details {
				if details.IsInErrorState() {
					if !first {
						logger.Infof("\tcircuit: %s ctrl: %v, fwd: %v, edge: %v, sdk: %v, dest: %+v",
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
