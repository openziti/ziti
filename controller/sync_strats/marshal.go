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

package sync_strats

import (
	"fmt"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"time"
)

func apiSessionToProto(ae *env.AppEnv, token, identityId, apiSessionId string) (*edge_ctrl_pb.ApiSession, error) {
	fingerprints, err := getFingerprints(ae, identityId, apiSessionId)
	if err != nil {
		return nil, err
	}

	return &edge_ctrl_pb.ApiSession{
		Token:            token,
		CertFingerprints: fingerprints,
		Id:               apiSessionId,
	}, nil
}

func sessionToProto(ae *env.AppEnv, session *persistence.Session) (*edge_ctrl_pb.Session, error) {
	service, err := ae.Handlers.EdgeService.Read(session.ServiceId)
	if err != nil {
		return nil, fmt.Errorf("could not convert to session proto, could not find service: %s", err)
	}

	fps, err := getFingerprintsByApiSessionId(ae, session.ApiSessionId)

	if err != nil {
		return nil, fmt.Errorf("could not get fingerprints for network session: %s", err)
	}

	serviceProto := &edge_ctrl_pb.Service{
		Name:               service.Name,
		Id:                 service.Id,
		EncryptionRequired: service.EncryptionRequired,
	}

	sessionType := edge_ctrl_pb.SessionType_Dial
	if session.Type == persistence.SessionTypeBind {
		sessionType = edge_ctrl_pb.SessionType_Bind
	}

	return &edge_ctrl_pb.Session{
		Id:               session.Id,
		Token:            session.Token,
		Service:          serviceProto,
		CertFingerprints: fps,
		Type:             sessionType,
		ApiSessionId:     session.ApiSessionId,
	}, nil
}

func getFingerprintsByApiSessionId(ae *env.AppEnv, apiSessionId string) ([]string, error) {
	apiSession, err := ae.GetHandlers().ApiSession.Read(apiSessionId)

	if err != nil {
		return nil, fmt.Errorf("could not query fingerprints by api session id [%s]: %s", apiSessionId, err)
	}

	return getFingerprints(ae, apiSession.IdentityId, apiSessionId)
}

func getFingerprints(ae *env.AppEnv, identityId, apiSessionId string) ([]string, error) {
	identityPrints, err := getIdentityAuthenticatorFingerprints(ae, identityId)

	if err != nil {
		return nil, err
	}

	apiSessionPrints, err := getApiSessionCertificateFingerprints(ae, apiSessionId)

	if err != nil {
		return nil, err
	}

	for _, apiSessionPrint := range apiSessionPrints {
		identityPrints = append(identityPrints, apiSessionPrint)
	}

	return identityPrints, nil
}

func getIdentityAuthenticatorFingerprints(ae *env.AppEnv, identityId string) ([]string, error) {
	fingerprintsMap := map[string]struct{}{}

	err := ae.Handlers.Identity.CollectAuthenticators(identityId, func(authenticator *model.Authenticator) error {
		for _, authPrint := range authenticator.Fingerprints() {
			fingerprintsMap[authPrint] = struct{}{}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	var fingerprints []string
	for fingerprint := range fingerprintsMap {
		fingerprints = append(fingerprints, fingerprint)
	}

	return fingerprints, nil
}

func getApiSessionCertificateFingerprints(ae *env.AppEnv, apiSessionId string) ([]string, error) {
	apiSessionCerts, err := ae.GetHandlers().ApiSessionCertificate.ReadByApiSessionId(apiSessionId)

	if err != nil {
		return nil, err
	}

	var validPrints []string

	now := time.Now()
	for _, apiSessionCert := range apiSessionCerts {
		if apiSessionCert.ValidAfter != nil && now.After(*apiSessionCert.ValidAfter) &&
			apiSessionCert.ValidBefore != nil && now.Before(*apiSessionCert.ValidBefore) {
			validPrints = append(validPrints, apiSessionCert.Fingerprint)
		}
	}

	return validPrints, nil
}
