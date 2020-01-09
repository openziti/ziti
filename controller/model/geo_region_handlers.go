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

func (handler *GeoRegionHandler) Create(geoRegionModel *GeoRegion) (string, error) {
	return handler.createEntity(geoRegionModel, nil)
}

func (handler *GeoRegionHandler) Read(id string) (*GeoRegion, error) {
	modelEntity := &GeoRegion{}
	if err := handler.readEntity(id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *GeoRegionHandler) readInTx(tx *bbolt.Tx, id string) (*GeoRegion, error) {
	modelEntity := &GeoRegion{}
	if err := handler.readEntityInTx(tx, id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *GeoRegionHandler) Update(geoRegion *GeoRegion) error {
	return handler.updateEntity(geoRegion, nil, nil)
}

func (handler *GeoRegionHandler) Patch(geoRegion *GeoRegion, checker boltz.FieldChecker) error {
	return handler.patchEntity(geoRegion, checker, nil)
}

func (handler *GeoRegionHandler) Delete(id string) error {
	return handler.deleteEntity(id, nil, nil)
}

func (handler *GeoRegionHandler) Query(query string) (*GeoRegionListResult, error) {
	result := &GeoRegionListResult{handler: handler}
	err := handler.list(query, result.collect)
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
		entity, err := result.handler.readInTx(tx, key)
		if err != nil {
			return err
		}
		result.GeoRegions = append(result.GeoRegions, entity)
	}
	return nil
}
