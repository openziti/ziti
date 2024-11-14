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

package db

import (
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	"strconv"
	"time"
)

const (
	FieldControllerCtrlAddress       = "ctrlAddress"
	FieldControllerCertPem           = "certPem"
	FieldControllerFingerprint       = "fingerprint"
	FieldControllerIsOnline          = "isOnline"
	FieldControllerLastJoinedAt      = "lastJoinedAt"
	FieldControllerApiAddresses      = "apiAddresses"
	FieldControllerApiAddressVersion = "apiAddresses.version"
	FieldControllerApiAddressUrl     = "apiAddresses.url"
)

type Controller struct {
	boltz.BaseExtEntity
	Name         string    `json:"name"`
	CtrlAddress  string    `json:"address"`
	CertPem      string    `json:"certPem"`
	Fingerprint  string    `json:"fingerprint"`
	IsOnline     bool      `json:"isOnline"`
	LastJoinedAt time.Time `json:"lastJoinedAt"`
	ApiAddresses map[string][]ApiAddress
}

type ApiAddress struct {
	Url     string `json:"url"`
	Version string `json:"version"`
}

func (entity *Controller) GetName() string {
	return entity.Name
}

func (entity *Controller) GetEntityType() string {
	return EntityTypeControllers
}

var _ ControllerStore = (*controllerStoreImpl)(nil)

type ControllerStore interface {
	Store[*Controller]
	GetNameIndex() boltz.ReadIndex
}

func newControllerStore(stores *stores) *controllerStoreImpl {
	store := &controllerStoreImpl{}
	store.baseStore = newBaseStore[*Controller](stores, store)
	store.InitImpl(store)
	return store
}

type controllerStoreImpl struct {
	*baseStore[*Controller]
	indexName boltz.ReadIndex
}

func (store *controllerStoreImpl) GetNameIndex() boltz.ReadIndex {
	return store.indexName
}

func (store *controllerStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()
	store.indexName = store.addUniqueNameField()

	store.AddSymbol(FieldControllerCtrlAddress, ast.NodeTypeString)
	store.AddSymbol(FieldControllerCertPem, ast.NodeTypeString)
	store.AddSymbol(FieldControllerFingerprint, ast.NodeTypeString)
	store.AddSymbol(FieldControllerIsOnline, ast.NodeTypeBool)
	store.AddSymbol(FieldControllerLastJoinedAt, ast.NodeTypeDatetime)
}

func (store *controllerStoreImpl) initializeLinked() {}

func (store *controllerStoreImpl) NewEntity() *Controller {
	return &Controller{}
}

func (store *controllerStoreImpl) FillEntity(entity *Controller, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
	entity.CtrlAddress = bucket.GetStringWithDefault(FieldControllerCtrlAddress, "")
	entity.CertPem = bucket.GetStringOrError(FieldControllerCertPem)
	entity.Fingerprint = bucket.GetStringOrError(FieldControllerFingerprint)
	entity.IsOnline = bucket.GetBoolWithDefault(FieldControllerIsOnline, false)
	entity.LastJoinedAt = bucket.GetTimeOrError(FieldControllerLastJoinedAt)
	entity.ApiAddresses = map[string][]ApiAddress{}

	apiListBucket := bucket.GetBucket(FieldControllerApiAddresses)
	if apiListBucket != nil {
		apiListCursor := apiListBucket.Cursor()

		for apiKey, _ := apiListCursor.First(); apiKey != nil; apiKey, _ = apiListCursor.Next() {
			apiBucket := apiListBucket.GetBucket(string(apiKey))

			if apiBucket != nil {
				entity.ApiAddresses[string(apiKey)] = nil
				instanceCursor := apiBucket.Cursor()

				for instanceKey, _ := instanceCursor.First(); instanceKey != nil; instanceKey, _ = instanceCursor.Next() {
					instance := apiBucket.GetBucket(string(instanceKey))

					if instance != nil {
						newInstance := ApiAddress{
							Url:     instance.GetStringWithDefault(FieldControllerApiAddressUrl, ""),
							Version: instance.GetStringWithDefault(FieldControllerApiAddressVersion, ""),
						}
						entity.ApiAddresses[string(apiKey)] = append(entity.ApiAddresses[string(apiKey)], newInstance)
					}
				}
			}
		}
	}
}

func (store *controllerStoreImpl) PersistEntity(entity *Controller, ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldName, entity.Name)
	ctx.SetString(FieldControllerCtrlAddress, entity.CtrlAddress)
	ctx.SetString(FieldControllerCertPem, entity.CertPem)
	ctx.SetString(FieldControllerFingerprint, entity.Fingerprint)
	ctx.SetBool(FieldControllerIsOnline, entity.IsOnline)
	ctx.SetTimeP(FieldControllerLastJoinedAt, &entity.LastJoinedAt)

	apiListBucket := ctx.Bucket.GetOrCreateBucket(FieldControllerApiAddresses)

	for apiKey, apis := range entity.ApiAddresses {
		apiBucket, _ := apiListBucket.EmptyBucket(apiKey)
		for i, instance := range apis {
			instanceBucket := apiBucket.GetOrCreateBucket(strconv.Itoa(i))

			instanceBucket.SetString(FieldControllerApiAddressUrl, instance.Url, ctx.FieldChecker)
			instanceBucket.SetString(FieldControllerApiAddressVersion, instance.Version, ctx.FieldChecker)
		}
	}
}
