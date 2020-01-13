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
	"reflect"
	"strings"

	"github.com/netfoundry/ziti-edge/controller/persistence"
	"github.com/netfoundry/ziti-fabric/controller/network"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

type Service struct {
	BaseModelEntityImpl
	Name            string   `json:"name"`
	DnsHostname     string   `json:"hostname"`
	DnsPort         uint16   `json:"port"`
	EgressRouter    string   `json:"egressRouter"`
	EndpointAddress string   `json:"endpointAddress"`
	EdgeRouterRoles []string `json:"edgeRouterRoles"`
	RoleAttributes  []string `json:"roleAttributes"`
	Permissions     []string `json:"permissions"` // used on read to indicate if an identity has dial/bind permissions
	Configs         []string `json:"configs"`
}

func (entity *Service) ToBoltEntityForCreate(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	entity.Sanitize()
	if err := entity.mapConfigTypeNamesToIds(tx, handler); err != nil {
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
		EdgeRouterRoles:  entity.EdgeRouterRoles,
		RoleAttributes:   entity.RoleAttributes,
		Configs:          entity.Configs,
	}

	return edgeService, nil
}

func (entity *Service) mapConfigTypeNamesToIds(tx *bbolt.Tx, handler Handler) error {
	typeMap := map[string]*persistence.Config{}
	configStore := handler.GetEnv().GetStores().Config
	for idx, val := range entity.Configs {
		if !configStore.IsEntityPresent(tx, val) {
			id := configStore.GetNameIndex().Read(tx, []byte(val))
			if id == nil || !configStore.IsEntityPresent(tx, string(id)) {
				return NewFieldError(fmt.Sprintf("%v is not a valid config id or name", val), "configs", entity.Configs)
			}
			entity.Configs[idx] = string(id)
		}
		config, _ := configStore.LoadOneById(tx, entity.Configs[idx])
		if config == nil {
			return NewFieldError(fmt.Sprintf("%v is not a valid config id or name", val), "configs", entity.Configs)
		}
		conflictConfig, found := typeMap[config.Type]
		if found {
			configTypeName := "<not found>"
			if configType, _ := handler.GetEnv().GetStores().ConfigType.LoadOneById(tx, config.Type); configType != nil {
				configTypeName = configType.Name
			}
			msg := fmt.Sprintf("duplicate configs named %v and %v found for config type %v. Only one config of a given typed is allowed per service ",
				conflictConfig.Name, config.Name, configTypeName)
			return NewFieldError(msg, "configs", entity.Configs)
		}
		typeMap[config.Type] = config
	}
	return nil
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
	entity.DnsHostname = boltService.DnsHostname
	entity.DnsPort = boltService.DnsPort
	entity.EdgeRouterRoles = boltService.EdgeRouterRoles
	entity.EgressRouter = boltService.Egress
	entity.EndpointAddress = boltService.EndpointAddress
	entity.RoleAttributes = boltService.RoleAttributes
	entity.Configs = boltService.Configs
	return nil
}
