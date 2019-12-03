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
	"github.com/netfoundry/ziti-fabric/fabric/controller/network"
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
	Clusters        []string `json:"clusters"`
	HostIds         []string `json:"hostIds"`
}

func (entity *Service) ToBoltEntityForCreate(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	entity.Sanitize()

	// validate clusters exist
	if err := ValidateEntityList(tx, handler.GetEnv().GetStores().Cluster, "clusters", entity.Clusters); err != nil {
		return nil, err
	}

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
		Clusters:         entity.Clusters,
		HostIds:          entity.HostIds,
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

func (entity *Service) FillFrom(handler Handler, tx *bbolt.Tx, boltEntity boltz.BaseEntity) error {
	boltService, ok := boltEntity.(*persistence.EdgeService)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model service", reflect.TypeOf(boltEntity))
	}
	entity.fillCommon(boltService)
	entity.Name = boltService.Name
	entity.Clusters = boltService.Clusters
	entity.HostIds = boltService.HostIds
	entity.DnsHostname = boltService.DnsHostname
	entity.DnsPort = boltService.DnsPort
	return nil
}