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

package handler_ctrl

import (
	"errors"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v3"
	"github.com/openziti/channel/v3/protobufs"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/ziti/common/datapipe"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/router/env"
	"google.golang.org/protobuf/proto"
	"io"
	"net"
	"time"
)

var mgmtMsgTypes = datapipe.MessageTypes{
	DataMessageType:  int32(ctrl_pb.ContentType_CtrlPipeDataType),
	PipeIdHeaderType: int32(ctrl_pb.ControlHeaders_CtrlPipeIdHeader),
	CloseMessageType: int32(ctrl_pb.ContentType_CtrlPipeCloseType),
}

type mgmtPipe interface {
	WriteToServer(b []byte) error
	CloseWithErr(err error)
}

type pipeRegistry struct {
	pipes concurrenz.CopyOnWriteMap[uint32, mgmtPipe]
}

type ctrlPipeHandler struct {
	env      env.RouterEnv
	registry *pipeRegistry
	ch       channel.Channel
}

func newCtrlPipeHandler(routerEnv env.RouterEnv, registry *pipeRegistry, ch channel.Channel) *ctrlPipeHandler {
	return &ctrlPipeHandler{
		env:      routerEnv,
		registry: registry,
		ch:       ch,
	}
}

func (*ctrlPipeHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_CtrlPipeRequestType)
}

func (handler *ctrlPipeHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label()).Entry

	req := &ctrl_pb.CtrlPipeRequest{}
	if err := proto.Unmarshal(msg.Body, req); err != nil {
		log.WithError(err).Error("unable to unmarshal ssh tunnel request")
		return
	}

	if handler.env.GetMgmtPipeConfig().IsLocalPort() {
		handler.pipeToLocalPort(msg, req)
		return
	}

	log.Error("no configured pipe handler")
	handler.respondError(msg, "no configured pipe handler")
}

func (handler *ctrlPipeHandler) pipeToLocalPort(msg *channel.Message, req *ctrl_pb.CtrlPipeRequest) {
	log := pfxlog.ContextLogger(handler.ch.Label()).
		WithField("destination", fmt.Sprintf("127.0.0.1:%d", handler.env.GetMgmtPipeConfig().DestinationPort)).
		WithField("pipeId", req.ConnId)

	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", handler.env.GetMgmtPipeConfig().DestinationPort))
	if err != nil {
		log.WithError(err).Error("failed to dial pipe destination")
		handler.respondError(msg, err.Error())
		return
	}

	pipe := &ctrlChanPipe{
		conn: conn,
		ch:   handler.ch,
		id:   req.ConnId,
	}

	handler.registry.pipes.Put(pipe.id, pipe)

	log = log.WithField("connId", pipe.id)
	log.Info("registered ctrl channel pipe connection")

	response := &ctrl_pb.CtrlPipeResponse{
		Success: true,
	}

	if sendErr := protobufs.MarshalTyped(response).ReplyTo(msg).WithTimeout(5 * time.Second).SendAndWaitForWire(handler.ch); sendErr != nil {
		log.WithError(sendErr).Error("unable to send ctrl channel pipe response for successful pipe")
		pipe.CloseWithErr(sendErr)
		return
	}

	log.Info("started mgmt pipe")

	go pipe.readLoop()
}

func (handler *ctrlPipeHandler) PipeToEmbeddedSshServer(msg *channel.Message, req *ctrl_pb.CtrlPipeRequest) {
	log := pfxlog.ContextLogger(handler.ch.Label()).
		WithField("destination", "embedded-ssh-server").
		WithField("pipeId", req.ConnId)

	cfg := handler.env.GetMgmtPipeConfig()

	requestHandler, err := cfg.NewSshRequestHandler(handler.env.GetRouterId())
	if err != nil {
		log.WithError(err).Error("failed to connect pipe")
		handler.respondError(msg, "failed to connect pipe")
		return
	}

	pipe := datapipe.NewEmbeddedSshConn(handler.ch, req.ConnId, &mgmtMsgTypes)

	handler.registry.pipes.Put(pipe.Id(), pipe)
	log.Info("registered mgmt pipe connection")

	response := &ctrl_pb.CtrlPipeResponse{
		Success: true,
	}

	if sendErr := protobufs.MarshalTyped(response).ReplyTo(msg).WithTimeout(5 * time.Second).SendAndWaitForWire(handler.ch); sendErr != nil {
		log.WithError(sendErr).Error("unable to send mgmt pipe response for successful pipe")
		pipe.CloseWithErr(sendErr)
		return
	}

	if err = requestHandler.HandleSshRequest(pipe); err != nil {
		log.WithError(err).Error("failed to connect pipe")
		handler.respondError(msg, err.Error())
		pipe.CloseWithErr(err)
		return
	}

	log.Info("started mgmt pipe to local controller using embedded ssh server")
}

