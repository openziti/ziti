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

package handler_edge_ctrl

import (
	"bytes"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	nfpem "github.com/openziti/foundation/v2/pem"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/controller/env"
	"google.golang.org/protobuf/proto"
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

func (h *extendEnrollmentVerifyHandler) respond(respErr *edge_ctrl_pb.Error, msg *channel.Message, ch channel.Channel) {
	respErrBody, _ := proto.Marshal(respErr)
	respMsg := channel.NewMessage(int32(edge_ctrl_pb.ContentType_ErrorType), respErrBody)
	respMsg.ReplyTo(msg)

	if err := ch.Send(respMsg); err != nil {
		pfxlog.Logger().Errorf("could not send enrollment verification response, channel error: %v", err)
	}
}
func (h *extendEnrollmentVerifyHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
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

		if len(verifyMsg.ClientCertPem) == 0 {
			h.respond(&edge_ctrl_pb.Error{
				Code:    "MISSING_CLIENT_PEM",
				Message: "request did not contain a client certificate PEM",
			}, msg, ch)
			return
		}

		routerId := ch.Id()
		edgeRouter, _ := h.appEnv.Managers.EdgeRouter.Read(routerId)

		if edgeRouter != nil {
			if edgeRouter.UnverifiedCertPem == nil {
				h.respond(&edge_ctrl_pb.Error{
					Code:    "INVALID_ROUTER_STATE",
					Message: "request is not possible as the router has no outstanding enrollment requests",
				}, msg, ch)
				return
			}

			submittedCerts := nfpem.PemStringToCertificates(verifyMsg.ClientCertPem)

			if len(submittedCerts) == 0 {
				h.respond(&edge_ctrl_pb.Error{
					Code:    "INVALID_CLIENT_PEM",
					Message: "request did not contain a parsable PEM certificate",
				}, msg, ch)
				return
			}

			targetCerts := nfpem.PemStringToCertificates(*edgeRouter.UnverifiedCertPem)

			if len(targetCerts) == 0 {
				h.respond(&edge_ctrl_pb.Error{
					Code:    "INVALID_CLIENT_PEM",
					Message: "internal error, target PEM did not parse to a certificate",
				}, msg, ch)
				return
			}

			if !bytes.Equal(targetCerts[0].Raw, submittedCerts[0].Raw) {
				h.respond(&edge_ctrl_pb.Error{
					Code:    "INVALID_CLIENT_PEM",
					Message: "request did not contain a matching client pem",
				}, msg, ch)
				return
			}

			changeCtx := newRouterChangeContext(edgeRouter, ch)
			err := h.appEnv.Managers.EdgeRouter.ExtendEnrollmentVerify(edgeRouter, changeCtx)
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

		router, _ := h.appEnv.Managers.TransitRouter.Read(routerId)

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

			changeCtx := newRouterChangeContext(router, ch)
			err := h.appEnv.Managers.TransitRouter.ExtendEnrollmentVerify(router, changeCtx)
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
