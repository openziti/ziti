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
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/models"
	"go.etcd.io/bbolt"
	"sort"
	"time"
)

type Controller struct {
	models.BaseEntity
	Name         string
	CtrlAddress  string
	CertPem      string
	Fingerprint  string
	IsOnline     bool
	LastJoinedAt time.Time
	ApiAddresses map[string][]ApiAddress
}

func (entity *Controller) sortApiAddresses() {
	for _, v := range entity.ApiAddresses {
		sort.Slice(v, func(i, j int) bool {
			if v[i].Version < v[j].Version {
				return true
			}
			if v[i].Version > v[j].Version {
				return false
			}
			return v[i].Url < v[j].Url
		})
	}
}

func (entity *Controller) IsChanged(other *Controller) bool {
	if entity.Name != other.Name ||
		entity.CtrlAddress != other.CtrlAddress ||
		entity.CertPem != other.CertPem ||
		entity.Fingerprint != other.Fingerprint ||
		entity.IsOnline != other.IsOnline {
		return true
	}

	if len(entity.ApiAddresses) != len(other.ApiAddresses) {
		return true
	}

	entity.sortApiAddresses()
	other.sortApiAddresses()

	for k, v := range entity.ApiAddresses {
		v2, ok := other.ApiAddresses[k]
		if !ok {
			return true
		}
		if len(v) != len(v2) {
			return true
		}
		for idx, addr := range v {
			addr2 := v2[idx]
			if addr.Version != addr2.Version || addr.Url != addr2.Url {
				return true
			}
		}
	}

	return false
}

type ApiAddress struct {
	Url     string `json:"url"`
	Version string `json:"version"`
}

func (entity *Controller) toBoltEntity(tx *bbolt.Tx, env Env) (*db.Controller, error) {
	boltEntity := &db.Controller{
		BaseExtEntity: *boltz.NewExtEntity(entity.Id, entity.Tags),
		Name:          entity.Name,
		CtrlAddress:   entity.CtrlAddress,
		CertPem:       entity.CertPem,
		Fingerprint:   entity.Fingerprint,
		IsOnline:      entity.IsOnline,
		LastJoinedAt:  entity.LastJoinedAt,
		ApiAddresses:  map[string][]db.ApiAddress{},
	}

	for apiKey, instances := range entity.ApiAddresses {
		boltEntity.ApiAddresses[apiKey] = nil
		for _, instance := range instances {
			boltEntity.ApiAddresses[apiKey] = append(boltEntity.ApiAddresses[apiKey], db.ApiAddress{
				Url:     instance.Url,
				Version: instance.Version,
			})
		}
	}

	return boltEntity, nil
}

func (entity *Controller) toBoltEntityForCreate(tx *bbolt.Tx, env Env) (*db.Controller, error) {
	return entity.toBoltEntity(tx, env)
}

func (entity *Controller) toBoltEntityForUpdate(tx *bbolt.Tx, env Env, _ boltz.FieldChecker) (*db.Controller, error) {
	return entity.toBoltEntity(tx, env)
}

func (entity *Controller) fillFrom(env Env, tx *bbolt.Tx, boltController *db.Controller) error {
	entity.FillCommon(boltController)
	entity.Name = boltController.Name
	entity.CtrlAddress = boltController.CtrlAddress
	entity.CertPem = boltController.CertPem
	entity.Fingerprint = boltController.Fingerprint
	entity.IsOnline = boltController.IsOnline
	entity.LastJoinedAt = boltController.LastJoinedAt
	entity.ApiAddresses = map[string][]ApiAddress{}

	for apiKey, instances := range boltController.ApiAddresses {
		entity.ApiAddresses[apiKey] = nil
		for _, instance := range instances {
			entity.ApiAddresses[apiKey] = append(entity.ApiAddresses[apiKey], ApiAddress{
				Url:     instance.Url,
				Version: instance.Version,
			})
		}
	}

	return nil
}
