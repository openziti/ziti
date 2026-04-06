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
	"fmt"
	"math/big"
	"sync/atomic"
	"time"

	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/v2/common/capabilities"
	"github.com/openziti/ziti/v2/common/ctrlchan"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/controller/db"
	"github.com/openziti/ziti/v2/controller/models"
	"go.etcd.io/bbolt"
)

type Listener interface {
	AdvertiseAddress() string
	Protocol() string
	Groups() []string
}

type Router struct {
	models.BaseEntity
	Name              string
	Fingerprint       *string
	Listeners         []*ctrl_pb.Listener
	Control           ctrlchan.CtrlChannel
	Connected         atomic.Bool
	ConnectTime       time.Time
	VersionInfo       *versions.VersionInfo
	routerLinks       RouterLinks
	Cost              uint16
	NoTraversal       bool
	Disabled          bool
	Capabilities      *big.Int
	Interfaces        []*Interface
	CtrlChanListeners map[string][]string
	Configs           []string
}

func (entity *Router) GetLinks() []*Link {
	return entity.routerLinks.GetLinks()
}

func (entity *Router) toBoltEntityForUpdate(tx *bbolt.Tx, env Env, _ boltz.FieldChecker) (*db.Router, error) {
	return entity.toBoltEntityForCreate(tx, env)
}

func (entity *Router) toBoltEntityForCreate(tx *bbolt.Tx, env Env) (*db.Router, error) {
	if err := validateRouterConfigs(tx, env, entity.Configs); err != nil {
		return nil, err
	}
	return &db.Router{
		BaseExtEntity:     *boltz.NewExtEntity(entity.Id, entity.Tags),
		Name:              entity.Name,
		Fingerprint:       entity.Fingerprint,
		Cost:              entity.Cost,
		NoTraversal:       entity.NoTraversal,
		Disabled:          entity.Disabled,
		CtrlChanListeners: entity.CtrlChanListeners,
		Interfaces:        InterfacesToBolt(entity.Interfaces),
		Configs:           entity.Configs,
	}, nil
}

// validateRouterConfigs validates that all configs target routers and have unique config types.
func validateRouterConfigs(tx *bbolt.Tx, env Env, configs []string) error {
	if len(configs) == 0 {
		return nil
	}

	typeMap := map[string]*db.Config{}
	configStore := env.GetStores().Config
	configTypeStore := env.GetStores().ConfigType
	for _, id := range configs {
		config, _ := configStore.LoadById(tx, id)
		if config == nil {
			return boltz.NewNotFoundError(db.EntityTypeConfigs, "id", id)
		}

		configType, _ := configTypeStore.LoadById(tx, config.TypeId)
		configTypeName := "<not found>"
		if configType != nil {
			configTypeName = configType.Name
			if configType.Target == nil || *configType.Target != db.ConfigTypeTargetRouter {
				msg := fmt.Sprintf("config %v has config type %v which does not target routers",
					config.Name, configTypeName)
				return errorz.NewFieldError(msg, "configs", configs)
			}
		}

		if conflictConfig, found := typeMap[config.TypeId]; found {
			msg := fmt.Sprintf("duplicate configs named %v and %v found for config type %v. Only one config of a given type is allowed per router",
				conflictConfig.Name, config.Name, configTypeName)
			return errorz.NewFieldError(msg, "configs", configs)
		}
		typeMap[config.TypeId] = config
	}
	return nil
}

func (entity *Router) fillFrom(_ Env, _ *bbolt.Tx, boltRouter *db.Router) error {
	entity.Name = boltRouter.Name
	entity.Fingerprint = boltRouter.Fingerprint
	entity.Cost = boltRouter.Cost
	entity.NoTraversal = boltRouter.NoTraversal
	entity.Disabled = boltRouter.Disabled
	entity.CtrlChanListeners = boltRouter.CtrlChanListeners
	entity.Interfaces = InterfacesFromBolt(boltRouter.Interfaces)
	entity.Configs = boltRouter.Configs
	entity.FillCommon(boltRouter)
	return nil
}

func (entity *Router) AddLinkListener(addr, linkProtocol string, linkCostTags []string, groups []string) {
	entity.Listeners = append(entity.Listeners, &ctrl_pb.Listener{
		Address:  addr,
		Protocol: linkProtocol,
		CostTags: linkCostTags,
		Groups:   groups,
	})
}

func (entity *Router) SetLinkListeners(listeners []*ctrl_pb.Listener) {
	entity.Listeners = listeners
}

func (entity *Router) HasCapability(capability int) bool {
	return entity.Capabilities != nil && capabilities.IsSet(entity.Capabilities, capability)
}
