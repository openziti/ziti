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

package handler_ctrl

import (
	"testing"

	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/stretchr/testify/require"
)

// Test_settingsHandler_NewCtrlAddress locks in that a controller's NewCtrlAddress setting moves only that
// controller, applied in order with the controller lists.
func Test_settingsHandler_NewCtrlAddress(t *testing.T) {
	t.Run("a router without stored details applies the address and keeps its connected controllers", func(t *testing.T) {
		req := require.New(t)

		// arrange: the router reached ctrl2 from its initial endpoints and has stored no details
		ctrls := newCtrlListControllers()
		ctrls.details = nil
		ctrls.connected["ctrl2"] = &connectedCtrl{address: "tls:ctrl2:6262"}
		handler := newCtrlListHandler(t, ctrls)

		// act
		receive(t, newSettingsHandler(handler), "ctrl1", newCtrlAddressSetting("tls:ctrl1-new:6262"))
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl2"}, ctrls.ids())
		req.Equal("tls:ctrl1-new:6262", ctrls.detail("ctrl1").Endpoints[0].Address)
		req.Contains(ctrls.GetAll(), "ctrl2", "a connected controller without a detail must be kept")
	})

	t.Run("the address replaces only the sending controller's endpoint", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrls := newCtrlListControllers("ctrl1", "ctrl2")
		handler := newCtrlListHandler(t, ctrls)

		// act
		receive(t, newSettingsHandler(handler), "ctrl1", newCtrlAddressSetting("tls:ctrl1-new:6262"))
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl2"}, ctrls.ids())
		req.Equal("tls:ctrl1-new:6262", ctrls.detail("ctrl1").Endpoints[0].Address)
		req.Equal("tls:ctrl2:6262", ctrls.detail("ctrl2").Endpoints[0].Address, "another controller's endpoint must be left as it was")
	})

	t.Run("an empty address changes nothing", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrls := newCtrlListControllers("ctrl1", "ctrl2")
		handler := newCtrlListHandler(t, ctrls)

		// act
		receive(t, newSettingsHandler(handler), "ctrl1", newCtrlAddressSetting(""))
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl2"}, ctrls.ids())
		req.Equal("tls:ctrl1:6262", ctrls.detail("ctrl1").Endpoints[0].Address)
	})

	t.Run("each controller's latest address survives too many waiting updates", func(t *testing.T) {
		req := require.New(t)

		// arrange
		ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
		handler := newCtrlListHandler(t, ctrls)
		settings := newSettingsHandler(handler)
		held, err := holdApplying(t, ctrls, handler)
		req.NoError(err)

		// act: with the two settings ahead of them, the lists fill the queue, so the last setting overflows it
		receive(t, settings, "ctrl1", newCtrlAddressSetting("tls:ctrl1-old:6262"))
		receive(t, settings, "ctrl1", newCtrlAddressSetting("tls:ctrl1-new:6262"))
		for i := range maxPendingCtrlUpdates - 2 {
			receive(t, handler, "ctrl2", followerList(uint64(i+1), "ctrl1", "ctrl2", "ctrl3", "ctrl4"))
		}
		receive(t, settings, "ctrl2", newCtrlAddressSetting("tls:ctrl2-new:6262"))
		held.release()
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl2", "ctrl3"}, ctrls.ids(), "the waiting lists must be dropped")
		req.Equal("tls:ctrl1-new:6262", ctrls.detail("ctrl1").Endpoints[0].Address, "the latest address before the overflow must apply")
		req.Equal("tls:ctrl2-new:6262", ctrls.detail("ctrl2").Endpoints[0].Address, "the address that overflows the queue must apply")
	})
}

// newCtrlAddressSetting returns a Settings message moving its sender to address.
func newCtrlAddressSetting(address string) *ctrl_pb.Settings {
	return &ctrl_pb.Settings{Data: map[int32][]byte{
		int32(ctrl_pb.SettingTypes_NewCtrlAddress): []byte(address),
	}}
}
