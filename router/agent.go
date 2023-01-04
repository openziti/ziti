package router

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/fabric/handler_common"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"io"
	"net"
	"time"
)

const (
	AgentAppId byte = 2
)

func (self *Router) RegisterAgentBindHandler(bindHandler channel.BindHandler) {
	self.agentBindHandlers = append(self.agentBindHandlers, bindHandler)
}

func (self *Router) RegisterDefaultAgentOps(debugEnabled bool) {
	self.agentBindHandlers = append(self.agentBindHandlers, channel.BindHandlerF(func(binding channel.Binding) error {
		binding.AddReceiveHandlerF(int32(mgmt_pb.ContentType_RouterDebugDumpForwarderTablesRequestType), self.agentOpDumpForwarderTables)
		binding.AddReceiveHandlerF(int32(mgmt_pb.ContentType_RouterDebugDumpLinksRequestType), self.agentOpsDumpLinks)

		if debugEnabled {
			binding.AddReceiveHandlerF(int32(mgmt_pb.ContentType_RouterDebugUpdateRouteRequestType), self.agentOpUpdateRoute)
			binding.AddReceiveHandlerF(int32(mgmt_pb.ContentType_RouterDebugForgetLinkRequestType), self.agentOpForgetLink)
			binding.AddReceiveHandlerF(int32(mgmt_pb.ContentType_RouterDebugToggleCtrlChannelRequestType), self.agentOpToggleCtrlChan)
		}
		return nil
	}))
}

func (self *Router) RegisterAgentOp(opId byte, f func(c *bufio.ReadWriter) error) {
	self.debugOperations[opId] = f
}

func (self *Router) bindAgentChannel(binding channel.Binding) error {
	for _, bh := range self.agentBindHandlers {
		if err := binding.Bind(bh); err != nil {
			return err
		}
	}
	return nil
}

func (self *Router) HandleAgentAsyncOp(conn net.Conn) error {
	logrus.Debug("received agent operation request")

	appIdBuf := []byte{0}
	_, err := io.ReadFull(conn, appIdBuf)
	if err != nil {
		return err
	}
	appId := appIdBuf[0]

	if appId != AgentAppId {
		logrus.WithField("appId", appId).Debug("invalid app id on agent request")
		return errors.New("invalid operation for controller")
	}

	options := channel.DefaultOptions()
	options.ConnectTimeout = time.Second
	listener := channel.NewExistingConnListener(self.config.Id, conn, nil)
	_, err = channel.NewChannel("agent", listener, channel.BindHandlerF(self.bindAgentChannel), options)
	return err
}

func (self *Router) agentOpForgetLink(m *channel.Message, ch channel.Channel) {
	log := pfxlog.Logger()
	linkId := string(m.Body)
	var found bool
	if link, _ := self.xlinkRegistry.GetLinkById(linkId); link != nil {
		self.xlinkRegistry.DebugForgetLink(linkId)
		self.forwarder.UnregisterLink(link)
		found = true
	}

	log.Infof("forget of link %v was requested. link found? %v", linkId, found)
	result := fmt.Sprintf("link removed: %v", found)
	handler_common.SendOpResult(m, ch, "link.remove", result, true)
}

func (self *Router) agentOpToggleCtrlChan(m *channel.Message, ch channel.Channel) {
	ctrlId := string(m.Body)

	results := &bytes.Buffer{}
	toggleOn, _ := m.GetBoolHeader(int32(mgmt_pb.Header_CtrlChanToggle))

	success := true
	count := 0
	self.ctrls.ForEach(func(controllerId string, ch channel.Channel) {
		if ctrlId == "" || controllerId == ctrlId {
			log := pfxlog.Logger().WithField("ctrlId", controllerId)
			if toggleable, ok := ch.Underlay().(connectionToggle); ok {
				if toggleOn {
					if err := toggleable.Reconnect(); err != nil {
						log.WithError(err).Error("control channel: failed to reconnect")
						_, _ = fmt.Fprintf(results, "control channel: failed to reconnect (%v)\n", err)
						success = false
					} else {
						log.Warn("control channel: reconnected")
						_, _ = fmt.Fprint(results, "control channel: reconnected")
						count++
					}
				} else {
					if err := toggleable.Disconnect(); err != nil {
						log.WithError(err).Error("control channel: failed to close")
						_, _ = fmt.Fprintf(results, "control channel: failed to close (%v)\n", err)
						success = false
					} else {
						log.Warn("control channel: closed")
						_, _ = fmt.Fprint(results, "control channel: closed")
						count++
					}
				}
			} else {
				log.Warn("control channel: not toggleable")
				_, _ = fmt.Fprint(results, "control channel: not toggleable")
				success = false
			}
		}
	})

	if count == 0 {
		_, _ = fmt.Fprintf(results, "control channel: no controllers matched id [%v]", ctrlId)
		success = false
	}

	handler_common.SendOpResult(m, ch, "ctrl.toggle", results.String(), success)
}

func (self *Router) agentOpDumpForwarderTables(m *channel.Message, ch channel.Channel) {
	tables := self.forwarder.Debug()
	handler_common.SendOpResult(m, ch, "dump.forwarder_tables", tables, true)
}

func (self *Router) agentOpsDumpLinks(m *channel.Message, ch channel.Channel) {
	result := &bytes.Buffer{}
	for link := range self.xlinkRegistry.Iter() {
		line := fmt.Sprintf("id: %v dest: %v protocol: %v\n", link.Id(), link.DestinationId(), link.LinkProtocol())
		_, err := result.WriteString(line)
		if err != nil {
			handler_common.SendOpResult(m, ch, "dump.links", err.Error(), false)
			return
		}
	}

	output := result.String()
	if len(output) == 0 {
		output = "no links\n"
	}
	handler_common.SendOpResult(m, ch, "dump.links", output, true)
}

func (self *Router) agentOpUpdateRoute(m *channel.Message, ch channel.Channel) {
	logrus.Warn("received debug operation to update routes")
	ctrlId, _ := m.GetStringHeader(int32(mgmt_pb.Header_ControllerId))
	if ctrlId == "" {
		handler_common.SendOpResult(m, ch, "update.route", "no controller id provided", false)
		return
	}

	ctrl := self.ctrls.GetCtrlChannel(ctrlId)
	if ctrl == nil {
		handler_common.SendOpResult(m, ch, "update.route", fmt.Sprintf("no control channel found for [%v]", ctrlId), false)
		return
	}

	route := &ctrl_pb.Route{}
	if err := proto.Unmarshal(m.Body, route); err != nil {
		handler_common.SendOpResult(m, ch, "update.route", err.Error(), false)
		return
	}

	if err := self.forwarder.Route(ctrlId, route); err != nil {
		handler_common.SendOpResult(m, ch, "update.route", errors.Wrap(err, "error adding route").Error(), false)
		return
	}

	logrus.Warnf("route added: %+v", route)
	handler_common.SendOpResult(m, ch, "update.route", "route added", true)
}

func (self *Router) HandleAgentOp(conn net.Conn) error {
	bconn := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	appId, err := bconn.ReadByte()
	if err != nil {
		return err
	}

	if appId != AgentAppId {
		return errors.Errorf("invalid operation for router")
	}

	op, err := bconn.ReadByte()

	if err != nil {
		return err
	}

	if opF, ok := self.debugOperations[op]; ok {
		if err := opF(bconn); err != nil {
			return err
		}
		return bconn.Flush()
	}
	return errors.Errorf("invalid operation %v", op)
}
