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
	"sync"
	"sync/atomic"
	"time"

	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/ziti/v2/common/capabilities"
	"github.com/openziti/ziti/v2/common/ctrlchan"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/controller/db"
	"github.com/openziti/ziti/v2/controller/models"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"go.etcd.io/bbolt"
)

type Router struct {
	models.BaseEntity
	Name        string
	Fingerprint *string
	// listeners is the router's currently-advertised link listener set.
	// Hello carries the initial snapshot; UpdateLinkListeners pushes
	// mid-session changes from the router. Guarded by mu, so it is
	// unexported: reach it through SetLinkListeners / GetLinkListeners.
	listeners []*ctrl_pb.Listener

	// mu guards the fields a connected Router mutates mid-session, which
	// today is just listeners. Fields added later should share it rather
	// than take their own lock; contention is irrelevant at this scale.
	mu sync.RWMutex

	Control     ctrlchan.CtrlChannel
	Connected   atomic.Bool
	ConnectTime time.Time
	// VersionInfo is reported in the router's hello and is not persisted, so it is only populated on the
	// instance built for a control-channel connection. It is nil on an instance loaded from the database.
	// Read it from GetConnected rather than from whatever instance is to hand.
	VersionInfo       *versions.VersionInfo
	routerLinks       RouterLinks
	Cost              uint16
	NoTraversal       bool
	Disabled          bool
	Capabilities      *capabilities.RouterCapabilityMask
	Interfaces        []*Interface
	CtrlChanListeners map[string][]string
	Configs           []string
}

func (entity *Router) GetLinks() []*Link {
	return entity.routerLinks.GetLinks()
}

func (entity *Router) toBoltEntityForUpdate(tx *bbolt.Tx, env Env, checker boltz.FieldChecker) (*db.Router, error) {
	if err := validateRouterConfigs(tx, env, entity.Configs, checker); err != nil {
		return nil, err
	}
	return entity.toBoltEntity(), nil
}

func (entity *Router) toBoltEntityForCreate(tx *bbolt.Tx, env Env) (*db.Router, error) {
	if err := validateRouterConfigs(tx, env, entity.Configs, nil); err != nil {
		return nil, err
	}
	return entity.toBoltEntity(), nil
}

func (entity *Router) toBoltEntity() *db.Router {
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
	}
}

// validateRouterConfigs validates that all configs target routers and have unique config types.
// If checker is non-nil and the configs field is not being updated, validation is skipped.
func validateRouterConfigs(tx *bbolt.Tx, env Env, configs []string, checker boltz.FieldChecker) error {
	if checker != nil && !checker.IsUpdated(db.EntityTypeConfigs) {
		return nil
	}
	if len(configs) == 0 {
		return nil
	}

	typeMap := map[string]*db.Config{}
	configStore := env.GetStores().Config
	configTypeStore := env.GetStores().ConfigType
	for _, id := range configs {
		config, err := configStore.LoadById(tx, id)
		if err != nil {
			return err
		}

		configType, err := configTypeStore.LoadById(tx, config.TypeId)
		if err != nil {
			if boltz.IsErrNotFoundErr(err) {
				msg := fmt.Sprintf("config %v references config type %v which does not exist",
					config.Name, config.TypeId)
				return errorz.NewFieldError(msg, "configs", configs)
			}
			return err
		}
		if configType.Target != db.ConfigTypeTargetRouter {
			msg := fmt.Sprintf("config %v has config type %v which does not target routers",
				config.Name, configType.Name)
			return errorz.NewFieldError(msg, "configs", configs)
		}

		if conflictConfig, found := typeMap[config.TypeId]; found {
			msg := fmt.Sprintf("duplicate configs named %v and %v found for config type %v. Only one config of a given type is allowed per router",
				conflictConfig.Name, config.Name, configType.Name)
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

func (entity *Router) addLinkListener(addr, linkProtocol string, groups []string) {
	entity.mu.Lock()
	defer entity.mu.Unlock()
	entity.listeners = append(entity.listeners, &ctrl_pb.Listener{
		Address:  addr,
		Protocol: linkProtocol,
		Groups:   groups,
	})
}

// SetLinkListeners atomically replaces the router's link listener slice.
// Callers do not mutate the previous slice — readers may still hold and
// iterate it safely after a Set.
func (entity *Router) SetLinkListeners(listeners []*ctrl_pb.Listener) {
	entity.mu.Lock()
	defer entity.mu.Unlock()
	entity.listeners = listeners
}

// GetLinkListeners returns the current link listener slice under a read
// lock. The returned slice is the live header — safe to iterate because
// SetLinkListeners always replaces the whole slice rather than mutating
// it in place — but callers should not modify it.
func (entity *Router) GetLinkListeners() []*ctrl_pb.Listener {
	entity.mu.RLock()
	defer entity.mu.RUnlock()
	return entity.listeners
}

func (entity *Router) HasCapability(capability capabilities.RouterCapability) bool {
	return entity.Capabilities != nil && entity.Capabilities.IsSet(capability)
}
