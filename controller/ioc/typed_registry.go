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
	"fmt"
	"reflect"

	"github.com/openziti/foundation/v2/concurrenz"
)

// TypedProvider instances return instances of a given type. They may return singletons or
// a new instance each time
type TypedProvider[T any] interface {
	Get() (T, error)
}

type typedSingletonProvider[T any] struct {
	value T
}

func (s typedSingletonProvider[T]) Get() (T, error) {
	return s.value, nil
}

type providerFunc[T any] struct {
	providerF func() (T, error)
}

func (p providerFunc[T]) Get() (T, error) {
	return p.providerF()
}

func TypedProviderF[T any](providerF func() (T, error)) TypedProvider[T] {
	return providerFunc[T]{
		providerF: providerF,
	}
}

func CachingProvider[T any](p TypedProvider[T]) TypedProvider[T] {
	var cached *T
	return TypedProviderF(func() (T, error) {
		if cached == nil {
			v, err := p.Get()
			if err != nil {
				return v, err
			}
			cached = &v
		}
		return *cached, nil
	})
}

// NewTypedRegistry returns a new TypedRegistry instance
func NewTypedRegistry() *TypedRegistry {
	return &TypedRegistry{}
}

type TypedRegistry struct {
	m concurrenz.CopyOnWriteMap[reflect.Type, any]
}

func GetTypedProvider[T any](r *TypedRegistry) TypedProvider[T] {
	p := r.m.Get(reflect.TypeFor[T]())
	if tp, ok := p.(TypedProvider[T]); ok {
		return tp
	}
	return nil
}

func RegisterTyped[T any](r *TypedRegistry, provider TypedProvider[T]) {
	r.m.Put(reflect.TypeFor[T](), provider)
}

func RegisterTypedSingleton[T any](r *TypedRegistry, val T) {
	RegisterTyped[T](r, typedSingletonProvider[T]{val})
}

func GetTyped[T any](r *TypedRegistry) (T, error) {
	provider := GetTypedProvider[T](r)
	if provider != nil {
		return provider.Get()
	}

	var result T
	return result, fmt.Errorf("no provider for type '%v'", reflect.TypeFor[T]())
}

func InjectableProvider[T any](r *TypedRegistry, provider any) TypedProvider[T] {
	providerType := reflect.TypeOf(provider)
	funcValue := reflect.ValueOf(provider)
	if providerType.Kind() != reflect.Func {
		return TypedProviderF(func() (T, error) {
			var result T
			return result, fmt.Errorf("provider for '%v' is not a function", reflect.TypeOf(provider))
		})
	}

	injectorF := func() (T, error) {
		var result T
		var params []reflect.Value
		for i := 0; i < providerType.NumIn(); i++ {
			paramType := providerType.In(i)
			paramProvider := r.m.Get(paramType)

			if paramProvider == nil {
				return result, fmt.Errorf("no provider for type '%v' when building '%v'", paramType, providerType)
			}

			getMethod := reflect.ValueOf(paramProvider).MethodByName("Get")
			if getMethod.IsZero() {
				return result, fmt.Errorf("provider for type '%v' has no Get method when building '%v'", paramType, providerType)
			}

			results := getMethod.Call(nil)
			if len(results) != 2 {
				return result, fmt.Errorf("provider get method for type '%v' returned wrong number of results when building '%v'", paramType, providerType)
			}

			if !results[1].IsNil() {
				if err, ok := results[1].Interface().(error); ok {
					return result, fmt.Errorf("provider get method for type '%v' returned error when building '%v' (%w)", paramType, providerType, err)
				} else {
					return result, fmt.Errorf("provider get method for type '%v' returned non-error error when building '%v' (%T)", paramType, providerType, results[1].Interface())
				}
			}

			if !results[0].CanConvert(paramType) {
				return result, fmt.Errorf("provider get method for type '%v' returned wrong type for argument %d when building '%v'", paramType, i, providerType)
			}

			params = append(params, results[0].Convert(paramType))
		}

		results := funcValue.Call(params)
		if len(results) != 2 {
			return result, fmt.Errorf("provider get method for type '%v' returned wrong number of results", providerType)
		}

		if !results[1].IsNil() {
			if err, ok := results[1].Interface().(error); ok {
				return result, fmt.Errorf("provider get method for type '%v' returned error (%w)", providerType, err)
			} else {
				return result, fmt.Errorf("provider get method for type '%v' returned non-error error (%T)", providerType, results[1].Interface())
			}
		}

		if !results[0].CanConvert(reflect.TypeFor[T]()) {
			return result, fmt.Errorf("provider get method for type '%v' returned wrong type '%T", providerType, results[0].Type())
		}

		return results[0].Interface().(T), nil
	}

	return TypedProviderF(injectorF)
}
