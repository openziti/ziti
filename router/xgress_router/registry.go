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

package xgress_router

import (
	"errors"
	"fmt"
	"sync"

	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/ziti/router/env"
)

type registry struct {
	factories   concurrenz.CopyOnWriteMap[string, Factory]
	factoryF    map[string]func(env.RouterEnv) Factory
	initialized bool
	lock        sync.Mutex
}

func NewRegistry() *registry {
	return &registry{
		factoryF: make(map[string]func(env.RouterEnv) Factory),
	}
}

func (registry *registry) Register(name string, factory Factory) {
	registry.factories.Put(name, factory)
}

func (registry *registry) RegisterF(name string, f func(routerEnv env.RouterEnv) Factory) error {
	registry.lock.Lock()
	defer registry.lock.Unlock()
	if registry.initialized {
		return errors.New("cannot add factory function after registry has already been initialized")
	}
	registry.factoryF[name] = f
	return nil
}

func (registry *registry) Initialize(env env.RouterEnv) {
	registry.lock.Lock()
	defer registry.lock.Unlock()
	for k, f := range registry.factoryF {
		registry.Register(k, f(env))
	}
	registry.factoryF = nil
	registry.initialized = true
}

func (registry *registry) Factory(name string) (Factory, error) {
	if factory := registry.factories.Get(name); factory != nil {
		return factory, nil
	} else {
		return nil, fmt.Errorf("xgress factory [%s] not found", name)
	}
}

func (registry *registry) List() []string {
	names := make([]string, 0)
	for k := range registry.factories.AsMap() {
		names = append(names, k)
	}
	return names
}

func (registry *registry) Debug() string {
	out := "{"
	for _, name := range registry.List() {
		out += " " + name
	}
	out += " }"
	return out
}

var globalRegistry *registry

func GlobalRegistry() *registry {
	if globalRegistry == nil {
		globalRegistry = NewRegistry()
	}
	return globalRegistry
}
