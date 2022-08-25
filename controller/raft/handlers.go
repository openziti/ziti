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

package raft

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"github.com/hashicorp/raft"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/fabric/metrics"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/foundation/v2/goroutines"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"time"
)

const (
	NewLogEntryType     = 2050
	ErrorResponseType   = 2051
	SuccessResponseType = 2052
	JoinRequestType     = 2053
	RemoveRequestType   = 2054

	HeaderErrorCode = 1000
	IndexHeader     = 1001

	ErrorCodeBadMessage = 1
	ErrorCodeNotLeader  = 2
	ErrorCodeApiError   = 3
	ErrorCodeGeneric    = 4
)

func NewJoinHandler(controller *Controller) channel.TypedReceiveHandler {
	return &joinHandler{
		controller: controller,
	}
}

type joinHandler struct {
	controller *Controller
}

func (self *joinHandler) ContentType() int32 {
	return JoinRequestType
}

func (self *joinHandler) HandleReceive(m *channel.Message, ch channel.Channel) {
	go func() {
		req := &JoinRequest{}
		err := req.Decode(m)

		if err != nil {
			logrus.WithError(err).Error("error decoding join request")
			sendErrorResponse(m, ch, err, ErrorCodeBadMessage)
			return
		}

		logrus.Infof("received join request id: %v, addr: %v, voter: %v", req.Id, req.Addr, !req.IsVoter)

		err = self.controller.HandleJoin(req)
		if err != nil {
			if errors.Is(err, raft.ErrNotLeader) {
				sendErrorResponse(m, ch, err, ErrorCodeNotLeader)
			} else {
				sendErrorResponse(m, ch, err, ErrorCodeGeneric)
			}
		} else {
			resp := channel.NewMessage(SuccessResponseType, nil)
			resp.ReplyTo(m)
			if sendErr := ch.Send(resp); sendErr != nil {
				logrus.WithError(sendErr).Error("error while sending success response")
			}
		}
	}()
}

func NewRemoveHandler(controller *Controller) channel.TypedReceiveHandler {
	return &removeHandler{
		controller: controller,
	}
}

type removeHandler struct {
	controller *Controller
}

func (self *removeHandler) ContentType() int32 {
	return RemoveRequestType
}

func (self *removeHandler) HandleReceive(m *channel.Message, ch channel.Channel) {
	go func() {
		req := &RemoveRequest{}
		err := req.Decode(m)

		if err != nil {
			logrus.WithError(err).Error("error decoding remove request")
			sendErrorResponse(m, ch, err, ErrorCodeBadMessage)
			return
		}

		logrus.Infof("received remove request id: %v", req.Id)

		err = self.controller.HandleRemove(req)
		if err != nil {
			if errors.Is(err, raft.ErrNotLeader) {
				sendErrorResponse(m, ch, err, ErrorCodeNotLeader)
			} else {
				sendErrorResponse(m, ch, err, ErrorCodeGeneric)
			}
		} else {
			resp := channel.NewMessage(SuccessResponseType, nil)
			resp.ReplyTo(m)
			if sendErr := ch.Send(resp); sendErr != nil {
				logrus.WithError(sendErr).Error("error while sending success response")
			}
		}
	}()
}

func NewCommandHandler(controller *Controller) channel.TypedReceiveHandler {
	poolConfig := goroutines.PoolConfig{
		QueueSize:   uint32(controller.Config.CommandHandlerOptions.MaxQueueSize),
		MinWorkers:  0,
		MaxWorkers:  uint32(controller.Config.CommandHandlerOptions.MaxWorkers),
		IdleTime:    time.Second,
		CloseNotify: controller.closeNotify,
		PanicHandler: func(err interface{}) {
			pfxlog.Logger().WithField(logrus.ErrorKey, err).Error("panic during command processing")
		},
	}
	metrics.ConfigureGoroutinesPoolMetrics(&poolConfig, controller.metricsRegistry, "command_handler")
	pool, err := goroutines.NewPool(poolConfig)
	if err != nil {
		panic(err)
	}
	return &commandHandler{
		controller: controller,
		pool:       pool,
	}
}

