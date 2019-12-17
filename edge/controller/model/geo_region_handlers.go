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
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"go.etcd.io/bbolt"
)

func NewGeoRegionHandler(env Env) *GeoRegionHandler {
	handler := &GeoRegionHandler{
		baseHandler: baseHandler{
			env:   env,
			store: env.GetStores().GeoRegion,
		},
	}
	handler.impl = handler
	return handler
}

type GeoRegionHandler struct {
	baseHandler
}

func (handler *GeoRegionHandler) NewModelEntity() BaseModelEntity {
	return &GeoRegion{}
}

func (handler *GeoRegionHandler) HandleCreate(geoRegionModel *GeoRegion) (string, error) {
	return handler.create(geoRegionModel, nil)
}

func (handler *GeoRegionHandler) HandleRead(id string) (*GeoRegion, error) {
	modelEntity := &GeoRegion{}
	if err := handler.read(id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *GeoRegionHandler) handleReadInTx(tx *bbolt.Tx, id string) (*GeoRegion, error) {
	modelEntity := &GeoRegion{}
	if err := handler.readInTx(tx, id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *GeoRegionHandler) HandleUpdate(geoRegion *GeoRegion) error {
	return handler.update(geoRegion, nil, nil)
}

func (handler *GeoRegionHandler) HandlePatch(geoRegion *GeoRegion, checker boltz.FieldChecker) error {
	return handler.patch(geoRegion, checker, nil)
}

func (handler *GeoRegionHandler) HandleDelete(id string) error {
	return handler.delete(id, nil, nil)
}

func (handler *GeoRegionHandler) HandleQuery(query string) (*GeoRegionListResult, error) {
	result := &GeoRegionListResult{handler: handler}
	err := handler.list(query, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *GeoRegionHandler) HandleList(queryOptions *QueryOptions) (*GeoRegionListResult, error) {
	result := &GeoRegionListResult{handler: handler}
	err := handler.parseAndList(queryOptions, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

type GeoRegionListResult struct {
	handler    *GeoRegionHandler
	GeoRegions []*GeoRegion
	QueryMetaData
}

func (result *GeoRegionListResult) collect(tx *bbolt.Tx, ids []string, queryMetaData *QueryMetaData) error {
	result.QueryMetaData = *queryMetaData
	for _, key := range ids {
		entity, err := result.handler.handleReadInTx(tx, key)
		if err != nil {
			return err
		}
		result.GeoRegions = append(result.GeoRegions, entity)
	}
	return nil
}
