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
	"fmt"
	"github.com/openziti/foundation/v2/concurrenz"
)

type registry struct {
	factories concurrenz.CopyOnWriteMap[string, Factory]
}

func NewRegistry() *registry {
	return &registry{}
}

func (registry *registry) Register(name string, factory Factory) {
	registry.factories.Put(name, factory)
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
