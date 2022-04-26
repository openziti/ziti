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

package persistence

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/util/errorz"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	"go.etcd.io/bbolt"
)

const (
	FieldAuthenticatorMethod   = "method"
	FieldAuthenticatorIdentity = "identity"

	FieldAuthenticatorCertFingerprint = "certFingerprint"
	FieldAuthenticatorCertPem         = "certPem"

	FieldAuthenticatorUnverifiedCertPem         = "unverifiedCertPem"
	FieldAuthenticatorUnverifiedCertFingerprint = "unverifiedCertFingerprint"

	FieldAuthenticatorUpdbUsername = "updbUsername"
	FieldAuthenticatorUpdbPassword = "updbPassword"
	FieldAuthenticatorUpdbSalt     = "updbSalt"

	MethodAuthenticatorUpdb = "updb"
	MethodAuthenticatorCert = "cert"
	// MethodAuthenticatorCertCaExternalId represents authentication with a certificate that isn't directly
	// registered with an authenticator. Instead, it uses `externalId` values on identities and matches them to a
	// "x509 claim" (custom values stuffed into SANs or other x509 properties). This type will never actually
	// be stored for persistence and is defined here for as tobe near the other authenticator methods.
	MethodAuthenticatorCertCaExternalId = "certCaExternalId"
)

type AuthenticatorSubType interface {
	Fingerprints() []string
}

type AuthenticatorCert struct {
	Authenticator
	Fingerprint string
	Pem         string

	UnverifiedPem         string
	UnverifiedFingerprint string
}

func (entity *AuthenticatorCert) Fingerprints() []string {
	return []string{entity.Fingerprint}
}

type AuthenticatorUpdb struct {
	Authenticator
	Username string
	Password string
	Salt     string
}

func (entity *AuthenticatorUpdb) Fingerprints() []string {
	return nil
}

type Authenticator struct {
	boltz.BaseExtEntity
	Type       string
	IdentityId string
	SubType    AuthenticatorSubType
}

var authenticatorFieldMappings = map[string]string{
	FieldAuthenticatorIdentity:        "identityId",
	FieldAuthenticatorUpdbPassword:    "password",
	FieldAuthenticatorUpdbUsername:    "username",
	FieldAuthenticatorUpdbSalt:        "salt",
	FieldAuthenticatorCertFingerprint: "fingerprint"}

func (entity *Authenticator) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.Type = bucket.GetStringOrError(FieldAuthenticatorMethod)
	entity.IdentityId = bucket.GetStringOrError(FieldAuthenticatorIdentity)

	if entity.Type == MethodAuthenticatorCert {
		authCert := &AuthenticatorCert{}
		authCert.Fingerprint = bucket.GetStringWithDefault(FieldAuthenticatorCertFingerprint, "")
		authCert.Pem = bucket.GetStringWithDefault(FieldAuthenticatorCertPem, "")

		authCert.UnverifiedPem = bucket.GetStringWithDefault(FieldAuthenticatorUnverifiedCertPem, "")
		authCert.UnverifiedFingerprint = bucket.GetStringWithDefault(FieldAuthenticatorUnverifiedCertFingerprint, "")
		entity.SubType = authCert
	} else if entity.Type == MethodAuthenticatorUpdb {
		authUpdb := &AuthenticatorUpdb{}
		authUpdb.Username = bucket.GetStringWithDefault(FieldAuthenticatorUpdbUsername, "")
		authUpdb.Password = bucket.GetStringWithDefault(FieldAuthenticatorUpdbPassword, "")
		authUpdb.Salt = bucket.GetStringWithDefault(FieldAuthenticatorUpdbSalt, "")
		entity.SubType = authUpdb
	}
}

