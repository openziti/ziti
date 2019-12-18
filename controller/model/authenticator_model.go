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
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/persistence"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
)

type Authenticator struct {
	BaseModelEntityImpl
	Method     string
	IdentityId string
	SubType    interface{}
}

func (entity *Authenticator) Fingerprints() []string {
	switch entity.SubType.(type) {
	case *AuthenticatorCert:
		cert, _ := entity.SubType.(*AuthenticatorCert)
		return []string{cert.Fingerprint}
	default:
		return nil
	}
}

func (entity *Authenticator) FillFrom(handler Handler, tx *bbolt.Tx, boltEntity boltz.BaseEntity) error {
	boltAuthenticator, ok := boltEntity.(*persistence.Authenticator)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model authenticator", reflect.TypeOf(boltEntity))
	}
	entity.fillCommon(boltAuthenticator)
	entity.Method = boltAuthenticator.Type
	entity.IdentityId = boltAuthenticator.IdentityId

	boltSubType := boltAuthenticator.ToSubType()

	switch bothAuth := boltSubType.(type) {
	case *persistence.AuthenticatorUpdb:
		entity.SubType = &AuthenticatorUpdb{
			Authenticator: entity,
			Username:      bothAuth.Username,
			Password:      bothAuth.Password,
			Salt:          bothAuth.Salt,
		}
	case *persistence.AuthenticatorCert:
		entity.SubType = &AuthenticatorCert{
			Authenticator: entity,
			Fingerprint:   bothAuth.Fingerprint}
	default:
		pfxlog.Logger().Panicf("unexpected type %v when filling model %s", reflect.TypeOf(boltSubType), "authenticator")
	}

	return nil
}

func (entity *Authenticator) ToBoltEntityForCreate(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	boltEntity := &persistence.Authenticator{
		BaseEdgeEntityImpl: *persistence.NewBaseEdgeEntity(entity.Id, entity.Tags),
		Type:               entity.Method,
		IdentityId:         entity.IdentityId,
	}

	var subType persistence.AuthenticatorSubType

	switch entity.SubType.(type) {
	case *AuthenticatorUpdb:
		updbModel, ok := entity.SubType.(*AuthenticatorUpdb)

		if !ok {
			pfxlog.Logger().Panicf("unexpected type assertion failure to updb authenticator conversion to bolt model for type %s", reflect.TypeOf(entity.SubType))
		}

		subType = &persistence.AuthenticatorUpdb{
			Authenticator: *boltEntity,
			Username:      updbModel.Username,
			Password:      updbModel.Password,
			Salt:          updbModel.Salt,
		}
	case *AuthenticatorCert:
		certModel, ok := entity.SubType.(*AuthenticatorCert)

		if !ok {
			pfxlog.Logger().Panicf("unexpected type assertion failure to cert authenticator conversion to bolt model for type %s", reflect.TypeOf(entity.SubType))
		}

		subType = &persistence.AuthenticatorCert{
			Authenticator: *boltEntity,
			Fingerprint:   certModel.Fingerprint,
		}

	default:
		pfxlog.Logger().Panicf("unexpected type %v when converting to bolt model authenticator", reflect.TypeOf(entity.SubType))
	}

	boltEntity.SubType = subType

	return boltEntity, nil
}

func (entity *Authenticator) ToBoltEntityForUpdate(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	return entity.ToBoltEntityForCreate(tx, handler)

}

func (entity *Authenticator) ToBoltEntityForPatch(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	return entity.ToBoltEntityForUpdate(tx, handler)
}

func (entity *Authenticator) ToCert() *AuthenticatorCert {
	cert, ok := entity.SubType.(*AuthenticatorCert)

	if !ok {
		pfxlog.Logger().Panicf("unexpected type assertion failure for authenticator cert sub type %s", reflect.TypeOf(entity.SubType))
	}
	cert.Authenticator = entity

	return cert
}

func (entity *Authenticator) ToUpdb() *AuthenticatorUpdb {
	updb, ok := entity.SubType.(*AuthenticatorUpdb)

	if !ok {
		pfxlog.Logger().Panicf("unexpected type assertion failure for authenticator cert sub type %s", reflect.TypeOf(entity.SubType))
	}
	updb.Authenticator = entity

	return updb
}

type AuthenticatorCert struct {
	*Authenticator
	Fingerprint string
}

type AuthenticatorUpdb struct {
	*Authenticator
	Username string
	Password string
	Salt     string
}
