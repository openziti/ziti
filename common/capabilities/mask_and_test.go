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

package capabilities

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Test_Mask_And_KeepsOnlyWhatBothHave: the cluster-wide answer for a capability is the intersection over
// its members, so folding And over them has to narrow rather than widen.
func Test_Mask_And_KeepsOnlyWhatBothHave(t *testing.T) {
	req := require.New(t)

	a := NewMask(ControllerLinkGossip, ControllerGroupedCtrlChan)
	b := NewMask(ControllerLinkGossip, ControllerCreateCircuitV2)

	both := a.And(b)
	req.True(both.IsSet(ControllerLinkGossip))
	req.False(both.IsSet(ControllerGroupedCtrlChan), "a capability only one side has must not survive")
	req.False(both.IsSet(ControllerCreateCircuitV2))

	req.True(a.IsSet(ControllerGroupedCtrlChan), "And must not mutate its operands")
	req.True(b.IsSet(ControllerCreateCircuitV2))
}

// Test_Mask_And_NilYieldsNothing is the case that matters for a cluster member nothing is known about: it
// must contribute no capabilities rather than being skipped.
func Test_Mask_And_NilYieldsNothing(t *testing.T) {
	req := require.New(t)

	full := NewMask(ControllerLinkGossip)
	var missing *Mask[ControllerCapability]

	req.False(full.And(missing).IsSet(ControllerLinkGossip))
	req.False(missing.And(full).IsSet(ControllerLinkGossip))
	req.False(full.And(NewMask[ControllerCapability]()).IsSet(ControllerLinkGossip))
}

// Test_Mask_Equals covers the comparison a transition check rests on: reporting a change that did not
// happen would give up a store that is still in use.
func Test_Mask_Equals(t *testing.T) {
	req := require.New(t)

	req.True(NewMask(ControllerLinkGossip).Equals(NewMask(ControllerLinkGossip)))
	req.False(NewMask(ControllerLinkGossip).Equals(NewMask(ControllerGroupedCtrlChan)))
	req.False(NewMask(ControllerLinkGossip).Equals(NewMask[ControllerCapability]()))

	var missing *Mask[ControllerCapability]
	req.True(missing.Equals(NewMask[ControllerCapability]()), "nil and empty both have nothing set")
	req.True(missing.Equals(nil))
}