func (entity *Authenticator) SetValues(ctx *boltz.PersistContext) {
	ctx.WithFieldOverrides(authenticatorFieldMappings)

	entity.SetBaseValues(ctx)
	ctx.SetString(FieldAuthenticatorMethod, entity.Type)
	ctx.SetString(FieldAuthenticatorIdentity, entity.IdentityId)

	if entity.Type == MethodAuthenticatorCert {
		if authCert, ok := entity.SubType.(*AuthenticatorCert); ok {
			ctx.SetString(FieldAuthenticatorCertFingerprint, authCert.Fingerprint)
			ctx.SetString(FieldAuthenticatorCertPem, authCert.Pem)

			ctx.SetString(FieldAuthenticatorUnverifiedCertFingerprint, authCert.UnverifiedFingerprint)
			ctx.SetString(FieldAuthenticatorUnverifiedCertPem, authCert.UnverifiedPem)
		} else {
			pfxlog.Logger().Panic("type conversion error setting values for AuthenticatorCert")
		}
	} else if entity.Type == MethodAuthenticatorUpdb {
		if authUpdb, ok := entity.SubType.(*AuthenticatorUpdb); ok {
			ctx.SetString(FieldAuthenticatorUpdbPassword, authUpdb.Password)
			ctx.SetString(FieldAuthenticatorUpdbUsername, authUpdb.Username)
			ctx.SetString(FieldAuthenticatorUpdbSalt, authUpdb.Salt)
		} else {
			pfxlog.Logger().Panic("type conversion error setting values for AuthenticatorUpdb")
		}
	}

	store := ctx.Store.(*authenticatorStoreImpl)
	_, identityId := store.symbolIdentityId.Eval(ctx.Tx(), []byte(entity.Id))
	if identityId != nil {
		identityType := boltz.FieldToString(store.stores.identity.symbolIdentityTypeId.Eval(ctx.Tx(), identityId))
		if identityType != nil && *identityType == RouterIdentityType {
			err := errorz.NewFieldError("may not create authenticators for router identities", "identityId", string(identityId))
			ctx.Bucket.SetError(errorz.NewFieldApiError(err))
		}
	}
}

func (entity *Authenticator) GetEntityType() string {
	return EntityTypeAuthenticators
}

func (entity *Authenticator) ToUpdb() *AuthenticatorUpdb {
	if updb, ok := entity.SubType.(*AuthenticatorUpdb); ok {
		updb.Authenticator = *entity
		return updb
	}
	return nil
}

func (entity *Authenticator) ToCert() *AuthenticatorCert {
	if cert, ok := entity.SubType.(*AuthenticatorCert); ok {
		cert.Authenticator = *entity
		return cert
	}
	return nil
}

func (entity *Authenticator) ToSubType() AuthenticatorSubType {
	if entity.Type == MethodAuthenticatorCert {
		return entity.ToCert()
	}
	if entity.Type == MethodAuthenticatorUpdb {
		return entity.ToUpdb()
	}

	pfxlog.Logger().Panicf("unknown authenticator subtype %s", entity.Type)
	return nil
}

type AuthenticatorStore interface {
	Store
	LoadOneById(tx *bbolt.Tx, id string) (*Authenticator, error)
	LoadOneByQuery(tx *bbolt.Tx, query string) (*Authenticator, error)
}

func newAuthenticatorStore(stores *stores) *authenticatorStoreImpl {
	store := &authenticatorStoreImpl{
		baseStore: newBaseStore(stores, EntityTypeAuthenticators),
	}
	store.InitImpl(store)

	return store
}

type authenticatorStoreImpl struct {
	*baseStore
	symbolIdentityId boltz.EntitySymbol
}

func (store *authenticatorStoreImpl) NewStoreEntity() boltz.Entity {
	return &Authenticator{}
}

func (store *authenticatorStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()

	store.AddSymbol(FieldAuthenticatorMethod, ast.NodeTypeString)
	store.AddSymbol(FieldAuthenticatorCertFingerprint, ast.NodeTypeString)
	store.AddSymbol(FieldAuthenticatorCertPem, ast.NodeTypeString)
	store.AddSymbol(FieldAuthenticatorUpdbUsername, ast.NodeTypeString)
	store.AddSymbol(FieldAuthenticatorUpdbPassword, ast.NodeTypeString)
	store.AddSymbol(FieldAuthenticatorUpdbSalt, ast.NodeTypeString)

	store.symbolIdentityId = store.AddFkSymbol(FieldAuthenticatorIdentity, store.stores.identity)
}

func (store *authenticatorStoreImpl) initializeLinked() {
	store.AddFkIndex(store.symbolIdentityId, store.stores.identity.symbolAuthenticators)
}

func (store *authenticatorStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*Authenticator, error) {
	entity := &Authenticator{}
	if err := store.baseLoadOneById(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *authenticatorStoreImpl) LoadOneByQuery(tx *bbolt.Tx, query string) (*Authenticator, error) {
	entity := &Authenticator{}
	if found, err := store.BaseLoadOneByQuery(tx, query, entity); !found || err != nil {
		return nil, err
	}
	return entity, nil
}
