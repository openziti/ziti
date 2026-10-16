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

	"github.com/openziti/channel/v5"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// sendNewCtrlAddress delivers controller from's NewCtrlAddress setting, and waits for it to be applied.
func sendNewCtrlAddress(t *testing.T, handler *updateCtrlAddressesHandler, from string, address string) {
	body, err := proto.Marshal(&ctrl_pb.Settings{Data: map[int32][]byte{
		int32(ctrl_pb.SettingTypes_NewCtrlAddress): []byte(address),
	}})
	require.NoError(t, err)
	msg := channel.NewMessage(int32(ctrl_pb.ContentType_SettingsType), body)
	newSettingsHandler(handler).HandleReceive(msg, &ctrlListChannel{id: from})
	flush(t, handler)
}

// Test_Settings_NewCtrlAddressWithoutStoredDetails: a router that has never stored controller details
// applies a controller's new address, and keeps the controllers it reached from its initial endpoints.
func Test_Settings_NewCtrlAddressWithoutStoredDetails(t *testing.T) {
	req := require.New(t)
	ctrls := newCtrlListControllers()
	ctrls.details = nil
	ctrls.connected["ctrl2"] = &connectedCtrl{address: "tls:ctrl2:6262"}
	handler := newCtrlListHandler(t, ctrls)

	sendNewCtrlAddress(t, handler, "ctrl1", "tls:ctrl1-new:6262")

	req.Equal([]string{"ctrl1", "ctrl2"}, ctrls.ids())
	req.Equal("tls:ctrl1-new:6262", ctrls.detail("ctrl1").Endpoints[0].Address)
	req.Contains(ctrls.GetAll(), "ctrl2", "a connected controller without a detail must be kept")
}

// Test_Settings_NewCtrlAddressReplacesThatControllersEndpoint: a controller's new address replaces its own
// endpoint and leaves the others as they were.
func Test_Settings_NewCtrlAddressReplacesThatControllersEndpoint(t *testing.T) {
	req := require.New(t)
	ctrls := newCtrlListControllers("ctrl1", "ctrl2")
	handler := newCtrlListHandler(t, ctrls)

	sendNewCtrlAddress(t, handler, "ctrl1", "tls:ctrl1-new:6262")

	req.Equal([]string{"ctrl1", "ctrl2"}, ctrls.ids())
	req.Equal("tls:ctrl1-new:6262", ctrls.detail("ctrl1").Endpoints[0].Address)
	req.Equal("tls:ctrl2:6262", ctrls.detail("ctrl2").Endpoints[0].Address)
}

// Test_Settings_EmptyNewCtrlAddressIsIgnored: an empty new address changes nothing.
func Test_Settings_EmptyNewCtrlAddressIsIgnored(t *testing.T) {
	req := require.New(t)
	ctrls := newCtrlListControllers("ctrl1", "ctrl2")
	handler := newCtrlListHandler(t, ctrls)

	sendNewCtrlAddress(t, handler, "ctrl1", "")

	req.Equal([]string{"ctrl1", "ctrl2"}, ctrls.ids())
	req.Equal("tls:ctrl1:6262", ctrls.detail("ctrl1").Endpoints[0].Address)
}

// Test_Settings_NewCtrlAddressSurvivesOverflow: past the bound on waiting updates, the waiting lists are
// dropped but each controller's latest new address is still applied, since it can't be requested again.
func Test_Settings_NewCtrlAddressSurvivesOverflow(t *testing.T) {
	req := require.New(t)
	ctrls := newCtrlListControllers("ctrl1", "ctrl2", "ctrl3")
	handler := newCtrlListHandler(t, ctrls)

	settingMsg := func(address string) *channel.Message {
		body, err := proto.Marshal(&ctrl_pb.Settings{Data: map[int32][]byte{
			int32(ctrl_pb.SettingTypes_NewCtrlAddress): []byte(address),
		}})
		req.NoError(err)
		return channel.NewMessage(int32(ctrl_pb.ContentType_SettingsType), body)
	}
	// With the two settings ahead of them, these fill the queue, so the last setting overflows it.
	var lists []*channel.Message
	for i := range maxPendingCtrlUpdates - 2 {
		lists = append(lists, ctrlListMsg(t, uint64(i+1), false, "ctrl1", "ctrl2", "ctrl3", "ctrl4"))
	}

	whileApplying(t, ctrls, handler, func() {
		settings := newSettingsHandler(handler)
		settings.HandleReceive(settingMsg("tls:ctrl1-old:6262"), &ctrlListChannel{id: "ctrl1"})
		settings.HandleReceive(settingMsg("tls:ctrl1-new:6262"), &ctrlListChannel{id: "ctrl1"})
		for _, msg := range lists {
			handler.HandleReceive(msg, &ctrlListChannel{id: "ctrl2"})
		}
		settings.HandleReceive(settingMsg("tls:ctrl2-new:6262"), &ctrlListChannel{id: "ctrl2"})
	})

	req.Equal([]string{"ctrl1", "ctrl2", "ctrl3"}, ctrls.ids(), "the waiting lists must be dropped")
	req.Equal("tls:ctrl1-new:6262", ctrls.detail("ctrl1").Endpoints[0].Address, "the latest address before the overflow must apply")
	req.Equal("tls:ctrl2-new:6262", ctrls.detail("ctrl2").Endpoints[0].Address, "the address that overflows the queue must apply")
}
