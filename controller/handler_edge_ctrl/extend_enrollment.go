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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/models"
	"google.golang.org/protobuf/proto"
)

func newRouterChangeContext(router interface {
	models.Named
	GetId() string
}, ch channel.Channel) *change.Context {
	return change.New().SetSourceType(change.SourceTypeControlChannel).
		SetSourceMethod("extend.router.enrollment").
		SetSourceLocal(ch.Underlay().GetLocalAddr().String()).
		SetSourceRemote(ch.Underlay().GetRemoteAddr().String()).
		SetChangeAuthorType(change.AuthorTypeRouter).
		SetChangeAuthorId(router.GetId()).
		SetChangeAuthorName(router.GetName())
}

type ExtendEnrollmentHandler struct {
	appEnv *env.AppEnv
}

func NewExtendEnrollmentHandler(appEnv *env.AppEnv) *ExtendEnrollmentHandler {
	return &ExtendEnrollmentHandler{
		appEnv: appEnv,
	}
}

func (h *ExtendEnrollmentHandler) ContentType() int32 {
	return env.EnrollmentExtendRouterRequestType
}

func (h *ExtendEnrollmentHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
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

			var clientPem string
			var serverPem string
			var newCerts *model.ExtendedCerts

			if router, _ := h.appEnv.Managers.EdgeRouter.ReadOneByFingerprint(fingerprint); router != nil {
				changeCtx := newRouterChangeContext(router, ch)

				log = log.WithFields(map[string]interface{}{
					"routerId":   router.Id,
					"routerName": router.Name,
				})

				if req.RequireVerification {
					newCerts, err = h.appEnv.Managers.EdgeRouter.ExtendEnrollmentWithVerify(router, []byte(req.ClientCertCsr), []byte(req.ServerCertCsr), changeCtx)
				} else {
					newCerts, err = h.appEnv.Managers.EdgeRouter.ExtendEnrollment(router, []byte(req.ClientCertCsr), []byte(req.ServerCertCsr), changeCtx)
				}

				if err != nil {
					log.WithError(err).Error("request to extend edge router enrollment failed")
					return
				}

			} else if router, _ := h.appEnv.Managers.TransitRouter.ReadOneByFingerprint(fingerprint); router != nil {
				changeCtx := newRouterChangeContext(router, ch)

				if req.RequireVerification {
					newCerts, err = h.appEnv.Managers.TransitRouter.ExtendEnrollmentWithVerify(router, []byte(req.ClientCertCsr), []byte(req.ServerCertCsr), changeCtx)
				} else {
					newCerts, err = h.appEnv.Managers.TransitRouter.ExtendEnrollment(router, []byte(req.ClientCertCsr), []byte(req.ServerCertCsr), changeCtx)
				}

				if err != nil {
					log.WithError(err).Error("request to extend router enrollment failed")
					return
				}
			} else {
				log.WithError(err).Errorf("request to extend route enrollment failed, router not found by fingerprint")
				return
			}

			clientPem, err := h.appEnv.GetManagers().Enrollment.GetCertChainPem(newCerts.RawClientCert)

			if err != nil {
				log.WithError(err).Error("request to extend router enrollment failed to marshal client certificate to PEM format")
				return
			}
			serverPem, err = h.appEnv.GetManagers().Enrollment.GetCertChainPem(newCerts.RawServerCert)
			if err != nil {
				log.WithError(err).Error("request to extend router enrollment failed to marshal server certificate to PEM format")
				return
			}

			data := &edge_ctrl_pb.EnrollmentCertsResponse{
				ClientCertPem: clientPem,
				ServerCertPem: serverPem,
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
