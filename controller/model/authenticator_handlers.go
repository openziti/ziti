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
	"fmt"
	"github.com/netfoundry/ziti-edge/controller/apierror"
	"github.com/netfoundry/ziti-edge/controller/persistence"
	"go.etcd.io/bbolt"
	"reflect"
)

type AuthenticatorHandler struct {
	baseHandler
	authStore persistence.AuthenticatorStore
}

func (handler AuthenticatorHandler) IsUpdated(_ string) bool {
	return true
}

func NewAuthenticatorHandler(env Env) *AuthenticatorHandler {
	handler := &AuthenticatorHandler{
		baseHandler: baseHandler{
			env:   env,
			store: env.GetStores().Authenticator,
		},
		authStore: env.GetStores().Authenticator,
	}

	handler.impl = handler
	return handler
}

func (handler AuthenticatorHandler) newModelEntity() boltEntitySink {
	return &Authenticator{}
}

func (handler AuthenticatorHandler) IsAuthorized(authContext AuthContext) (*Identity, error) {

	authModule := handler.env.GetAuthRegistry().GetByMethod(authContext.GetMethod())

	if authModule == nil {
		return nil, apierror.NewInvalidAuthMethod()
	}

	identityId, err := authModule.Process(authContext)

	if err != nil {
		return nil, err
	}

	if identityId == "" {
		return nil, apierror.NewInvalidAuth()
	}

	return handler.env.GetHandlers().Identity.Read(identityId)
}

func (handler AuthenticatorHandler) ReadFingerprints(authenticatorId string) ([]string, error) {
	var authenticator *persistence.Authenticator

	err := handler.env.GetStores().DbProvider.GetDb().View(func(tx *bbolt.Tx) error {
		var err error
		authenticator, err = handler.authStore.LoadOneById(tx, authenticatorId)
		return err
	})

	if err != nil {
		return nil, err
	}

	return authenticator.ToSubType().Fingerprints(), nil
}

func (handler *AuthenticatorHandler) Read(id string) (*Authenticator, error) {
	modelEntity := &Authenticator{}
	if err := handler.readEntity(id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *AuthenticatorHandler) Create(authenticator *Authenticator) (string, error) {
	return handler.createEntity(authenticator)
}

func (handler AuthenticatorHandler) ReadByUsername(username string) (*Authenticator, error) {
	query := fmt.Sprintf("%s = \"%v\"", persistence.FieldAuthenticatorUpdbUsername, username)

	entity, err := handler.readEntityByQuery(query)

	if err != nil {
		return nil, err
	}

	authenticator, ok := entity.(*Authenticator)

	if !ok {
		return nil, fmt.Errorf("could not cast from %v to authenticator", reflect.TypeOf(entity))
	}

	return authenticator, nil
}

func (handler AuthenticatorHandler) ReadByFingerprint(fingerprint string) (*Authenticator, error) {
	query := fmt.Sprintf("%s = \"%v\"", persistence.FieldAuthenticatorCertFingerprint, fingerprint)

	entity, err := handler.readEntityByQuery(query)

	if err != nil {
		return nil, err
	}

	authenticator, ok := entity.(*Authenticator)

	if !ok {
		return nil, fmt.Errorf("could not cast from %v to authenticator", reflect.TypeOf(entity))
	}

	return authenticator, nil
}

func (handler AuthenticatorHandler) Update(authenticator *Authenticator) error {
	return handler.updateEntity(authenticator, handler)
}
