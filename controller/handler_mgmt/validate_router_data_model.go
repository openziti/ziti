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

package handler_mgmt

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v3"
	"github.com/openziti/channel/v3/protobufs"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/ziti/common/pb/mgmt_pb"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/network"
	"google.golang.org/protobuf/proto"
	"time"
)

type validateRouterDataModelHandler struct {
	appEnv *env.AppEnv
}

func newValidateRouterDataModelHandler(appEnv *env.AppEnv) channel.TypedReceiveHandler {
	return &validateRouterDataModelHandler{appEnv: appEnv}
}

func (*validateRouterDataModelHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_ValidateRouterDataModelRequestType)
}

func (handler *validateRouterDataModelHandler) getNetwork() *network.Network {
	return handler.appEnv.GetHostController().GetNetwork()
}

func (handler *validateRouterDataModelHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label())
	request := &mgmt_pb.ValidateRouterDataModelRequest{}

	var err error

	var count int64
	var evalF func()
	if err = proto.Unmarshal(msg.Body, request); err == nil {
		count, evalF, err = handler.ValidateRouterDataModel(request.ValidateCtrl, request.RouterFilter, func(detail *mgmt_pb.RouterDataModelDetails) {
			if !ch.IsClosed() {
				if sendErr := protobufs.MarshalTyped(detail).WithTimeout(15 * time.Second).SendAndWaitForWire(ch); sendErr != nil {
					log.WithError(sendErr).Error("send of router data model detail failed, closing channel")
					if closeErr := ch.Close(); closeErr != nil {
						log.WithError(closeErr).Error("failed to close channel")
					}
				}
			} else {
				log.Info("channel closed, unable to send router data model detail")
			}
		})
	} else {
		log.WithError(err).Error("failed to unmarshal request")
		return
	}

	response := &mgmt_pb.ValidateRouterDataModelResponse{
		Success:        err == nil,
		ComponentCount: uint64(count),
	}
	if err != nil {
		response.Message = fmt.Sprintf("%v: failed to unmarshall request: %v", handler.getNetwork().GetAppId(), err)
	}

	body, err := proto.Marshal(response)
	if err != nil {
		pfxlog.Logger().WithError(err).Error("unexpected error serializing ValidateRouterDataModelResponse")
		return
	}

	responseMsg := channel.NewMessage(int32(mgmt_pb.ContentType_ValidateRouterDataModelResponseType), body)
	responseMsg.ReplyTo(msg)
	if err = ch.Send(responseMsg); err != nil {
		pfxlog.Logger().WithError(err).Error("unexpected error sending ValidateRouterDataModelResponse")
	}

	if evalF != nil {
		evalF()
	}
}

type RouterDataModelValidationCallback func(detail *mgmt_pb.RouterDataModelDetails)

func (handler *validateRouterDataModelHandler) ValidateRouterDataModel(includeCtrl bool, filter string, cb RouterDataModelValidationCallback) (int64, func(), error) {
	result, err := handler.appEnv.Managers.Router.BaseList(filter)
	if err != nil {
		return 0, nil, err
	}

	sem := concurrenz.NewSemaphore(10)

	evalF := func() {
		if includeCtrl {
			sem.Acquire()
			go func() {
				defer sem.Release()
				errs := handler.appEnv.Broker.ValidateRouterDataModel()

				var errStrings []string
				for _, err := range errs {
					errStrings = append(errStrings, err.Error())
				}

				details := &mgmt_pb.RouterDataModelDetails{
					ComponentType:   "controller",
					ComponentId:     handler.getNetwork().GetAppId(),
					ComponentName:   handler.getNetwork().GetAppId(),
					ValidateSuccess: len(errs) == 0,
					Errors:          errStrings,
				}
				cb(details)
			}()
		}
		for _, router := range result.Entities {
			connectedRouter := handler.appEnv.GetHostController().GetNetwork().GetConnectedRouter(router.Id)
			if connectedRouter != nil {
				sem.Acquire()
				go func() {
					defer sem.Release()

					details := &mgmt_pb.RouterDataModelDetails{
						ComponentType:   "router",
						ComponentId:     router.Id,
						ComponentName:   router.Name,
						ValidateSuccess: false,
						Errors:          []string{"not yet implemented"},
					}
					cb(details)
				}()
			} else {
				details := &mgmt_pb.RouterDataModelDetails{
					ComponentType:   "router",
					ComponentId:     router.Id,
					ComponentName:   router.Name,
					ValidateSuccess: false,
					Errors:          []string{"router not connected to controller"},
				}
				cb(details)
			}
		}
	}

	count := int64(len(result.Entities))
	if includeCtrl {
		count++
	}

	return count, evalF, nil
}
