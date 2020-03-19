/*
	Copyright 2019 NetFoundry, Inc.

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
	"encoding/base64"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/persistence"
	"github.com/netfoundry/ziti-fabric/controller/models"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
)

type Authenticator struct {
	models.BaseEntity
	Method     string
	IdentityId string
	SubType    interface{}
}

type AuthenticatorSelf struct {
	models.BaseEntity
	CurrentPassword string
	NewPassword     string
	IdentityId      string
	Username        string
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

func (entity *Authenticator) fillFrom(_ Handler, _ *bbolt.Tx, boltEntity boltz.Entity) error {
	boltAuthenticator, ok := boltEntity.(*persistence.Authenticator)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model authenticator", reflect.TypeOf(boltEntity))
	}
	entity.FillCommon(boltAuthenticator)
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
			Fingerprint:   bothAuth.Fingerprint,
			Pem:           bothAuth.Pem}
	default:
		pfxlog.Logger().Panicf("unexpected type %v when filling model %s", reflect.TypeOf(boltSubType), "authenticator")
	}

	return nil
}

func (entity *Authenticator) toBoltEntity() (boltz.Entity, error) {
	boltEntity := &persistence.Authenticator{
		BaseExtEntity: *boltz.NewExtEntity(entity.Id, entity.Tags),
		Type:          entity.Method,
		IdentityId:    entity.IdentityId,
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
			Pem:           certModel.Pem,
		}

	default:
		pfxlog.Logger().Panicf("unexpected type %v when converting to bolt model authenticator", reflect.TypeOf(entity.SubType))
	}

	boltEntity.SubType = subType

	return boltEntity, nil
}

func (entity *Authenticator) toBoltEntityForCreate(*bbolt.Tx, Handler) (boltz.Entity, error) {
	return entity.toBoltEntity()
}

func (entity *Authenticator) toBoltEntityForUpdate(*bbolt.Tx, Handler) (boltz.Entity, error) {
	return entity.toBoltEntity()
}

func (entity *Authenticator) toBoltEntityForPatch(*bbolt.Tx, Handler) (boltz.Entity, error) {
	return entity.toBoltEntity()
}

func (entity *Authenticator) ToCert() *AuthenticatorCert {
	cert, ok := entity.SubType.(*AuthenticatorCert)

	if !ok {
		return nil
	}
	cert.Authenticator = entity

	return cert
}

func (entity *Authenticator) ToUpdb() *AuthenticatorUpdb {
	updb, ok := entity.SubType.(*AuthenticatorUpdb)

	if !ok {
		return nil
	}
	updb.Authenticator = entity

	return updb
}

type AuthenticatorCert struct {
	*Authenticator
	Fingerprint string
	Pem         string
}

type AuthenticatorUpdb struct {
	*Authenticator
	Username string
	Password string
	Salt     string
}

func (au *AuthenticatorUpdb) DecodedSalt() []byte {
	result, _ := base64.StdEncoding.DecodeString(au.Salt)
	return result
}