func (handler *ctrlPipeHandler) respondError(request *channel.Message, msg string) {
	response := &ctrl_pb.CtrlPipeResponse{
		Success: false,
		Msg:     msg,
	}

	if sendErr := protobufs.MarshalTyped(response).ReplyTo(request).WithTimeout(5 * time.Second).SendAndWaitForWire(handler.ch); sendErr != nil {
		log := pfxlog.ContextLogger(handler.ch.Label()).Entry
		log.WithError(sendErr).Error("unable to send ctrl channel pipe response for failed pipe")
	}
}

type ctrlChanPipe struct {
	id   uint32
	conn net.Conn
	ch   channel.Channel
}

func (self *ctrlChanPipe) WriteToServer(b []byte) error {
	_, err := self.conn.Write(b)
	return err
}

func (self *ctrlChanPipe) readLoop() {
	for {
		buf := make([]byte, 10240)
		n, err := self.conn.Read(buf)
		if err != nil {
			self.CloseWithErr(err)
			return
		}
		buf = buf[:n]
		msg := channel.NewMessage(int32(ctrl_pb.ContentType_CtrlPipeDataType), buf)
		msg.PutUint32Header(int32(ctrl_pb.ControlHeaders_CtrlPipeIdHeader), self.id)
		if err = self.ch.Send(msg); err != nil {
			self.CloseWithErr(err)
			return
		}
	}
}

func (self *ctrlChanPipe) CloseWithErr(err error) {
	log := pfxlog.ContextLogger(self.ch.Label()).WithField("connId", self.id)

	log.WithError(err).Info("closing ctrl channel pipe connection")

	if closeErr := self.conn.Close(); closeErr != nil {
		log.WithError(closeErr).Error("failed closing ctrl channel pipe connection")
	}

	if err != io.EOF && err != nil {
		msg := channel.NewMessage(int32(ctrl_pb.ContentType_CtrlPipeCloseType), []byte(err.Error()))
		msg.PutUint32Header(int32(ctrl_pb.ControlHeaders_CtrlPipeIdHeader), self.id)
		if sendErr := self.ch.Send(msg); sendErr != nil {
			log.WithError(sendErr).Error("failed sending ctrl channel pipe close message")
		}
	}
}

func newCtrlPipeDataHandler(registry *pipeRegistry) *ctrlPipeDataHandler {
	return &ctrlPipeDataHandler{
		registry: registry,
	}
}

type ctrlPipeDataHandler struct {
	registry *pipeRegistry
}

func (*ctrlPipeDataHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_CtrlPipeDataType)
}

func (handler *ctrlPipeDataHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	pipeId, _ := msg.GetUint32Header(int32(ctrl_pb.ControlHeaders_CtrlPipeIdHeader))
	pipe := handler.registry.pipes.Get(pipeId)

	if pipe == nil {
		log := pfxlog.ContextLogger(ch.Label()).WithField("pipeId", pipeId)
		log.Error("no ctrl channel pipe found for given id")

		go func() {
			errorMsg := fmt.Sprintf("invalid pipe id '%v", pipeId)
			replyMsg := channel.NewMessage(int32(ctrl_pb.ContentType_CtrlPipeCloseType), []byte(errorMsg))
			replyMsg.PutUint32Header(int32(ctrl_pb.ControlHeaders_CtrlPipeIdHeader), pipeId)
			if sendErr := ch.Send(msg); sendErr != nil {
				log.WithError(sendErr).Error("failed sending ctrl channel pipe close message after data with invalid conn")
			}
		}()
		return
	}

	if err := pipe.WriteToServer(msg.Body); err != nil {
		pipe.CloseWithErr(err)
	}
}

func newCtrlPipeCloseHandler(registry *pipeRegistry) *ctrlPipeCloseHandler {
	return &ctrlPipeCloseHandler{
		registry: registry,
	}
}

type ctrlPipeCloseHandler struct {
	registry *pipeRegistry
}

func (*ctrlPipeCloseHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_CtrlPipeCloseType)
}

func (handler *ctrlPipeCloseHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	pipeId, _ := msg.GetUint32Header(int32(ctrl_pb.ControlHeaders_CtrlPipeIdHeader))
	log := pfxlog.ContextLogger(ch.Label()).WithField("pipeId", pipeId)

	if pipe := handler.registry.pipes.Get(pipeId); pipe == nil {
		log.Error("no mgmt pipe found for given id")
	} else {
		pipe.CloseWithErr(errors.New("close message received"))
	}
}
