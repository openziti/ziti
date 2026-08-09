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

	"github.com/openziti/channel/v5"
	"github.com/stretchr/testify/require"
)

// connIterTestChannel and connIterTestUnderlay are doubles supplying only what the underlay event handlers
// read. They embed their interfaces, so a method the handlers do not use panics rather than returning a
// zero value.
type connIterTestChannel struct {
	channel.Channel
}

func (self *connIterTestChannel) Label() string { return "test" }
func (self *connIterTestChannel) GetUnderlayCountsByType() map[string]int {
	return map[string]int{ChannelTypeDefault: 1}
}
func (self *connIterTestChannel) IsClosed() bool { return false }

type connIterTestUnderlay struct {
	channel.Underlay
}

func (self *connIterTestUnderlay) Headers() map[int32][]byte { return nil }

// TestConnIteration_AdvancesOnRemoveAsWellAsAdd covers the iteration a controller uses to tell whether the
// link connections it was told about have been superseded. Advancing it only when an underlay is added lets
// the connection set shrink while the iteration still claims the controller's copy is current, so the
// controller cannot tell its copy is stale and goes on comparing against connections that are gone.
//
// The listener side is what makes this reachable: it inherits these handlers rather than overriding them,
// where the dial side used to bump on removal itself.
func TestConnIteration_AdvancesOnRemoveAsWellAsAdd(t *testing.T) {
	ch := &connIterTestChannel{}
	underlay := &connIterTestUnderlay{}

	base := &BaseLinkChannel{}

	start := base.GetConnStateIteration()

	base.UnderlayAdded(ch, underlay)
	afterAdd := base.GetConnStateIteration()
	require.Greater(t, afterAdd, start, "adding an underlay must advance the connection state iteration")

	base.UnderlayRemoved(ch, underlay)
	afterRemove := base.GetConnStateIteration()
	require.Greater(t, afterRemove, afterAdd, "removing an underlay must advance the connection state iteration")
}

// TestConnIteration_DialSideAdvancesOnceOnRemove: the dial side used to bump on removal on top of the base
// handler. Now that the base does it, doing so again would advance the iteration twice for one change and
// pull the two ends of a link further out of step.
func TestConnIteration_DialSideAdvancesOnceOnRemove(t *testing.T) {
	ch := &connIterTestChannel{}
	underlay := &connIterTestUnderlay{}

	dial := &DialLinkChannel{
		BaseLinkChannel: &BaseLinkChannel{},
		changeCallback:  func(*DialLinkChannel) {},
	}

	before := dial.GetConnStateIteration()
	dial.UnderlayRemoved(ch, underlay)

	require.Equal(t, before+1, dial.GetConnStateIteration(),
		"one underlay removal must advance the iteration exactly once")
}
