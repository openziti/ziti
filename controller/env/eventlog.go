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

package env

import (
	"encoding/json"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/model"
)

type DefaultEventLogger struct {
	Ae *AppEnv
}

func (el *DefaultEventLogger) Log(actorType, actorId, eventType, entityType, entityId, formatString string, formatData []string, data map[interface{}]interface{}) {
	fmtDataStr, err := json.Marshal(formatData)

	if err != nil {
		pfxlog.Logger().WithField("cause", err).Error("could not save event log")
		return
	}

	var fmtInt []interface{}

	for _, fd := range formatData {
		fmtInt = append(fmtInt, fd)
	}

	pm := map[string]interface{}{}

	for k, v := range data {
		ks := k.(string)
		pm[ks] = v
	}

	_, err = el.Ae.Managers.EventLog.Create(&model.EventLog{
		Type:             eventType,
		ActorId:          actorId,
		ActorType:        actorType,
		EntityId:         entityId,
		EntityType:       entityType,
		FormatData:       string(fmtDataStr),
		FormatString:     formatString,
		FormattedMessage: fmt.Sprintf(formatString, fmtInt...),
		Data:             pm,
	})

	if err != nil {
		pfxlog.Logger().WithField("cause", err).Errorf("could not store event log: %s", err)
	}
}
