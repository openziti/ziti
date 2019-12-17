/*
	Copyright 2019 Netfoundry, Inc.

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

import (
	"go.etcd.io/bbolt"
)

func NewEventLogHandler(env Env) *EventLogHandler {
	handler := &EventLogHandler{
		baseHandler: baseHandler{
			env:   env,
			store: env.GetStores().EventLog,
		},
	}
	handler.impl = handler
	return handler
}

type EventLogHandler struct {
	baseHandler
}

func (handler *EventLogHandler) NewModelEntity() BaseModelEntity {
	return &EventLog{}
}

func (handler *EventLogHandler) HandleCreate(entity *EventLog) (string, error) {
	return handler.create(entity, nil)
}

func (handler *EventLogHandler) HandleRead(id string) (*EventLog, error) {
	modelEntity := &EventLog{}
	if err := handler.read(id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *EventLogHandler) handleReadInTx(tx *bbolt.Tx, id string) (*EventLog, error) {
	modelEntity := &EventLog{}
	if err := handler.readInTx(tx, id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *EventLogHandler) HandleList(queryOptions *QueryOptions) (*EventLogListResult, error) {
	result := &EventLogListResult{handler: handler}
	err := handler.parseAndList(queryOptions, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

type EventLogListResult struct {
	handler   *EventLogHandler
	EventLogs []*EventLog
	QueryMetaData
}

func (result *EventLogListResult) collect(tx *bbolt.Tx, ids []string, queryMetaData *QueryMetaData) error {
	result.QueryMetaData = *queryMetaData
	for _, key := range ids {
		entity, err := result.handler.handleReadInTx(tx, key)
		if err != nil {
			return err
		}
		result.EventLogs = append(result.EventLogs, entity)
	}
	return nil
}
