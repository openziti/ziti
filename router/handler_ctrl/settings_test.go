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

	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func newCtrlAddressMsg(t *testing.T, address string) *channel.Message {
	body, err := proto.Marshal(&ctrl_pb.Settings{Data: map[int32][]byte{
		int32(ctrl_pb.SettingTypes_NewCtrlAddress): []byte(address),
	}})
	require.NoError(t, err)
	return channel.NewMessage(int32(ctrl_pb.ContentType_SettingsType), body)
}

// Test_Settings_NewCtrlAddressReplacesEndpoints: a controller's new address becomes the router's
// endpoint.
func Test_Settings_NewCtrlAddressReplacesEndpoints(t *testing.T) {
	req := require.New(t)
	updater := newCtrlListUpdater("ctrl1")
	handler := newCtrlListHandler(t, updater)

	newSettingsHandler(handler).HandleReceive(newCtrlAddressMsg(t, ctrlAddr("ctrl1-new")), &ctrlListChannel{id: "ctrl1"})
	flush(t, handler)

	req.Equal([]string{"ctrl1-new"}, updater.ids())
}

// Test_Settings_EmptyNewCtrlAddressIsIgnored: an empty new address changes nothing.
func Test_Settings_EmptyNewCtrlAddressIsIgnored(t *testing.T) {
	req := require.New(t)
	updater := newCtrlListUpdater("ctrl1", "ctrl2")
	handler := newCtrlListHandler(t, updater)

	newSettingsHandler(handler).HandleReceive(newCtrlAddressMsg(t, ""), &ctrlListChannel{id: "ctrl1"})
	flush(t, handler)

	req.Equal([]string{"ctrl1", "ctrl2"}, updater.ids())
}

// Test_Settings_NewCtrlAddressAppliesInOrderWithLists: a new address received after a leader's list, both
// waiting, is applied after it, since both change the endpoints.
func Test_Settings_NewCtrlAddressAppliesInOrderWithLists(t *testing.T) {
	req := require.New(t)
	updater := newCtrlListUpdater("ctrl1", "ctrl2")
	handler := newCtrlListHandler(t, updater)

	whileApplying(t, updater, handler, func() {
		handler.HandleReceive(ctrlListMsg(t, 10, true, "ctrl1", "ctrl2", "ctrl3"), &ctrlListChannel{id: "ctrl1"})
		newSettingsHandler(handler).HandleReceive(newCtrlAddressMsg(t, ctrlAddr("ctrl1-new")), &ctrlListChannel{id: "ctrl1"})
	})

	req.Equal([]string{"ctrl1-new"}, updater.ids(), "the later address must be applied last")
}
