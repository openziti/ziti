package router

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel"
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

	DumpForwarderTables byte = 1
	UpdateRoute         byte = 2
	CloseControlChannel byte = 3
	OpenControlChannel  byte = 4
	DumpLinks           byte = 5
)

func (self *Router) RegisterAgentBindHandler(bindHandler channel.BindHandler) {
	self.agentBindHandlers = append(self.agentBindHandlers, bindHandler)
}

func (self *Router) RegisterDefaultAgentOps(debugEnabled bool) {
	self.debugOperations[DumpForwarderTables] = self.debugOpWriteForwarderTables
	self.debugOperations[DumpLinks] = self.debugOpWriteLinks

	if debugEnabled {
		self.debugOperations[UpdateRoute] = self.debugOpUpdateRouter
		self.debugOperations[CloseControlChannel] = self.debugOpCloseControlChannel
		self.debugOperations[OpenControlChannel] = self.debugOpOpenControlChannel
	}

	self.agentBindHandlers = append(self.agentBindHandlers, channel.BindHandlerF(func(binding channel.Binding) error {
		if debugEnabled {
			binding.AddReceiveHandlerF(int32(mgmt_pb.ContentType_RouterDebugForgetLinkRequestType), self.agentOpForgetLink)
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

func (self *Router) debugOpWriteForwarderTables(c *bufio.ReadWriter) error {
	tables := self.forwarder.Debug()
	_, err := c.Write([]byte(tables))
	return err
}

func (self *Router) debugOpWriteLinks(c *bufio.ReadWriter) error {
	noLinks := true
	for link := range self.xlinkRegistry.Iter() {
		line := fmt.Sprintf("id: %v dest: %v protocol: %v\n", link.Id().Token, link.DestinationId(), link.LinkProtocol())
		_, err := c.WriteString(line)
		if err != nil {
			return err
		}
		noLinks = false
	}
	if noLinks {
		_, err := c.WriteString("no links\n")
		return err
	}
	return nil
}

func (self *Router) debugOpUpdateRouter(c *bufio.ReadWriter) error {
	logrus.Error("received debug operation to update routes")
	sizeBuf := make([]byte, 4)
	if _, err := c.Read(sizeBuf); err != nil {
		return err
	}
	size := binary.LittleEndian.Uint32(sizeBuf)
	messageBuf := make([]byte, size)

	if _, err := c.Read(messageBuf); err != nil {
		return err
	}

	route := &ctrl_pb.Route{}
	if err := proto.Unmarshal(messageBuf, route); err != nil {
		return err
	}

	logrus.Errorf("updating with route: %+v", route)
	logrus.Errorf("updating with route: %v", route)

	self.forwarder.Route(route)
	_, _ = c.WriteString("route added")
	return nil
}

func (self *Router) debugOpCloseControlChannel(c *bufio.ReadWriter) error {
	logrus.Warn("control channel: closing")
	_, _ = c.WriteString("control channel: closing\n")
	if toggleable, ok := self.Channel().Underlay().(connectionToggle); ok {
		if err := toggleable.Disconnect(); err != nil {
			logrus.WithError(err).Error("control channel: failed to close")
			_, _ = c.WriteString(fmt.Sprintf("control channel: failed to close (%v)\n", err))
		} else {
			logrus.Warn("control channel: closed")
			_, _ = c.WriteString("control channel: closed")
		}
	} else {
		logrus.Warn("control channel: error not toggleable")
		_, _ = c.WriteString("control channel: error not toggleable")
	}
	return nil
}

func (self *Router) debugOpOpenControlChannel(c *bufio.ReadWriter) error {
	logrus.Warn("control channel: reconnecting")
	if togglable, ok := self.Channel().Underlay().(connectionToggle); ok {
		if err := togglable.Reconnect(); err != nil {
			logrus.WithError(err).Error("control channel: failed to reconnect")
			_, _ = c.WriteString(fmt.Sprintf("control channel: failed to reconnect (%v)\n", err))
		} else {
			logrus.Warn("control channel: reconnected")
			_, _ = c.WriteString("control channel: reconnected")
		}
	} else {
		logrus.Warn("control channel: error not toggleable")
		_, _ = c.WriteString("control channel: error not toggleable")
	}
	return nil
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
