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
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/identity/identity"
)

type extendEnrollmentCertsHandler struct {
	id               *identity.TokenId
	notifyCertUpdate func()
}

func NewExtendEnrollmentCertsHandler(id *identity.TokenId, notifyCertUpdate func()) *extendEnrollmentCertsHandler {
	return &extendEnrollmentCertsHandler{
		id:               id,
		notifyCertUpdate: notifyCertUpdate,
	}
}

func (h *extendEnrollmentCertsHandler) ContentType() int32 {
	return env.EnrollmentCertsResponseType
}

func (h *extendEnrollmentCertsHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	go func() {
		req := &edge_ctrl_pb.EnrollmentCertsResponse{}
		if err := proto.Unmarshal(msg.Body, req); err == nil {

			if req.ClientCertPem == "" {
				pfxlog.Logger().Error("expected enrollment certs response to contain a client cert")
				return
			}

			if req.ServerCertPem == "" {
				pfxlog.Logger().Error("expected enrollment certs response to contain a server cert")
				return
			}

			if err := h.id.SetCert(req.ClientCertPem); err != nil {
				pfxlog.Logger().Errorf("enrollment certs could not set client pem: %v", err)
			}

			if err := h.id.SetServerCert(req.ServerCertPem); err != nil {
				pfxlog.Logger().Errorf("enrollment certs could not set server pem: %v", err)
			}

			if err := h.id.Reload(); err == nil {
				h.notifyCertUpdate()
			} else {
				pfxlog.Logger().Errorf("could not reload new enrollment certs, please manually restart the router: %v", err)
			}
			pfxlog.Logger().Info("enrollment extension done")
		} else {
			pfxlog.Logger().Panic("could not convert message as enrollment certs response")
		}
	}()
}
