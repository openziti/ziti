package validations

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/fablab/kernel/lib/tui"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/v2/common/pb/mgmt_pb"
	"github.com/openziti/ziti/v2/controller/rest_client/terminator"
	"github.com/openziti/ziti/v2/zitirest"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
	"google.golang.org/protobuf/proto"
)

type TerminatorValidationType int

const (
	ValidateSdkTerminators TerminatorValidationType = 0b01
	ValidateErtTerminators TerminatorValidationType = 0b10
)

func MinCount(n int64) func(int64) bool {
	return func(count int64) bool {
		return count >= n
	}
}

func ExactCount(n int64) func(int64) bool {
	return func(count int64) bool {
		return count == n
	}
}

func ValidateTerminators(run model.Run, timeout time.Duration, countOk func(int64) bool, validationType TerminatorValidationType) error {
	ctrls := run.GetModel().SelectComponents(".ctrl")
	errC := make(chan error, len(ctrls))
	deadline := time.Now().Add(timeout)
	for _, ctrl := range ctrls {
		ctrlComponent := ctrl
		go func() {
			errC <- ValidateTerminatorsForCtrl(run, ctrlComponent, deadline, countOk, validationType)
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

func ValidateTerminatorsForCtrl(run model.Run, c *model.Component, deadline time.Time, countOk func(int64) bool, validationType TerminatorValidationType) error {
	logger := tui.ValidationLogger().WithField("ctrl", c.Id)

	var clients *zitirest.Clients
	start := time.Now()
	var lastLog time.Time

	// Wait for terminator count to satisfy the caller's check
	for time.Now().Before(deadline) {
		if clients == nil {
			var err error
			clients, err = chaos.EnsureLoggedIntoCtrl(run, c, time.Minute)
			if err != nil {
				logger.WithError(err).Info("error logging into ctrl, will retry")
				time.Sleep(5 * time.Second)
				continue
			}
		}
		terminatorCount, err := GetTerminatorCount(clients)
		if err != nil {
			logger.WithError(err).Warn("error getting terminator count, will retry")
			clients = nil
			time.Sleep(5 * time.Second)
			continue
		}
		if countOk(terminatorCount) {
			logger.Infof("terminator count satisfied: %d, elapsed: %v", terminatorCount, time.Since(start))
			break
		}
		if time.Since(lastLog) > 30*time.Second {
			logger.Infof("waiting for terminators, current count: %d, elapsed: %v", terminatorCount, time.Since(start))
			lastLog = time.Now()
		}
		time.Sleep(5 * time.Second)
	}

	type validatorEntry struct {
		name     string
		validate func(string, *zitirest.Clients) (int, error)
	}

	var validators []validatorEntry
	if validationType&ValidateSdkTerminators != 0 {
		validators = append(validators, validatorEntry{name: "sdk", validate: ValidateRouterSdkTerminators})
	}
	if validationType&ValidateErtTerminators != 0 {
		validators = append(validators, validatorEntry{name: "ert", validate: ValidateRouterErtTerminators})
	}

	for _, v := range validators {
		for {
			if clients == nil {
				var err error
				clients, err = chaos.EnsureLoggedIntoCtrl(run, c, time.Minute)
				if err != nil {
					logger.WithError(err).Info("error logging into ctrl, will retry")
					if time.Now().After(deadline) {
						return err
					}
					time.Sleep(15 * time.Second)
					continue
				}
			}

			count, err := v.validate(c.Id, clients)
			if err == nil {
				break
			}

			clients = nil

			if time.Now().After(deadline) {
				return err
			}

			logger.Infof("current count of invalid %s terminators: %v, elapsed: %v", v.name, count, time.Since(start))
			time.Sleep(15 * time.Second)
		}
	}

	return nil
}

func GetTerminatorCount(clients *zitirest.Clients) (int64, error) {
	ctx, cancelF := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelF()

	filter := "limit 1"
	result, err := clients.Fabric.Terminator.ListTerminators(&terminator.ListTerminatorsParams{
		Filter:  &filter,
		Context: ctx,
	}, nil)

	if err != nil {
		return 0, err
	}
	count := *result.Payload.Meta.Pagination.TotalCount
	return count, nil
}

func ValidateRouterSdkTerminators(id string, clients *zitirest.Clients) (int, error) {
	logger := tui.ValidationLogger().WithField("ctrl", id)

	closeNotify := make(chan struct{})
	eventNotify := make(chan *mgmt_pb.RouterSdkTerminatorsDetails, 1)

	handleSdkTerminatorResults := func(msg *channel.Message, _ channel.Channel) {
		detail := &mgmt_pb.RouterSdkTerminatorsDetails{}
		if err := proto.Unmarshal(msg.Body, detail); err != nil {
			pfxlog.Logger().WithError(err).Error("unable to unmarshal router sdk terminator details")
			return
		}
		eventNotify <- detail
	}

	bindHandler := func(binding channel.Binding) error {
		binding.AddReceiveHandlerF(int32(mgmt_pb.ContentType_ValidateRouterSdkTerminatorsResultType), handleSdkTerminatorResults)
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

	request := &mgmt_pb.ValidateRouterSdkTerminatorsRequest{
		Filter: "limit none",
	}
	responseMsg, err := protobufs.MarshalTyped(request).WithTimeout(10 * time.Second).SendForReply(ch)

	response := &mgmt_pb.ValidateRouterSdkTerminatorsResponse{}
	if err = protobufs.TypedResponse(response).Unmarshall(responseMsg, err); err != nil {
		return 0, err
	}

	if !response.Success {
		return 0, fmt.Errorf("failed to start sdk terminator validation: %s", response.Message)
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
				return invalid, fmt.Errorf("error: unable to validate router %s (%s) on controller %s (%s)",
					routerDetail.RouterId, routerDetail.RouterName, id, routerDetail.Message)
			}
			for _, linkDetail := range routerDetail.Details {
				if !linkDetail.IsValid {
					invalid++
				}
			}
			expected--
		}
	}
	if invalid == 0 {
		logger.Infof("sdk terminator validation of %v routers successful", response.RouterCount)
		return invalid, nil
	}
	return invalid, fmt.Errorf("invalid sdk terminators found")
}

func ValidateRouterErtTerminators(id string, clients *zitirest.Clients) (int, error) {
	logger := tui.ValidationLogger().WithField("ctrl", id)

	closeNotify := make(chan struct{})
	eventNotify := make(chan *mgmt_pb.RouterErtTerminatorsDetails, 1)

	handleErtTerminatorResults := func(msg *channel.Message, _ channel.Channel) {
		detail := &mgmt_pb.RouterErtTerminatorsDetails{}
		if err := proto.Unmarshal(msg.Body, detail); err != nil {
			pfxlog.Logger().WithError(err).Error("unable to unmarshal router ert terminator details")
			return
		}
		eventNotify <- detail
	}

	bindHandler := func(binding channel.Binding) error {
		binding.AddReceiveHandlerF(int32(mgmt_pb.ContentType_ValidateRouterErtTerminatorsResultType), handleErtTerminatorResults)
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

	request := &mgmt_pb.ValidateRouterErtTerminatorsRequest{
		Filter: "limit none",
	}
	responseMsg, err := protobufs.MarshalTyped(request).WithTimeout(10 * time.Second).SendForReply(ch)

	response := &mgmt_pb.ValidateRouterErtTerminatorsResponse{}
	if err = protobufs.TypedResponse(response).Unmarshall(responseMsg, err); err != nil {
		return 0, err
	}

	if !response.Success {
		return 0, fmt.Errorf("failed to start ert terminator validation: %s", response.Message)
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
				return invalid, fmt.Errorf("error: unable to validate router %s (%s) on controller %s (%s)",
					routerDetail.RouterId, routerDetail.RouterName, id, routerDetail.Message)
			}
			for _, linkDetail := range routerDetail.Details {
				if !linkDetail.IsValid {
					invalid++
				}
			}
			expected--
		}
	}
	if invalid == 0 {
		logger.Infof("ert terminator validation of %v routers successful", response.RouterCount)
		return invalid, nil
	}
	return invalid, fmt.Errorf("invalid ert terminators found")
}
