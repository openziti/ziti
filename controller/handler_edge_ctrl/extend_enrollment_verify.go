/*
	Copyright NetFoundry, Inc.

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

package handler_edge_ctrl

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/openziti/foundation/channel2"
)

type extendEnrollmentVerifyHandler struct {
	appEnv *env.AppEnv
}

func NewExtendEnrollmentVerifyHandler(appEnv *env.AppEnv) *extendEnrollmentVerifyHandler {
	return &extendEnrollmentVerifyHandler{
		appEnv: appEnv,
	}
}

func (h *extendEnrollmentVerifyHandler) ContentType() int32 {
	return env.EnrollmentExtendRouterVerifyRequestType
}

func (h *extendEnrollmentVerifyHandler) respond(respErr *edge_ctrl_pb.Error, msg *channel2.Message, ch channel2.Channel) {
	respErrBody, _ := proto.Marshal(respErr)
	respMsg := channel2.NewMessage(int32(edge_ctrl_pb.ContentType_ErrorType), respErrBody)
	respMsg.ReplyTo(msg)

	if err := ch.Send(respMsg); err != nil {
		pfxlog.Logger().Errorf("could not send enrollment verification response, channel error: %v", err)
	}
}
func (h *extendEnrollmentVerifyHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	go func() {

		verifyMsg := &edge_ctrl_pb.EnrollmentExtendRouterVerifyRequest{}
		err := proto.Unmarshal(msg.Body, verifyMsg)

		if err != nil {
			h.respond(&edge_ctrl_pb.Error{
				Code:    "COULD_NOT_UNMARSHAL",
				Message: fmt.Sprintf("request could not be unmarhsalled: %v", err),
			}, msg, ch)
			return
		}

		if verifyMsg == nil || len(verifyMsg.ClientCertPem) == 0 {
			h.respond(&edge_ctrl_pb.Error{
				Code:    "MISSING_CLIENT_PEM",
				Message: "request did not contain a client certificate PEM",
			}, msg, ch)
			return
		}

		routerId := ch.Id().Token
		edgeRouter, _ := h.appEnv.Handlers.EdgeRouter.Read(routerId)

		if edgeRouter != nil {
			if edgeRouter.UnverifiedCertPem == nil {
				h.respond(&edge_ctrl_pb.Error{
					Code:    "INVALID_ROUTER_STATE",
					Message: "request is not possible as the router has no outstanding enrollment requests",
				}, msg, ch)
				return
			}

			if *edgeRouter.UnverifiedCertPem != verifyMsg.ClientCertPem {
				h.respond(&edge_ctrl_pb.Error{
					Code:    "INVALID_CLIENT_PEM",
					Message: "request did not contain a matching client pem",
				}, msg, ch)
				return
			}

			err := h.appEnv.Handlers.EdgeRouter.ExtendEnrollmentVerify(edgeRouter)
			if err != nil {
				h.respond(&edge_ctrl_pb.Error{
					Code:    "VERIFY_ERROR",
					Message: fmt.Sprintf("enrollment extension error: %v", err),
				}, msg, ch)
				return
			}

			h.respond(&edge_ctrl_pb.Error{}, msg, ch)
			return
		}

		router, _ := h.appEnv.Handlers.TransitRouter.Read(routerId)

		if router != nil {
			if router.UnverifiedCertPem == nil {
				h.respond(&edge_ctrl_pb.Error{
					Code:    "INVALID_ROUTER_STATE",
					Message: "not possible as the router has no outstanding enrollment requests",
				}, msg, ch)
				return
			}

			if *router.UnverifiedCertPem != verifyMsg.ClientCertPem {
				h.respond(&edge_ctrl_pb.Error{
					Code:    "INVALID_CLIENT_PEM",
					Message: "request did not contain a matching client pem",
				}, msg, ch)
				return
			}

			err := h.appEnv.Handlers.TransitRouter.ExtendEnrollmentVerify(router)
			if err != nil {
				h.respond(&edge_ctrl_pb.Error{
					Code:    "VERIFY_ERROR",
					Message: err.Error(),
				}, msg, ch)
				return
			}

			h.respond(&edge_ctrl_pb.Error{}, msg, ch)
			return
		}

		h.respond(&edge_ctrl_pb.Error{
			Code:    "ROUTER_NOT_FOUND",
			Message: fmt.Sprintf("router with id %s not found", routerId),
		}, msg, ch)
	}()
}
