package validations

import (
	"errors"
	"fmt"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5"
	"github.com/openziti/channel/v5/protobufs"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/v2/common/pb/mgmt_pb"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
	"github.com/openziti/ziti/zititest/zitirest"
	"google.golang.org/protobuf/proto"
)

// ValidateRouterDataModel checks that every controller's router data model matches each router's,
// retrying each controller until deadline passes.
func ValidateRouterDataModel(run model.Run, deadline time.Duration) error {
	ctrls := run.GetModel().SelectComponents(".ctrl")
	errC := make(chan error, len(ctrls))
	dl := time.Now().Add(deadline)
	for _, ctrl := range ctrls {
		ctrlComponent := ctrl
		go func() {
			errC <- ValidateRouterDataModelForCtrl(run, ctrlComponent, dl)
		}()
	}

	for range len(ctrls) {
		if err := <-errC; err != nil {
			return err
		}
	}
	return nil
}

// ValidateRouterDataModelForCtrl runs ValidateRouterDataModelOnce against controller c until it passes
// or deadline passes, returning the last error.
func ValidateRouterDataModelForCtrl(run model.Run, c *model.Component, deadline time.Time) error {
	clients, err := chaos.EnsureLoggedIntoCtrl(run, c, time.Minute)
	if err != nil {
		return err
	}

	start := time.Now()
	logger := pfxlog.Logger().WithField("ctrl", c.Id)

	for {
		count, err := ValidateRouterDataModelOnce(c.Id, clients)
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

// ValidateRouterDataModelOnce asks the controller id's clients reach to compare its router data model
// with every router's and its own, and returns how many components disagree.
func ValidateRouterDataModelOnce(id string, clients *zitirest.Clients) (int, error) {
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
