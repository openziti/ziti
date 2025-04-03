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
	"sync"
	"time"
)

type validateIdentityConnectionStatusesHandler struct {
	appEnv *env.AppEnv
}

func newValidateIdentityConnectionStatusesHandler(appEnv *env.AppEnv) channel.TypedReceiveHandler {
	return &validateIdentityConnectionStatusesHandler{appEnv: appEnv}
}

func (*validateIdentityConnectionStatusesHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_ValidateIdentityConnectionStatusesRequestType)
}

func (handler *validateIdentityConnectionStatusesHandler) getNetwork() *network.Network {
	return handler.appEnv.GetHostController().GetNetwork()
}

func (handler *validateIdentityConnectionStatusesHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label())
	request := &mgmt_pb.ValidateIdentityConnectionStatusesRequest{}

	var err error

	var count int64
	var evalF func()
	if err = proto.Unmarshal(msg.Body, request); err == nil {
		count, evalF, err = handler.ValidateEdgeConnections(request.RouterFilter, func(detail *mgmt_pb.RouterIdentityConnectionStatusesDetails) {
			if !ch.IsClosed() {
				if sendErr := protobufs.MarshalTyped(detail).WithTimeout(15 * time.Second).SendAndWaitForWire(ch); sendErr != nil {
					log.WithError(sendErr).Error("send of router edge connections detail failed, closing channel")
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

	response := &mgmt_pb.ValidateIdentityConnectionStatusesResponse{
		Success:        err == nil,
		ComponentCount: uint64(count),
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

type EdgeConnectionsValidationCallback func(detail *mgmt_pb.RouterIdentityConnectionStatusesDetails)

func (handler *validateIdentityConnectionStatusesHandler) ValidateEdgeConnections(filter string, cb EdgeConnectionsValidationCallback) (int64, func(), error) {
	result, err := handler.appEnv.Managers.Router.BaseList(filter)
	if err != nil {
		return 0, nil, err
	}

	sem := concurrenz.NewSemaphore(10)

	m := handler.appEnv.Managers.Identity.GetIdentityStatusMapCopy()
	lock := &sync.Mutex{}

	evalF := func() {
		for _, router := range result.Entities {
			connectedRouter := handler.appEnv.GetHostController().GetNetwork().GetConnectedRouter(router.Id)
			if connectedRouter != nil {
				router = connectedRouter
			}
			sem.Acquire()
			go func() {
				defer sem.Release()
				handler.validRouterEdgeConnections(router, m, lock, cb)
			}()
		}
	}

	count := int64(len(result.Entities))
	return count, evalF, nil
}

func (handler *validateIdentityConnectionStatusesHandler) validRouterEdgeConnections(
	router *model.Router,
	m map[string]map[string]channel.Channel,
	lock *sync.Mutex,
	cb EdgeConnectionsValidationCallback) {

	var identityConnections *inspect.RouterIdentityConnections

	if router.Control != nil && !router.Control.IsClosed() {
		request := &ctrl_pb.InspectRequest{RequestedValues: []string{inspect.RouterIdentityConnectionStatusesKey}}
		resp := &ctrl_pb.InspectResponse{}
		respMsg, err := protobufs.MarshalTyped(request).WithTimeout(time.Minute).SendForReply(router.Control)
		if err = protobufs.TypedResponse(resp).Unmarshall(respMsg, err); err != nil {
			handler.reportError(router, err, cb)
			return
		}

		for _, val := range resp.Values {
			if val.Name == inspect.RouterIdentityConnectionStatusesKey {
				if err = json.Unmarshal([]byte(val.Value), &identityConnections); err != nil {
					handler.reportError(router, err, cb)
					return
				}
			}
		}

		if identityConnections == nil {
			if len(resp.Errors) > 0 {
				err = errors.New(strings.Join(resp.Errors, ","))
				handler.reportError(router, err, cb)
				return
			}
			handler.reportError(router, errors.New("no identity in connection details returned from router"), cb)
			return
		}
	} else {
		identityConnections = &inspect.RouterIdentityConnections{}
	}

	var errList []string

	for identityId, detail := range identityConnections.IdentityConnections {
		isConnected := false
		for _, conn := range detail.Connections {
			if !conn.Closed {
				isConnected = true
			}
		}

		lock.Lock()
		connMap := m[identityId]
		routerConn := connMap[router.Id]
		delete(connMap, router.Id)
		if len(connMap) == 0 {
			delete(m, identityId)
		}
		lock.Unlock()

		if isConnected {
			if routerConn == nil || routerConn.IsClosed() {
				errList = append(errList, fmt.Sprintf("router reports identity %s connected, but controller disagrees", identityId))
			}
		} else {
			if routerConn != nil && !routerConn.IsClosed() {
				errList = append(errList, fmt.Sprintf("router reports identity %s is not connected, but controller disagrees", identityId))
			}
		}

		if detail.UnreportedCount > 0 {
			errList = append(errList, fmt.Sprintf("identity %s has unreported events", identityId))
		}

		if detail.BeingReportedCount > 0 {
			errList = append(errList, fmt.Sprintf("identity %s has events still being reported", identityId))
		}

		if detail.UnreportedStateChanged {
			errList = append(errList, fmt.Sprintf("identity %s has unreported state change", identityId))
		}

		if detail.BeingReportedStateChanged {
			errList = append(errList, fmt.Sprintf("identity %s has state change being reported", identityId))
		}
	}

	lock.Lock()
	for identityId, connMap := range m {
		if routerConn := connMap[router.Id]; routerConn != nil {
			if !routerConn.IsClosed() {
				errList = append(errList, fmt.Sprintf("ctrl reports identity %s is connected, but router disagrees", identityId))
			} else if routerConn.GetTimeSinceLastRead() > handler.appEnv.GetConfig().Edge.IdentityStatusConfig.UnknownTimeout {
				errList = append(errList, fmt.Sprintf("ctrl still has identity %s router conn, even though time since last read %s > timeout of %s",
					identityId, routerConn.GetTimeSinceLastRead(), handler.appEnv.GetConfig().Edge.IdentityStatusConfig.UnknownTimeout))
			}
		}
		delete(connMap, router.Id)
		if len(connMap) == 0 {
			delete(m, identityId)
		}
	}
	lock.Unlock()

	details := &mgmt_pb.RouterIdentityConnectionStatusesDetails{
		ComponentType:   "router",
		ComponentId:     router.Id,
		ComponentName:   router.Name,
		ValidateSuccess: true,
		Errors:          errList,
	}
	cb(details)
}

func (handler *validateIdentityConnectionStatusesHandler) reportError(router *model.Router, err error, cb EdgeConnectionsValidationCallback) {
	result := &mgmt_pb.RouterIdentityConnectionStatusesDetails{
		ComponentId:     router.Id,
		ComponentName:   router.Name,
		ValidateSuccess: false,
		Errors:          []string{err.Error()},
	}
	cb(result)
}
