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

// Package ioc provides a simple generics based registry which allows registering
// instance providers for a name and then instantiating instances, returning
// instances of the requested type.
package ioc

import (
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/pkg/errors"
)

// Provider instances return instances of a given type. They may return singletons or
// a new instance each time
type Provider interface {
	Get() any
}

// ProviderF is a function implemenation of the Provider interface
type ProviderF func() any

func (f ProviderF) Get() any {
	return f()
}

// Registry instances store Providers keyed by name
type Registry interface {
	// Register adds a Provider for the given name
	Register(name string, provider Provider)

	// RegisterSingleton adds a Provider which always returns the given instance
	RegisterSingleton(name string, val interface{})

	// GetProvider returns the Provider for the given name, or nil if no Provider has been
	// registered for the given name
	GetProvider(name string) Provider
}

// NewRegistry returns a new Registry instance
func NewRegistry() Registry {
	return &registry{}
}

type registry struct {
	m concurrenz.CopyOnWriteMap[string, Provider]
}

func (self *registry) GetProvider(name string) Provider {
	return self.m.Get(name)
}

func (self *registry) Register(name string, provider Provider) {
	self.m.Put(name, provider)
}

func (self *registry) RegisterSingleton(name string, val any) {
	self.Register(name, ProviderF(func() any {
		return val
	}))
}

// Get will return an instance provided by the Provider mapped to the given name
// If no provider for the given name exists, or if the instance is not of the type
// requested an error will be returned
func Get[T any](r Registry, name string) (T, error) {
	provider := r.GetProvider(name)
	if provider == nil {
		var result T
		return result, errors.Errorf("no provider for name '%v'", name)
	}

	val := provider.Get()
	if result, ok := val.(T); ok {
		return result, nil
	}

	var defaultV T
	return defaultV, errors.Errorf("provider '%v' returned val of type %T instead of %T", name, val, defaultV)
}
