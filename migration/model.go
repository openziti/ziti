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

package migration

import (
	"github.com/kataras/go-events"
	"github.com/netfoundry/ziti-edge/controller/predicate"
	"time"
)

type ListStats struct {
	Count  int64
	Offset int64
	Limit  int64
}

type CreateStore interface {
	BaseCreate(e BaseDbModel) (string, error)
}

type Store interface {
	BaseLoadOneById(id string, pl *Preloads) (BaseDbModel, error)
	BaseLoadOne(p *predicate.Predicate, pl *Preloads) (BaseDbModel, error)
	BaseLoadList(qo *QueryOptions) ([]BaseDbModel, error)
	BaseStatsList(qo *QueryOptions) (*ListStats, error)
	BaseCreate(e BaseDbModel) (string, error)
	BaseDeleteById(id string) error
	BaseDeleteWhere(p *predicate.Predicate) (int64, error)
	BaseIdentifierMap() *predicate.IdentifierMap
	BaseUpdate(e BaseDbModel) error
	BasePatch(e BaseDbModel) error
	EntityName() string
	PluralEntityName() string
	events.EventEmmiter
}

type BaseDbModel interface {
	GetId() string
	SetId(id string)
	SetUpdatedAt(t *time.Time)
	GetTags() *PropertyMap
	SetTags(tags *PropertyMap)
}

type ModelHandlers struct {
	Identity      *IdentityHandlers
	Ca            *CaHandlers
	Authenticator *AuthenticatorHandlers
	Enrollment    *EnrollmentHandlers
}

var modelHandlers = &ModelHandlers{
	Identity:      identityHandlersInstance,
	Ca:            caHandlersInstance,
	Authenticator: authenticatorHandlersInstance,
	Enrollment:    enrollmentHandlersInstance,
}

func GetModelHandlers() *ModelHandlers {
	return modelHandlers
}
