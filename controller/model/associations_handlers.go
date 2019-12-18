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
	"github.com/netfoundry/ziti-edge/controller/persistence"
	"github.com/netfoundry/ziti-edge/controller/util"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

type AssociationAction int

const (
	AssociationsActionAdd AssociationAction = iota
	AssociationsActionRemove
	AssociationsActionSet
)

func NewAssociationsHandler(env Env) *AssociationsHandler {
	return &AssociationsHandler{env: env}
}

type AssociationsHandler struct {
	env Env
}

func (handler *AssociationsHandler) UpdateAssociations(store persistence.Store, action AssociationAction, association string, parentId string, childIds ...string) error {
	linkCollection := store.GetLinkCollection(association)
	if linkCollection == nil {
		return errors.Errorf("unknown association '%v' on entity type '%v'", association, store.GetEntityType())
	}

	return handler.env.GetDbProvider().GetDb().Update(func(tx *bbolt.Tx) error {
		if !store.IsEntityPresent(tx, parentId) {
			return util.NewNotFoundError(store.GetEntityType(), "id", parentId)
		}

		if err := ValidateEntityList(tx, linkCollection.GetLinkedSymbol().GetStore(), association, childIds); err != nil {
			return err
		}

		switch action {
		case AssociationsActionAdd:
			return linkCollection.AddLinks(tx, parentId, childIds...)
		case AssociationsActionRemove:
			return linkCollection.RemoveLinks(tx, parentId, childIds...)
		default:
			return linkCollection.SetLinks(tx, parentId, childIds)
		}
	})
}
