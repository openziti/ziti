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

	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/stretchr/testify/require"
)

// Test_settingsHandler_NewCtrlAddress locks in that a controller's NewCtrlAddress setting becomes the
// router's endpoint, applied in order with the controller lists.
func Test_settingsHandler_NewCtrlAddress(t *testing.T) {
	t.Run("the address replaces the router's endpoints", func(t *testing.T) {
		req := require.New(t)

		// arrange
		updater := newCtrlListUpdater("ctrl1")
		handler := newCtrlListHandler(t, updater)

		// act
		receive(t, newSettingsHandler(handler), "ctrl1", newCtrlAddressSetting(ctrlAddr("ctrl1-new")))
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1-new"}, updater.ids())
	})

	t.Run("an empty address changes nothing", func(t *testing.T) {
		req := require.New(t)

		// arrange
		updater := newCtrlListUpdater("ctrl1", "ctrl2")
		handler := newCtrlListHandler(t, updater)

		// act
		receive(t, newSettingsHandler(handler), "ctrl1", newCtrlAddressSetting(""))
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1", "ctrl2"}, updater.ids())
	})

	t.Run("an address received after a leader's list is applied after it", func(t *testing.T) {
		req := require.New(t)

		// arrange
		updater := newCtrlListUpdater("ctrl1", "ctrl2")
		handler := newCtrlListHandler(t, updater)
		held, err := holdApplying(t, updater, handler)
		req.NoError(err)

		// act
		receive(t, handler, "ctrl1", leaderList(10, "ctrl1", "ctrl2", "ctrl3"))
		receive(t, newSettingsHandler(handler), "ctrl1", newCtrlAddressSetting(ctrlAddr("ctrl1-new")))
		held.release()
		flush(t, handler)

		// assert
		req.Equal([]string{"ctrl1-new"}, updater.ids(), "the later address must be applied last")
	})
}

// newCtrlAddressSetting returns a Settings message moving its sender to address.
func newCtrlAddressSetting(address string) *ctrl_pb.Settings {
	return &ctrl_pb.Settings{Data: map[int32][]byte{
		int32(ctrl_pb.SettingTypes_NewCtrlAddress): []byte(address),
	}}
}
