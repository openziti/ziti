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
	"github.com/netfoundry/ziti-edge/edge/controller/persistence"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"go.etcd.io/bbolt"
)

func NewAppwanHandler(env Env) *AppwanHandler {
	handler := &AppwanHandler{
		baseHandler: baseHandler{
			env:   env,
			store: env.GetStores().Appwan,
		},
	}
	handler.impl = handler
	return handler
}

type AppwanHandler struct {
	baseHandler
}

func (handler *AppwanHandler) NewModelEntity() BaseModelEntity {
	return &Appwan{}
}

func (handler *AppwanHandler) HandleCreate(appwanModel *Appwan) (string, error) {
	return handler.create(appwanModel, nil)
}

func (handler *AppwanHandler) HandleRead(id string) (*Appwan, error) {
	modelAppwan := &Appwan{}
	if err := handler.read(id, modelAppwan); err != nil {
		return nil, err
	}
	return modelAppwan, nil
}

func (handler *AppwanHandler) handleReadInTx(tx *bbolt.Tx, id string) (*Appwan, error) {
	modelAppwan := &Appwan{}
	if err := handler.readInTx(tx, id, modelAppwan); err != nil {
		return nil, err
	}
	return modelAppwan, nil
}

func (handler *AppwanHandler) IsUpdated(field string) bool {
	return field != "Services" && field != "Identities"
}

func (handler *AppwanHandler) HandleUpdate(appwan *Appwan) error {
	return handler.update(appwan, handler, nil)
}

func (handler *AppwanHandler) HandlePatch(appwan *Appwan, checker boltz.FieldChecker) error {
	combinedChecker := &AndFieldChecker{first: handler, second: checker}
	return handler.patch(appwan, combinedChecker, nil)
}

func (handler *AppwanHandler) HandleDelete(id string) error {
	return handler.delete(id, nil, nil)
}

func (handler *AppwanHandler) HandleQuery(query string) (*AppwanListResult, error) {
	result := &AppwanListResult{handler: handler}
	err := handler.list(query, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *AppwanHandler) HandleList(queryOptions *QueryOptions) (*AppwanListResult, error) {
	result := &AppwanListResult{handler: handler}
	err := handler.parseAndList(queryOptions, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *AppwanHandler) HandleListServices(id string) ([]*Service, error) {
	var result []*Service
	err := handler.HandleCollectServices(id, func(entity BaseModelEntity) {
		result = append(result, entity.(*Service))
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *AppwanHandler) HandleCollectServices(id string, collector func(entity BaseModelEntity)) error {
	return handler.GetDb().View(func(tx *bbolt.Tx) error {
		_, err := handler.handleReadInTx(tx, id)
		if err != nil {
			return err
		}
		association := handler.store.GetLinkCollection(persistence.FieldAppwanServices)
		for _, serviceId := range association.GetLinks(tx, id) {
			service, err := handler.env.GetHandlers().Service.handleReadInTx(tx, serviceId)
			if err != nil {
				return err
			}
			collector(service)
		}
		return nil
	})
}

func (handler *AppwanHandler) HandleCollectIdentities(id string, collector func(entity BaseModelEntity)) error {
	return handler.GetDb().View(func(tx *bbolt.Tx) error {
		_, err := handler.handleReadInTx(tx, id)
		if err != nil {
			return err
		}
		association := handler.store.GetLinkCollection(persistence.FieldAppwanIdentities)
		for _, identityId := range association.GetLinks(tx, id) {
			identity, err := handler.env.GetHandlers().Identity.handleReadInTx(tx, identityId)
			if err != nil {
				return err
			}
			collector(identity)
		}
		return nil
	})
}

type AppwanListResult struct {
	handler *AppwanHandler
	Appwans []*Appwan
	QueryMetaData
}

func (result *AppwanListResult) collect(tx *bbolt.Tx, ids []string, queryMetaData *QueryMetaData) error {
	result.QueryMetaData = *queryMetaData
	for _, key := range ids {
		appWan, err := result.handler.handleReadInTx(tx, key)
		if err != nil {
			return err
		}
		result.Appwans = append(result.Appwans, appWan)
	}
	return nil
}
