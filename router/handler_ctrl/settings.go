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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/fabric/common/pb/ctrl_pb"
	"google.golang.org/protobuf/proto"
)

// settingsHandler is a catch-all handler for all settings message (currently only sent on router connect).
type settingsHandler struct {
	updater CtrlAddressUpdater
}

func (handler *settingsHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_SettingsType)
}

func (handler *settingsHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	settings := &ctrl_pb.Settings{}
	if err := proto.Unmarshal(msg.Body, settings); err == nil {
		for settingType, settingValue := range settings.Data {
			log := pfxlog.ContextLogger(ch.Label()).WithFields(map[string]interface{}{
				"settingType":  settingType,
				"settingValue": settingValue,
			})

			switch settingType {
			case int32(ctrl_pb.SettingTypes_NewCtrlAddress):
				newAddress := string(settingValue)
				handler.updater.UpdateCtrlEndpoints([]string{newAddress})
			default:
				log.Error("unknown setting type, ignored")
			}
		}
	} else {
		pfxlog.ContextLogger(ch.Label()).WithError(err).Error("error unmarshalling")
	}
}

func newSettingsHandler(updater CtrlAddressUpdater) channel.TypedReceiveHandler {
	return &settingsHandler{
		updater: updater,
	}
}
