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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/internal/cert"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"google.golang.org/protobuf/proto"
)

type extendEnrollmentHandler struct {
	appEnv *env.AppEnv
}

func NewExtendEnrollmentHandler(appEnv *env.AppEnv) *extendEnrollmentHandler {
	return &extendEnrollmentHandler{
		appEnv: appEnv,
	}
}

func (h *extendEnrollmentHandler) ContentType() int32 {
	return env.EnrollmentExtendRouterRequestType
}

func (h *extendEnrollmentHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	go func() {
		req := &edge_ctrl_pb.EnrollmentExtendRouterRequest{}
		certs := ch.Underlay().Certificates()

		fingerprint := "none"

		if len(certs) > 0 {
			fingerprint = h.appEnv.FingerprintGenerator.FromCert(certs[0])
		}

		log := pfxlog.Logger().WithFields(map[string]interface{}{
			"channel":     ch.LogicalName(),
			"fingerprint": fingerprint,
		})

		if fingerprint == "" || fingerprint == "none" {
			log.Errorf("request to extend the enrollment without certificate")
			return
		}

		if err := proto.Unmarshal(msg.Body, req); err == nil {

			var clientPem []byte
			var serverPem []byte
			var newCerts *model.ExtendedCerts

			if router, _ := h.appEnv.Managers.EdgeRouter.ReadOneByFingerprint(fingerprint); router != nil {

				log = log.WithFields(map[string]interface{}{
					"routerId":   router.Id,
					"routerName": router.Name,
				})

				if req.RequireVerification {
					newCerts, err = h.appEnv.Managers.EdgeRouter.ExtendEnrollmentWithVerify(router, []byte(req.ClientCertCsr), []byte(req.ServerCertCsr))
				} else {
					newCerts, err = h.appEnv.Managers.EdgeRouter.ExtendEnrollment(router, []byte(req.ClientCertCsr), []byte(req.ServerCertCsr))
				}

				if err != nil {
					log.WithError(err).Error("request to extend edge router enrollment failed")
					return
				}

			} else if router, _ := h.appEnv.Managers.TransitRouter.ReadOneByFingerprint(fingerprint); router != nil {

				if req.RequireVerification {
					newCerts, err = h.appEnv.Managers.TransitRouter.ExtendEnrollmentWithVerify(router, []byte(req.ClientCertCsr), []byte(req.ServerCertCsr))
				} else {
					newCerts, err = h.appEnv.Managers.TransitRouter.ExtendEnrollment(router, []byte(req.ClientCertCsr), []byte(req.ServerCertCsr))
				}

				if err != nil {
					log.WithError(err).Error("request to extend router enrollment failed")
					return
				}
			} else {
				log.WithError(err).Errorf("request to extend route enrollment failed, router not found by fingerprint")
				return
			}

			clientPem, err = cert.RawToPem(newCerts.RawClientCert)

			if err != nil {
				log.WithError(err).Error("request to extend router enrollment failed to marshal client certificate to PEM format")
				return
			}
			serverPem, err = cert.RawToPem(newCerts.RawServerCert)
			if err != nil {
				log.WithError(err).Error("request to extend router enrollment failed to marshal server certificate to PEM format")
				return
			}

			data := &edge_ctrl_pb.EnrollmentCertsResponse{
				ClientCertPem: string(clientPem),
				ServerCertPem: string(serverPem),
			}

			body, err := proto.Marshal(data)

			if err != nil {
				log.WithError(err).Error("request to extend router enrollment failed to marshal enrollment certificate response message")
				return
			}

			msg := channel.NewMessage(env.EnrollmentCertsResponseType, body)

			if err := ch.Send(msg); err != nil {
				log.WithError(err).Errorf("request to extend router enrollment failed to send enrollment certificate response")
				return
			}

			log.Infof("enrollment certificate response sent")

		} else {
			log.Error("could not convert message as enroll extend")
		}
	}()
}
