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
	"encoding/json"
	"errors"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/ziti/common/inspect"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/common/pb/mgmt_pb"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/network"
	"google.golang.org/protobuf/proto"
	"strings"
	"time"
)

type validateCircuitsHandler struct {
	appEnv *env.AppEnv
}

func newValidateCircuitsHandler(appEnv *env.AppEnv) *validateCircuitsHandler {
	return &validateCircuitsHandler{appEnv: appEnv}
}

func (*validateCircuitsHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_ValidateCircuitsRequestType)
}

func (handler *validateCircuitsHandler) getNetwork() *network.Network {
	return handler.appEnv.GetHostController().GetNetwork()
}

func (handler *validateCircuitsHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label())
	request := &mgmt_pb.ValidateCircuitsRequest{}

	var err error

	var count int64
	var evalF func()
	if err = proto.Unmarshal(msg.Body, request); err == nil {
		count, evalF, err = handler.ValidateCircuits(request.RouterFilter, func(detail *mgmt_pb.RouterCircuitDetails) {
			if !ch.IsClosed() {
				if sendErr := protobufs.MarshalTyped(detail).WithTimeout(15 * time.Second).SendAndWaitForWire(ch); sendErr != nil {
					log.WithError(sendErr).Error("send of router circuit details failed, closing channel")
					if closeErr := ch.Close(); closeErr != nil {
						log.WithError(closeErr).Error("failed to close channel")
					}
				}
			} else {
				log.Info("channel closed, unable to send router egge connections detail")
			}
		})
	} else {
		log.WithError(err).Error("failed to unmarshal request")
		return
	}

	response := &mgmt_pb.ValidateCircuitsResponse{
		Success:     err == nil,
		RouterCount: uint64(count),
	}

	if err != nil {
		response.Message = fmt.Sprintf("%v: failed to unmarshall request: %v", handler.getNetwork().GetAppId(), err)
	}

	if err = protobufs.MarshalTyped(response).ReplyTo(msg).WithTimeout(5 * time.Second).Send(ch); err != nil {
		pfxlog.Logger().WithError(err).Error("unexpected error sending response")
	}

	if evalF != nil {
		evalF()
	}
}

type CircuitValidationCallback func(detail *mgmt_pb.RouterCircuitDetails)

func (handler *validateCircuitsHandler) ValidateCircuits(filter string, cb CircuitValidationCallback) (int64, func(), error) {
	result, err := handler.appEnv.Managers.Router.BaseList(filter)
	if err != nil {
		return 0, nil, err
	}

	sem := concurrenz.NewSemaphore(10)

	evalF := func() {
		for _, router := range result.Entities {
			connectedRouter := handler.appEnv.GetHostController().GetNetwork().GetConnectedRouter(router.Id)
			if connectedRouter != nil {
				router = connectedRouter
			}
			sem.Acquire()
			go func() {
				defer sem.Release()
				if connectedRouter == nil {
					handler.validRouterCircuits(router, cb)
					return
				}

				supportsInspect, err := connectedRouter.VersionInfo.HasMinimumVersion("1.6.6")
				if err != nil {
					handler.reportError(connectedRouter, err, cb)
				} else if supportsInspect {
					handler.validRouterCircuits(router, cb)
				} else {
					// if the router doesn't support inspection, just report success
					cb(&mgmt_pb.RouterCircuitDetails{
						RouterId:        router.Id,
						RouterName:      router.Name,
						ValidateSuccess: true,
						Details:         map[string]*mgmt_pb.RouterCircuitDetail{},
					})
				}
			}()
		}
	}

	count := int64(len(result.Entities))
	return count, evalF, nil
}

