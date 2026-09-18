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

package xlink_transport

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestVerifyDialedPeer: a dial can land on a router other than the one it was for when a router is
// replaced on the same host and port. The acceptor's hello names who actually answered, and a link
// bound to the wrong router must be refused rather than recorded against the intended destination.
func TestVerifyDialedPeer(t *testing.T) {
	req := require.New(t)

	req.NoError(verifyDialedPeer("dest", "dest", "l1"))

	err := verifyDialedPeer("someone-else", "dest", "l1")
	req.Error(err)
	req.Contains(err.Error(), "dest")
	req.Contains(err.Error(), "someone-else")
	req.Contains(err.Error(), "l1")

	req.NoError(verifyDialedPeer("", "dest", "l1"), "a peer that presented no identity is not evidence of a mismatch")
}
