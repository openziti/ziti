/*
	Copyright NetFoundry, Inc.

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
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/foundation/channel2"
	"strings"
)

// CtrlAddressChanger provides indirect access to underlying configuration without creating
// import loops w/ *router.Config.
type CtrlAddressChanger interface {
	CreateBackup() (string, error)
	UpdateControllerEndpoint(address string) error
	Save() error
	CurrentCtrlAddress() string
}

// settingsHandler is a catch-all handler for all settings message (currently only sent on router connect).
type settingsHandler struct {
	config CtrlAddressChanger
}

func (handler *settingsHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_SettingsType)
}

func (handler *settingsHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
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
				handler.newCtrlAddress(newAddress)
			default:
				log.Error("unknown setting type, ignored")
			}
		}
	} else {
		pfxlog.ContextLogger(ch.Label()).WithError(err).Error("error unmarshalling")
	}
}

func newSettingsHandler(config CtrlAddressChanger) channel2.ReceiveHandler {
	return &settingsHandler{
		config: config,
	}
}

// newCtrlAddress interrogates teh current configuration for controller ctrl address updates.
// If necessary it will create a backup of the current config, alter the runtime ctrl address,
// and save a new version of the router configuration in place.
func (handler *settingsHandler) newCtrlAddress(newAddress string) {
	currentAddress := strings.TrimSpace(handler.config.CurrentCtrlAddress())
	newAddress = strings.TrimSpace(newAddress)

	log := pfxlog.Logger().WithFields(map[string]interface{}{
		"newAddress":     newAddress,
		"currentAddress": currentAddress,
	})

	log.Info("received new controller address header")

	if currentAddress == newAddress {
		log.Warn("ignoring new controller address, same as current value")
		return
	}

	log.Info("new controller address detect creating backup and saving new configuration")
	if destBackupPath, err := handler.config.CreateBackup(); err == nil {
		log.WithField("backupPath", destBackupPath).Info("backup configuration created")
	} else {
		log.WithError(err).Error("could not create configuration backup")
		return
	}

	if err := handler.config.UpdateControllerEndpoint(newAddress); err != nil {
		log.WithError(err).Error("could not update controller endpoint address")
		return
	}

	if err := handler.config.Save(); err != nil {
		log.WithError(err).Error("could not save new configuration")
		return
	}

	log.Info("successfully saved new controller address")

}
