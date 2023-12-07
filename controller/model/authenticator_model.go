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

package model

import (
	"encoding/base64"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/models"
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

func (entity *Authenticator) fillFrom(_ Env, _ *bbolt.Tx, boltAuthenticator *db.Authenticator) error {
	entity.FillCommon(boltAuthenticator)
	entity.Method = boltAuthenticator.Type
	entity.IdentityId = boltAuthenticator.IdentityId

	boltSubType := boltAuthenticator.ToSubType()

	switch boltAuth := boltSubType.(type) {
	case *db.AuthenticatorUpdb:
		entity.SubType = &AuthenticatorUpdb{
			Authenticator: entity,
			Username:      boltAuth.Username,
			Password:      boltAuth.Password,
			Salt:          boltAuth.Salt,
		}
	case *db.AuthenticatorCert:
		entity.SubType = &AuthenticatorCert{
			Authenticator: entity,
			Fingerprint:   boltAuth.Fingerprint,
			Pem:           boltAuth.Pem,

			UnverifiedPem:         boltAuth.UnverifiedPem,
			UnverifiedFingerprint: boltAuth.UnverifiedFingerprint,
		}
	default:
		pfxlog.Logger().Panicf("unexpected type %v when filling model %s", reflect.TypeOf(boltSubType), "authenticator")
	}

	return nil
}

func (entity *Authenticator) toBoltEntity() (*db.Authenticator, error) {
	boltEntity := &db.Authenticator{
		BaseExtEntity: *boltz.NewExtEntity(entity.Id, entity.Tags),
		Type:          entity.Method,
		IdentityId:    entity.IdentityId,
	}

	var subType db.AuthenticatorSubType

	switch entity.SubType.(type) {
	case *AuthenticatorUpdb:
		updbModel, ok := entity.SubType.(*AuthenticatorUpdb)

		if !ok {
			pfxlog.Logger().Panicf("unexpected type assertion failure to updb authenticator conversion to bolt model for type %s", reflect.TypeOf(entity.SubType))
		}

		subType = &db.AuthenticatorUpdb{
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

		subType = &db.AuthenticatorCert{
			Authenticator:         *boltEntity,
			Fingerprint:           certModel.Fingerprint,
			Pem:                   certModel.Pem,
			UnverifiedFingerprint: certModel.UnverifiedFingerprint,
			UnverifiedPem:         certModel.UnverifiedPem,
		}

	default:
		pfxlog.Logger().Panicf("unexpected type %v when converting to bolt model authenticator", reflect.TypeOf(entity.SubType))
	}

	boltEntity.SubType = subType

	return boltEntity, nil
}

func (entity *Authenticator) toBoltEntityForCreate(*bbolt.Tx, Env) (*db.Authenticator, error) {
	return entity.toBoltEntity()
}

func (entity *Authenticator) toBoltEntityForUpdate(*bbolt.Tx, Env, boltz.FieldChecker) (*db.Authenticator, error) {
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

	UnverifiedFingerprint string
	UnverifiedPem         string
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
