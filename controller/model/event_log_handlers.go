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

package model

func NewEventLogHandler(env Env) *EventLogHandler {
	handler := &EventLogHandler{
		baseEntityManager: newBaseEntityManager(env, env.GetStores().EventLog),
	}
	handler.impl = handler
	return handler
}

type EventLogHandler struct {
	baseEntityManager
}

func (handler *EventLogHandler) newModelEntity() boltEntitySink {
	return &EventLog{}
}

func (handler *EventLogHandler) Create(entity *EventLog) (string, error) {
	return handler.createEntity(entity)
}
