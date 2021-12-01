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
	"crypto/sha1"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/identity/identity"
	nfpem "github.com/openziti/foundation/util/pem"
	"time"
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
		enrollmentCerts := &edge_ctrl_pb.EnrollmentCertsResponse{}
		if err := proto.Unmarshal(msg.Body, enrollmentCerts); err == nil {

			if enrollmentCerts.ClientCertPem == "" {
				pfxlog.Logger().Error("expected enrollment certs response to contain a client cert")
				return
			}

			if enrollmentCerts.ServerCertPem == "" {
				pfxlog.Logger().Error("expected enrollment certs response to contain a server cert")
				return
			}

			certs := nfpem.PemToX509(enrollmentCerts.ClientCertPem)

			if len(certs) == 0 {
				pfxlog.Logger().Error("could not parse client certificate during enrollment extension")
				return
			}

			if err != nil {
				pfxlog.Logger().Errorf("error during enrollment extension, could not sign client cert: %v", err)
				return
			}

			verifyRequest := &edge_ctrl_pb.EnrollmentExtendRouterVerifyRequest{
				ClientCertPem: enrollmentCerts.ClientCertPem,
			}
			reply := &edge_ctrl_pb.Error{}
			err := ch.SendForReplyAndDecode(verifyRequest, 30*time.Second, reply)

			if err != nil {
				pfxlog.Logger().Errorf("error during enrollment extension, verification reply produced an error: %v", err)
				return
			}

			if reply.Code != "" {
				pfxlog.Logger().Errorf("error during enrollment extension, verification resulted in an error: %s - %s", reply.Code, reply.Message)
				return
			}

			if err := h.id.SetCert(enrollmentCerts.ClientCertPem); err != nil {
				pfxlog.Logger().Errorf("enrollment certs could not set client pem: %v", err)
			}

			if err := h.id.SetServerCert(enrollmentCerts.ServerCertPem); err != nil {
				pfxlog.Logger().Errorf("enrollment certs could not set server pem: %v", err)
			}

			if err := h.id.Reload(); err == nil {
				h.notifyCertUpdate()
			} else {
				pfxlog.Logger().Errorf("could not reload new enrollment certs, please manually restart the router: %v", err)
			}

			fingerprint := fmt.Sprintf("%x", sha1.Sum(h.id.Cert().Certificate[0]))

			pfxlog.Logger().Infof("enrollment extension done, new client certificate fingerprint (%s)", fingerprint)
		} else {
			pfxlog.Logger().Panic("could not convert message as enrollment certs response")
		}
	}()
}
