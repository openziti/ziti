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

package routes

import (
	"testing"

	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/stretchr/testify/require"
)

// Test_deleteLink_ReportsAnUnknownLinkAsNotFound: removing an id the controller does not know still goes
// ahead, since it faults the id at every connected router, but the caller is told no such link was found
// rather than that one was deleted.
func Test_deleteLink_ReportsAnUnknownLinkAsNotFound(t *testing.T) {
	req := require.New(t)
	var removed []string
	removeLink := func(known bool) func(string) bool {
		return func(id string) bool {
			removed = append(removed, id)
			return known
		}
	}

	err := deleteLink(removeLink(false), "never-existed")
	req.True(boltz.IsErrNotFoundErr(err), "an unknown link is reported as not found, got %v", err)
	req.Equal([]string{"never-existed"}, removed, "and is still removed")

	req.NoError(deleteLink(removeLink(true), "known"), "a known link deletes")
}
