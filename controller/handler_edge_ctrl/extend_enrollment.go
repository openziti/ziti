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
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/internal/cert"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/openziti/foundation/channel2"
)

type extendEnrollmentHandler struct {
	appEnv *env.AppEnv
}

func NewextendEnrollmentHandler(appEnv *env.AppEnv) *extendEnrollmentHandler {
	return &extendEnrollmentHandler{
		appEnv: appEnv,
	}
}

func (h *extendEnrollmentHandler) ContentType() int32 {
	return env.EnrollmentExtendRouterRequestType
}

func (h *extendEnrollmentHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	go func() {
		req := &edge_ctrl_pb.EnrollmentExtendRouterRequest{}
		certs := ch.Underlay().Certificates()

		if err := proto.Unmarshal(msg.Body, req); err == nil {
			fingerprint := ""

			if len(certs) > 0 {
				cert := certs[0]
				fingerprint = h.appEnv.FingerprintGenerator.FromCert(cert)
			} else {
				pfxlog.Logger().Errorf("request to extend the enrollment without certificate on channel [%s - %s]", fingerprint, ch.LogicalName())
			}

			var clientPem []byte
			var serverPem []byte
			var newCerts *model.ExtendedCerts
			routerId := ""
			routerName := ""

			if router, _ := h.appEnv.Handlers.EdgeRouter.ReadOneByFingerprint(fingerprint); router != nil {
				routerId = router.Id
				routerName = router.Name

				newCerts, err = h.appEnv.Handlers.EdgeRouter.ExtendEnrollment(router, []byte(req.ClientCertCsr), []byte(req.ServerCertCsr))

				if err != nil {
					pfxlog.Logger().Errorf("request to extend the enrollment for an edge router [%s - %s] errored: %s", routerId, routerName, err)
					return
				}

			} else if router, _ := h.appEnv.Handlers.TransitRouter.ReadOneByFingerprint(fingerprint); router != nil {
				routerId = router.Id
				routerName = router.Name

				newCerts, err = h.appEnv.Handlers.TransitRouter.ExtendEnrollment(router, []byte(req.ClientCertCsr), []byte(req.ServerCertCsr))

				if err != nil {
					pfxlog.Logger().Errorf("request to extend the enrollment for a router [%s - %s] errored: %s", routerId, routerName, err)
					return
				}
			} else {
				pfxlog.Logger().Errorf("request to extend the enrollment for a router that was not found by fingerprint [%s] on channel [%s]", fingerprint, ch.LogicalName())
				return
			}

			clientPem, err = cert.RawToPem(newCerts.RawClientCert)

			if err != nil {
				pfxlog.Logger().Errorf("request to extend the enrollment for a router [%s - %s] errored encoding client raw to pem: %s", routerId, routerName, err)
				return
			}
			serverPem, err = cert.RawToPem(newCerts.RawServerCert)
			if err != nil {
				pfxlog.Logger().Errorf("request to extend the enrollment for a router [%s - %s] errored encoding server raw to pem: %s", routerId, routerName, err)
				return
			}

			data := &edge_ctrl_pb.EnrollmentCertsResponse{
				ClientCertPem: string(clientPem),
				ServerCertPem: string(serverPem),
			}

			body, err := proto.Marshal(data)

			if err != nil {
				pfxlog.Logger().Errorf("request to extend the enrollment for a router [%s - %s] errored marshaling message: %s", routerId, routerName, err)
				return
			}

			msg := channel2.NewMessage(env.EnrollmentCertsResponseType, body)

			ch.Send(msg)

			pfxlog.Logger().Infof("request to extend the enrollment for a router [%s - %s] sent", routerId, routerName)

		} else {
			pfxlog.Logger().Error("could not convert message as enroll extend")
		}
	}()
}
