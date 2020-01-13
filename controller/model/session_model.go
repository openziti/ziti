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
	"github.com/netfoundry/ziti-edge/controller/apierror"
	"reflect"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/persistence"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/netfoundry/ziti-foundation/util/stringz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

type Session struct {
	BaseModelEntityImpl
	Token        string
	ApiSessionId string
	ServiceId    string
	IsHosting    bool
	SessionCerts []*SessionCert
}

type SessionCert struct {
	Cert        string
	Fingerprint string
	ValidFrom   time.Time
	ValidTo     time.Time
}

func (entity *Session) ToBoltEntityForCreate(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	apiSession, err := handler.GetEnv().GetStores().ApiSession.LoadOneById(tx, entity.ApiSessionId)
	if err != nil {
		return nil, err
	}
	if apiSession == nil {
		return nil, NewFieldError("api session not found", "ApiSessionId", entity.ApiSessionId)
	}

	service, err := handler.GetEnv().GetHandlers().Service.ReadForIdentity(entity.ServiceId, apiSession.IdentityId)
	if err != nil {
		return nil, err
	}

	if !entity.IsHosting && !stringz.Contains(service.Permissions, persistence.PolicyTypeDialName) {
		return nil, NewFieldError("service not found", "ServiceId", entity.ServiceId)
	}

	if entity.IsHosting && !stringz.Contains(service.Permissions, persistence.PolicyTypeBindName) {
		return nil, NewFieldError("service not found", "ServiceId", entity.ServiceId)
	}

	maxRows := 1
	result, err := handler.GetEnv().GetHandlers().EdgeRouter.ListForIdentityAndServiceWithTx(tx, apiSession.IdentityId, entity.ServiceId, &maxRows)
	if err != nil {
		return nil, err
	}
	if result.Count < 1 {
		return nil, apierror.NewNoEdgeRoutersAvailable()
	}

	boltEntity := &persistence.Session{
		BaseEdgeEntityImpl: *persistence.NewBaseEdgeEntity(entity.Id, entity.Tags),
		Token:              entity.Token,
		ApiSessionId:       entity.ApiSessionId,
		ServiceId:          entity.ServiceId,
		IsHosting:          entity.IsHosting,
	}

	identity, err := handler.GetEnv().GetStores().Identity.LoadOneById(tx, apiSession.IdentityId)

	if err != nil {
		return nil, err
	}

	fingerprints := map[string]bool{}

	for _, authenticatorId := range identity.Authenticators {
		authPrints, err := handler.GetEnv().GetHandlers().Authenticator.ReadFingerprints(authenticatorId)
		if err != nil {
			pfxlog.Logger().Errorf("encountered error retrieving fingerprints for authenticator [%s]", authenticatorId)
			continue
		}
		for _, fingerprint := range authPrints {
			fingerprints[fingerprint] = true
		}
	}
	for fingerprint := range fingerprints {
		validFrom := time.Now()
		validTo := time.Now().AddDate(1, 0, 0)
		cert := "unknown"

		boltEntity.Certs = append(boltEntity.Certs, &persistence.SessionCert{
			Cert:        cert,
			Fingerprint: fingerprint,
			ValidFrom:   validFrom,
			ValidTo:     validTo,
		})
	}

	return boltEntity, nil
}

func (entity *Session) ToBoltEntityForUpdate(_ *bbolt.Tx, _ Handler) (persistence.BaseEdgeEntity, error) {
	return &persistence.Session{
		BaseEdgeEntityImpl: *persistence.NewBaseEdgeEntity(entity.Id, entity.Tags),
		Token:              entity.Token,
		ApiSessionId:       entity.ApiSessionId,
		ServiceId:          entity.ServiceId,
		IsHosting:          entity.IsHosting,
	}, nil
}

func (entity *Session) ToBoltEntityForPatch(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	return entity.ToBoltEntityForUpdate(tx, handler)
}

func (entity *Session) FillFrom(_ Handler, _ *bbolt.Tx, boltEntity boltz.BaseEntity) error {
	boltSession, ok := boltEntity.(*persistence.Session)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model Session", reflect.TypeOf(boltEntity))
	}
	entity.fillCommon(boltSession)
	entity.Token = boltSession.Token
	entity.ApiSessionId = boltSession.ApiSessionId
	entity.ServiceId = boltSession.ServiceId
	entity.IsHosting = boltSession.IsHosting
	return nil
}

func (entity *SessionCert) FillFrom(_ Handler, _ *bbolt.Tx, boltEntity boltz.BaseEntity) error {
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
