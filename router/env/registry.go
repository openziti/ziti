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

package env

import (
	"fmt"
	"sync"

	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/ziti/router/xgress_router"
)

type Registry struct {
	factories   concurrenz.CopyOnWriteMap[string, xgress_router.Factory]
	factoryF    map[string]func(RouterEnv) xgress_router.Factory
	initialized bool
	lock        sync.Mutex
}

func NewRegistry() *Registry {
	return &Registry{
		factoryF: make(map[string]func(RouterEnv) xgress_router.Factory),
	}
}

func (registry *Registry) Replace(name string, factory xgress_router.Factory) {
	registry.factories.Put(name, factory)
}

func (registry *Registry) Register(name string, factory xgress_router.Factory) {
	err := registry.RegisterF(name, func(routerEnv RouterEnv) xgress_router.Factory {
		return factory
	})
	if err != nil {
		panic(err)
	}
}

func (registry *Registry) RegisterF(name string, f func(routerEnv RouterEnv) xgress_router.Factory) error {
	registry.lock.Lock()
	defer registry.lock.Unlock()
	if registry.initialized {
		return fmt.Errorf("cannot add xgress factory '%s' after Registry has already been initialized", name)
	}
	if _, ok := registry.factoryF[name]; ok {
		return fmt.Errorf("xgress factory with name '%s' already registered", name)
	}
	registry.factoryF[name] = f
	return nil
}

func (registry *Registry) Initialize(env RouterEnv) {
	registry.lock.Lock()
	defer registry.lock.Unlock()
	if !registry.initialized {
		for k, f := range registry.factoryF {
			registry.factories.Put(k, f(env))
		}
		registry.factoryF = nil
		registry.initialized = true
	}
}

func (registry *Registry) Factory(name string) (xgress_router.Factory, error) {
	if factory := registry.factories.Get(name); factory != nil {
		return factory, nil
	} else {
		return nil, fmt.Errorf("xgress factory [%s] not found", name)
	}
}

func (registry *Registry) List() []string {
	names := make([]string, 0)
	for k := range registry.factories.AsMap() {
		names = append(names, k)
	}
	return names
}

func (registry *Registry) Debug() string {
	out := "{"
	for _, name := range registry.List() {
		out += " " + name
	}
	out += " }"
	return out
}
