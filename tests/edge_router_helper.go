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

package tests

import (
	"time"

	"github.com/openziti/ziti/router"
)

// EdgeRouterHelper wraps a router.Router with test-oriented helper methods.
// All methods return values; none accept *testing.T.
// Because it embeds *router.Router, all router methods are available directly.
type EdgeRouterHelper struct {
	*router.Router
}

// WaitForRouterSync polls until the router has received its initial data state
// from the controller (indicated by a non-zero RDM index). Returns true if
// sync completes within the timeout.
func (h *EdgeRouterHelper) WaitForRouterSync(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		rdm := h.GetRouterDataModel()
		if rdm != nil {
			if idx, ok := rdm.CurrentIndex(); ok && idx > 0 {
				return true
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// HasRevocation reports whether the given id is currently present in the
// router's RDM revocations map.
func (h *EdgeRouterHelper) HasRevocation(id string) bool {
	_, ok := h.GetRouterDataModel().Revocations.Get(id)
	return ok
}

// WaitForRevocation polls the router's RDM until id appears in the revocations
// map or timeout elapses. Returns true if the entry is found within the timeout.
func (h *EdgeRouterHelper) WaitForRevocation(id string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if h.HasRevocation(id) {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// WaitForRevocationGone polls the router's RDM until id is absent from the
// revocations map or timeout elapses. Returns true if gone within the timeout.
func (h *EdgeRouterHelper) WaitForRevocationGone(id string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !h.HasRevocation(id) {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}
