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

package handler_peer_ctrl

import (
	"crypto/x509"

	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/v2/controller/raft/mesh"
	"github.com/pkg/errors"
)

// requireSentBy returns an error unless ch's peer certificate identifies the controller memberId.
func requireSentBy(ch channel.Channel, memberId string) error {
	return checkSentBy(ch.Underlay().Certificates(), memberId)
}

// checkSentBy returns an error unless certs carry the controller SPIFFE id memberId.
func checkSentBy(certs []*x509.Certificate, memberId string) error {
	id, err := mesh.ExtractSpiffeId(certs)
	if err != nil {
		return errors.Wrap(err, "unable to identify sending controller")
	}
	if id != memberId {
		return errors.Errorf("request for member %s was sent by controller %s", memberId, id)
	}
	return nil
}
