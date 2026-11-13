package validations

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5"
	"github.com/openziti/channel/v5/protobufs"
	"github.com/openziti/fablab/kernel/lib/tui"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/v2/common/pb/mgmt_pb"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
	"github.com/openziti/ziti/zititest/zitirest"
	"google.golang.org/protobuf/proto"
)

// ValidateRouterLinks checks that every controller's view of the routers' links matches the routers',
// retrying each controller until deadline passes.
func ValidateRouterLinks(run model.Run, deadline time.Duration) error {
	ctrls := run.GetModel().SelectComponents(".ctrl")
	errC := make(chan error, len(ctrls))
	dl := time.Now().Add(deadline)
	for _, ctrl := range ctrls {
		ctrlComponent := ctrl
		go func() {
			errC <- ValidateRouterLinksForCtrl(run, ctrlComponent, dl)
		}()
	}

	for range len(ctrls) {
		if err := <-errC; err != nil {
			return err
		}
	}
	return nil
}

// ValidateRouterLinksForCtrl runs ValidateRouterLinksOnce against controller c until it passes or
// deadline passes, returning the last error.
func ValidateRouterLinksForCtrl(run model.Run, c *model.Component, deadline time.Time) error {
	clients, err := chaos.EnsureLoggedIntoCtrl(run, c, time.Minute)
	if err != nil {
		return err
	}

	start := time.Now()
	logger := tui.ValidationLogger().WithField("ctrl", c.Id)

	for {
		count, err := ValidateRouterLinksOnce(c.Id, clients)
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}

		logger.Infof("router link validation not yet complete: %v (link errors: %v, elapsed time: %v)", err, count, time.Since(start))
		time.Sleep(15 * time.Second)

		clients, err = chaos.EnsureLoggedIntoCtrl(run, c, time.Minute)
		if err != nil {
			return err
		}
	}
}

// ValidateRouterLinksOnce asks the controller id's clients reach to compare its view of every router's
// links with the router's own, and returns how many links disagree. A router the controller can't
// reach is reported as an error after the disagreeing links, naming the first few.
func ValidateRouterLinksOnce(id string, clients *zitirest.Clients) (int, error) {
	logger := tui.ValidationLogger().WithField("ctrl", id)

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
	var unreachable []string
	for expected > 0 {
		select {
		case <-closeNotify:
			return 0, errors.New("unexpected close of mgmt channel")
		case routerDetail := <-eventNotify:
			expected--
			if !routerDetail.ValidateSuccess {
				// the controller couldn't reach this router, so its links are unknown rather than wrong;
				// keep draining so the other routers' results still count
				unreachable = append(unreachable,
					fmt.Sprintf("%s (%s): %s", routerDetail.RouterName, routerDetail.RouterId, routerDetail.Message))
				continue
			}
			for _, linkDetail := range routerDetail.LinkDetails {
				if !linkDetail.IsValid {
					invalid++
					logger.Infof("INVALID link %v on router %v (%v): ctrlState=%v routerState=%v destRouter=%v destConnected=%v dialed=%v messages=%v",
						linkDetail.LinkId, routerDetail.RouterId, routerDetail.RouterName,
						linkDetail.CtrlState, linkDetail.RouterState,
						linkDetail.DestRouterId, linkDetail.DestConnected,
						linkDetail.Dialed, linkDetail.Messages)
				}
			}
		}
	}

	// An inconsistent link is a real failure and is reported ahead of unreachability, which may just be
	// churn that has not settled yet.
	if invalid > 0 {
		return invalid, fmt.Errorf("invalid links found")
	}
	if len(unreachable) > 0 {
		return invalid, fmt.Errorf("%d of %d routers could not be validated on controller %s: %s",
			len(unreachable), response.RouterCount, id, describeUnreachable(unreachable))
	}
	logger.Infof("link validation of %v routers successful", response.RouterCount)
	return invalid, nil
}

// describeUnreachable renders the routers or components a controller could not reach, naming the first few
// so a failure says where to look, without embedding hundreds of entries in one error.
func describeUnreachable(entries []string) string {
	const maxNamed = 5
	if len(entries) <= maxNamed {
		return strings.Join(entries, "; ")
	}
	return fmt.Sprintf("%s; and %d more", strings.Join(entries[:maxNamed], "; "), len(entries)-maxNamed)
}