func (handler *validateCircuitsHandler) validRouterCircuits(router *model.Router, cb CircuitValidationCallback) {
	var forwarderCircuits *inspect.ForwarderCircuits
	var edgeListenerCircuits *inspect.EdgeListenerCircuits
	var sdkCircuits *inspect.SdkCircuits

	if router.Control != nil && !router.Control.IsClosed() {
		request := &ctrl_pb.InspectRequest{
			RequestedValues: []string{inspect.RouterCircuitsKey, inspect.RouterEdgeCircuitsKey, inspect.RouterSdkCircuitsKey},
		}
		resp := &ctrl_pb.InspectResponse{}
		respMsg, err := protobufs.MarshalTyped(request).WithTimeout(time.Minute).SendForReply(router.Control)
		if err = protobufs.TypedResponse(resp).Unmarshall(respMsg, err); err != nil {
			handler.reportError(router, err, cb)
			return
		}

		for _, val := range resp.Values {
			if val.Name == inspect.RouterCircuitsKey {
				if err = json.Unmarshal([]byte(val.Value), &forwarderCircuits); err != nil {
					handler.reportError(router, err, cb)
					return
				}
			} else if val.Name == inspect.RouterEdgeCircuitsKey {
				if err = json.Unmarshal([]byte(val.Value), &edgeListenerCircuits); err != nil {
					handler.reportError(router, err, cb)
					return
				}
			} else if val.Name == inspect.RouterSdkCircuitsKey {
				if err = json.Unmarshal([]byte(val.Value), &sdkCircuits); err != nil {
					handler.reportError(router, err, cb)
					return
				}
			}
		}

		if forwarderCircuits == nil {
			if len(resp.Errors) > 0 {
				err = errors.New(strings.Join(resp.Errors, ","))
				handler.reportError(router, err, cb)
				return
			}
			handler.reportError(router, errors.New("no identity in connection details returned from router"), cb)
			return
		}
	} else {
		handler.reportError(router, fmt.Errorf("router %s is not connected", router.Id), cb)
		return
	}

	details := &mgmt_pb.RouterCircuitDetails{
		RouterId:        router.Id,
		RouterName:      router.Name,
		ValidateSuccess: true,
		Details:         map[string]*mgmt_pb.RouterCircuitDetail{},
	}

	circuitMgr := handler.appEnv.Managers.Circuit

	for _, circuit := range circuitMgr.All() {
		routerRelevant := false
		for _, pathRouter := range circuit.Path.Nodes {
			if pathRouter.Id == router.Id {
				routerRelevant = true
				break
			}
		}

		if !routerRelevant {
			continue
		}

		detail := &mgmt_pb.RouterCircuitDetail{
			CircuitId:          circuit.Id,
			MissingInCtrl:      false,
			MissingInForwarder: true,
		}

		if fwdDetails, inForwarder := forwarderCircuits.Circuits[circuit.Id]; inForwarder {
			detail.MissingInForwarder = false
			detail.Destinations = fwdDetails.Destinations

			for _, v := range fwdDetails.Destinations {
				if v == "xg-edge-fwd" {
					detail.MissingInEdge = true
					detail.MissingInSdk = true
				}
			}
		}

		details.Details[circuit.Id] = detail
	}

	for circuitId, fwdDetail := range forwarderCircuits.Circuits {
		if fwdDetail.CtrlId != handler.appEnv.GetId() {
			continue
		}

		detail := details.Details[circuitId]
		if detail == nil {
			detail = &mgmt_pb.RouterCircuitDetail{
				CircuitId:     circuitId,
				MissingInCtrl: true,
			}
			details.Details[circuitId] = detail
		}
	}

	for _, inspectDetail := range edgeListenerCircuits.Circuits {
		if inspectDetail.CtrlId != handler.appEnv.GetId() {
			continue
		}

		detail := details.Details[inspectDetail.CircuitId]
		if detail == nil {
			detail = &mgmt_pb.RouterCircuitDetail{
				CircuitId:          inspectDetail.CircuitId,
				MissingInCtrl:      true,
				MissingInForwarder: true,
			}
			details.Details[inspectDetail.CircuitId] = detail
		} else {
			detail.MissingInEdge = false
		}
	}

	for _, inspectDetail := range sdkCircuits.Circuits {
		if inspectDetail.CtrlId != handler.appEnv.GetId() {
			continue
		}

		detail := details.Details[inspectDetail.CircuitId]
		if detail == nil {
			detail = &mgmt_pb.RouterCircuitDetail{
				CircuitId:          inspectDetail.CircuitId,
				MissingInCtrl:      true,
				MissingInForwarder: true,
				MissingInEdge:      true,
			}
			details.Details[inspectDetail.CircuitId] = detail
		} else {
			detail.MissingInSdk = false
		}
	}

	cb(details)
}

func (handler *validateCircuitsHandler) reportError(router *model.Router, err error, cb CircuitValidationCallback) {
	result := &mgmt_pb.RouterCircuitDetails{
		RouterId:        router.Id,
		RouterName:      router.Name,
		ValidateSuccess: false,
		Message:         err.Error(),
	}
	cb(result)
}
