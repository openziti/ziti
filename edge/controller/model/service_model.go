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
	"github.com/netfoundry/ziti-edge/edge/controller/persistence"
	"github.com/netfoundry/ziti-fabric/controller/network"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
	"strings"
)

type Service struct {
	BaseModelEntityImpl
	Name            string   `json:"name"`
	DnsHostname     string   `json:"hostname"`
	DnsPort         uint16   `json:"port"`
	EgressRouter    string   `json:"egressRouter"`
	EndpointAddress string   `json:"endpointAddress"`
	HostIds         []string `json:"hostIds"`
	EdgeRouterRoles []string `json:"edgeRouterRoles"`
	RoleAttributes  []string `json:"roleAttributes"`
}

func (entity *Service) ToBoltEntityForCreate(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	entity.Sanitize()

	// validate identities exist
	if err := ValidateEntityList(tx, handler.GetEnv().GetStores().Identity, "hostIds", entity.HostIds); err != nil {
		return nil, err
	}

	binding := "transport"
	if strings.HasPrefix(entity.EndpointAddress, "hosted") {
		binding = "edge"
	} else if strings.HasPrefix(entity.EndpointAddress, "udp") {
		binding = "udp"
	}

	edgeService := &persistence.EdgeService{
		Service: network.Service{
			Id:              entity.Id,
			Binding:         binding,
			EndpointAddress: entity.EndpointAddress,
			Egress:          entity.EgressRouter,
		},
		EdgeEntityFields: persistence.EdgeEntityFields{Tags: entity.Tags},
		Name:             entity.Name,
		DnsHostname:      entity.DnsHostname,
		DnsPort:          entity.DnsPort,
		HostIds:          entity.HostIds,
		EdgeRouterRoles:  entity.EdgeRouterRoles,
		RoleAttributes:   entity.RoleAttributes,
	}

	return edgeService, nil
}

func (entity *Service) ToBoltEntityForUpdate(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	return entity.ToBoltEntityForCreate(tx, handler)
}

func (entity *Service) ToBoltEntityForPatch(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	return entity.ToBoltEntityForCreate(tx, handler)
}

func (entity *Service) Sanitize() {
	entity.EndpointAddress = strings.Replace(entity.EndpointAddress, "://", ":", 1)
}

func (entity *Service) FillFrom(_ Handler, _ *bbolt.Tx, boltEntity boltz.BaseEntity) error {
	boltService, ok := boltEntity.(*persistence.EdgeService)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model service", reflect.TypeOf(boltEntity))
	}
	entity.fillCommon(boltService)
	entity.Name = boltService.Name
	entity.HostIds = boltService.HostIds
	entity.DnsHostname = boltService.DnsHostname
	entity.DnsPort = boltService.DnsPort
	entity.EdgeRouterRoles = boltService.EdgeRouterRoles
	entity.EgressRouter = boltService.Egress
	entity.EndpointAddress = boltService.EndpointAddress
	entity.RoleAttributes = boltService.RoleAttributes
	return nil
}