type commandHandler struct {
	controller *Controller
	pool       goroutines.Pool
}

func (self *commandHandler) ContentType() int32 {
	return NewLogEntryType
}

func (self *commandHandler) HandleReceive(m *channel.Message, ch channel.Channel) {
	err := self.pool.Queue(func() {
		if idx, err := self.controller.ApplyEncodedCommand(m.Body); err != nil {
			sendErrorResponseCalculateType(m, ch, err)
			return
		} else {
			sendSuccessResponse(m, ch, idx)
		}
	})

	if err != nil {
		pfxlog.Logger().WithError(err).Error("unable to queue command for processing")
	}
}

func sendErrorResponseCalculateType(m *channel.Message, ch channel.Channel, err error) {
	if errors.Is(err, raft.ErrNotLeader) {
		sendErrorResponse(m, ch, err, ErrorCodeNotLeader)
	} else {
		sendApiErrorResponse(m, ch, models.ToApiError(err))
	}
}

func sendErrorResponse(m *channel.Message, ch channel.Channel, err error, errorCode uint32) {
	resp := channel.NewMessage(ErrorResponseType, []byte(err.Error()))
	resp.ReplyTo(m)
	resp.PutUint32Header(HeaderErrorCode, errorCode)

	if sendErr := ch.Send(resp); sendErr != nil {
		logrus.WithError(sendErr).Error("error while sending error response")
	}
}

func sendApiErrorResponse(m *channel.Message, ch channel.Channel, err *errorz.ApiError) {
	encodingMap := map[string]interface{}{}
	encodingMap["code"] = err.Code
	encodingMap["message"] = err.Message
	encodingMap["status"] = err.Status
	encodingMap["cause"] = err.Cause

	buf, encodeErr := json.Marshal(encodingMap)
	if encodeErr != nil {
		logrus.WithError(encodeErr).WithField("apiErr", err).Error("unable to encode api error")
		sendErrorResponse(m, ch, err, ErrorCodeGeneric)
		return
	}
	resp := channel.NewMessage(ErrorResponseType, buf)
	resp.ReplyTo(m)
	resp.PutUint32Header(HeaderErrorCode, ErrorCodeApiError)

	if sendErr := ch.Send(resp); sendErr != nil {
		logrus.WithError(sendErr).Error("error while sending error response")
	}
}

func sendSuccessResponse(m *channel.Message, ch channel.Channel, index uint64) {
	resp := channel.NewMessage(SuccessResponseType, nil)
	resp.ReplyTo(m)
	resp.PutUint64Header(IndexHeader, index)
	if sendErr := ch.Send(resp); sendErr != nil {
		logrus.WithError(sendErr).Error("error while sending success response")
	}
}

type JoinRequest struct {
	Addr    string
	Id      string
	IsVoter bool
}

func (self *JoinRequest) Encode() (*channel.Message, error) {
	buf := &bytes.Buffer{}

	encoder := gob.NewEncoder(buf)
	err := encoder.Encode(self)
	if err != nil {
		return nil, err
	}

	return channel.NewMessage(JoinRequestType, buf.Bytes()), nil
}

func (self *JoinRequest) Decode(msg *channel.Message) error {
	buf := bytes.NewReader(msg.Body)
	decoder := gob.NewDecoder(buf)
	return decoder.Decode(self)
}

type RemoveRequest struct {
	Id string
}

func (self *RemoveRequest) Encode() (*channel.Message, error) {
	buf := &bytes.Buffer{}

	encoder := gob.NewEncoder(buf)
	err := encoder.Encode(self)
	if err != nil {
		return nil, err
	}

	return channel.NewMessage(RemoveRequestType, buf.Bytes()), nil
}

func (self *RemoveRequest) Decode(msg *channel.Message) error {
	buf := bytes.NewReader(msg.Body)
	decoder := gob.NewDecoder(buf)
	return decoder.Decode(self)
}
