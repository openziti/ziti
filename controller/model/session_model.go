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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/util/errorz"
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
	"time"
)

type Session struct {
	models.BaseEntity
	Token           string
	IdentityId      string
	ApiSessionId    string
	ServiceId       string
	Type            string
	SessionCerts    []*SessionCert
	ServicePolicies []string
}

type SessionCert struct {
	Cert        string
	Fingerprint string
	ValidFrom   time.Time
	ValidTo     time.Time
}

func (entity *Session) toBoltEntityForCreate(tx *bbolt.Tx, handler EntityManager) (boltz.Entity, error) {
	apiSession, err := handler.GetEnv().GetStores().ApiSession.LoadOneById(tx, entity.ApiSessionId)
	if err != nil {
		return nil, err
	}
	if apiSession == nil {
		return nil, errorz.NewFieldError("api session not found", "ApiSessionId", entity.ApiSessionId)
	}

	boltEntity := &persistence.Session{
		BaseExtEntity:   *boltz.NewExtEntity(entity.Id, entity.Tags),
		Token:           entity.Token,
		ApiSessionId:    entity.ApiSessionId,
		IdentityId:      entity.IdentityId,
		ServiceId:       entity.ServiceId,
		Type:            entity.Type,
		ApiSession:      apiSession,
		ServicePolicies: entity.ServicePolicies,
	}

	identity, err := handler.GetEnv().GetStores().Identity.LoadOneById(tx, apiSession.IdentityId)

	if err != nil {
		return nil, err
	}

	fingerprints := map[string]string{}

	for _, authenticatorId := range identity.Authenticators {
		authenticator, err := handler.GetEnv().GetStores().Authenticator.LoadOneById(tx, authenticatorId)
		if err != nil {
			pfxlog.Logger().Errorf("encountered error retrieving fingerprints for authenticator [%s]", authenticatorId)
			continue
		}
		if certAuth := authenticator.ToCert(); certAuth != nil {
			fingerprints[certAuth.Fingerprint] = certAuth.Pem
		}
	}

	for fingerprint, cert := range fingerprints {
		validFrom := time.Now()
		validTo := time.Now().AddDate(1, 0, 0)

		boltEntity.Certs = append(boltEntity.Certs, &persistence.SessionCert{
			Cert:        cert,
			Fingerprint: fingerprint,
			ValidFrom:   validFrom,
			ValidTo:     validTo,
		})
	}

	return boltEntity, nil
}

func (entity *Session) toBoltEntityForUpdate(*bbolt.Tx, EntityManager) (boltz.Entity, error) {
	return &persistence.Session{
		BaseExtEntity:   *boltz.NewExtEntity(entity.Id, entity.Tags),
		Token:           entity.Token,
		ApiSessionId:    entity.ApiSessionId,
		IdentityId:      entity.IdentityId,
		ServiceId:       entity.ServiceId,
		Type:            entity.Type,
		ServicePolicies: entity.ServicePolicies,
	}, nil
}

func (entity *Session) toBoltEntityForPatch(tx *bbolt.Tx, handler EntityManager, checker boltz.FieldChecker) (boltz.Entity, error) {
	return entity.toBoltEntityForUpdate(tx, handler)
}

func (entity *Session) fillFrom(_ EntityManager, _ *bbolt.Tx, boltEntity boltz.Entity) error {
	boltSession, ok := boltEntity.(*persistence.Session)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model Session", reflect.TypeOf(boltEntity))
	}
	entity.FillCommon(boltSession)
	entity.Token = boltSession.Token
	entity.ApiSessionId = boltSession.ApiSessionId
	entity.IdentityId = boltSession.IdentityId
	entity.ServiceId = boltSession.ServiceId
	entity.Type = boltSession.Type
	entity.ServicePolicies = boltSession.ServicePolicies
	return nil
}

func (entity *SessionCert) FillFrom(_ EntityManager, _ *bbolt.Tx, boltEntity boltz.Entity) error {
	boltSessionCert, ok := boltEntity.(*persistence.SessionCert)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model SessionCert", reflect.TypeOf(boltEntity))
	}
	entity.Fingerprint = boltSessionCert.Fingerprint
	entity.Cert = boltSessionCert.Cert
	entity.ValidFrom = boltSessionCert.ValidFrom
	entity.ValidTo = boltSessionCert.ValidTo
	return nil
}
