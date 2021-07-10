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

import (
	"fmt"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/storage/ast"
	"github.com/openziti/foundation/storage/boltz"
	"go.etcd.io/bbolt"
	"strings"
)

func NewCaHandler(env Env) *CaHandler {
	handler := &CaHandler{
		baseHandler: newBaseHandler(env, env.GetStores().Ca),
	}
	handler.impl = handler
	return handler
}

type CaHandler struct {
	baseHandler
}

func (handler *CaHandler) newModelEntity() boltEntitySink {
	return &Ca{}
}

func (handler *CaHandler) Create(caModel *Ca) (string, error) {
	if caModel.IdentityNameFormat == "" {
		caModel.IdentityNameFormat = DefaultCaIdentityNameFormat
	}
	return handler.createEntity(caModel)
}

func (handler *CaHandler) Read(id string) (*Ca, error) {
	modelEntity := &Ca{}
	if err := handler.readEntity(id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *CaHandler) readInTx(tx *bbolt.Tx, id string) (*Ca, error) {
	modelEntity := &Ca{}
	if err := handler.readEntityInTx(tx, id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *CaHandler) IsUpdated(field string) bool {
	return strings.EqualFold(field, persistence.FieldName) ||
		strings.EqualFold(field, boltz.FieldTags) ||
		strings.EqualFold(field, persistence.FieldCaIsAutoCaEnrollmentEnabled) ||
		strings.EqualFold(field, persistence.FieldCaIsOttCaEnrollmentEnabled) ||
		strings.EqualFold(field, persistence.FieldCaIsAuthEnabled) ||
		strings.EqualFold(field, persistence.FieldIdentityRoles) ||
		strings.EqualFold(field, persistence.FieldCaIdentityNameFormat)
}

func (handler *CaHandler) Update(ca *Ca) error {
	if ca.IdentityNameFormat == "" {
		ca.IdentityNameFormat = DefaultCaIdentityNameFormat
	}

	return handler.updateEntity(ca, handler)
}

func (handler *CaHandler) Patch(ca *Ca, checker boltz.FieldChecker) error {
	if checker.IsUpdated(persistence.FieldCaIdentityNameFormat) {
		if ca.IdentityNameFormat == "" {
			ca.IdentityNameFormat = DefaultCaIdentityNameFormat
		}
	}

	combinedChecker := &AndFieldChecker{first: handler, second: checker}
	return handler.patchEntity(ca, combinedChecker)
}

func (handler *CaHandler) Verified(ca *Ca) error {
	ca.IsVerified = true
	checker := &boltz.MapFieldChecker{
		persistence.FieldCaIsVerified: struct{}{},
	}
	return handler.patchEntity(ca, checker)
}

func (handler *CaHandler) Delete(id string) error {
	return handler.deleteEntity(id)
}

func (handler *CaHandler) Query(query string) (*CaListResult, error) {
	result := &CaListResult{handler: handler}
	if err := handler.list(query, result.collect); err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *CaHandler) Stream(query string, collect func(*Ca, error) error) error {
	filter, err := ast.Parse(handler.Store, query)

	if err != nil {
		return fmt.Errorf("could not parse query for streaming cas: %v", err)
	}

	return handler.env.GetDbProvider().GetDb().View(func(tx *bbolt.Tx) error {
		for cursor := handler.Store.IterateIds(tx, filter); cursor.IsValid(); cursor.Next() {
			current := cursor.Current()

			ca, err := handler.readInTx(tx, string(current))
			if err := collect(ca, err); err != nil {
				return err
			}
		}
		return collect(nil, nil)
	})
}

type CaListResult struct {
	handler *CaHandler
	Cas     []*Ca
	models.QueryMetaData
}

func (result *CaListResult) collect(tx *bbolt.Tx, ids []string, queryMetaData *models.QueryMetaData) error {
	result.QueryMetaData = *queryMetaData
	for _, key := range ids {
		entity, err := result.handler.readInTx(tx, key)
		if err != nil {
			return err
		}
		result.Cas = append(result.Cas, entity)
	}
	return nil
}

const (
	FormatSentinelStart       = "["
	FormatSentinelEnd         = "]"
	FormatSymbolCaName        = "caName"
	FormatSymbolCaId          = "caId"
	FormatSymbolCommonName    = "commonName"
	FormatSymbolRequestedName = "requestedName"
	FormatSymbolIdentityId    = "identityId"

	// DefaultCaIdentityNameFormat = "[caName] - [commonName]"
	DefaultCaIdentityNameFormat = FormatSentinelStart + FormatSymbolCaName + FormatSentinelEnd + "-" + FormatSentinelStart + FormatSymbolCommonName + FormatSentinelEnd
)

type Formatter struct {
	symbolValues  map[string]string
	sentinelStart string
	sentinelEnd   string
}

func NewFormatter(symbols map[string]string) *Formatter {
	return &Formatter{
		symbolValues:  symbols,
		sentinelStart: FormatSentinelStart,
		sentinelEnd:   FormatSentinelEnd,
	}
}

func (formatter *Formatter) Format(name string) string {
	for symbol, value := range formatter.symbolValues {
		searchSymbol := formatter.sentinelStart + symbol + formatter.sentinelEnd
		name = strings.Replace(name, searchSymbol, value, -1)
	}

	return name
}
