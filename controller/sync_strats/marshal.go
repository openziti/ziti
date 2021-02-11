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
	"github.com/openziti/edge/pb/edge_ctrl_pb"
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

func GetFingerprintsByApiSessionId(ae *env.AppEnv, apiSessionId string) ([]string, error) {
	apiSession, err := ae.GetHandlers().ApiSession.Read(apiSessionId)

	if err != nil {
		return nil, fmt.Errorf("could not query fingerprints by api session id [%s]: %s", apiSessionId, err)
	}

	return getFingerprints(ae, apiSession.IdentityId, apiSessionId)
}

func getFingerprints(ae *env.AppEnv, identityId, apiSessionId string) ([]string, error) {
	prints := map[string]struct{}{}
	err := ae.Handlers.ApiSession.VisitFingerprintsForApiSession(identityId, apiSessionId, func(fingerprint string) bool {
		prints[fingerprint] = struct{}{}
		return false
	})
	if err != nil {
		return nil, err
	}
	var result []string
	for k := range prints {
		result = append(result, k)
	}
	return result, nil
}
