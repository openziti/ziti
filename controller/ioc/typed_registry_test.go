package ioc

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

type testGreeter interface {
	SayHello() string
}

type testGreeterF func() string

func (self testGreeterF) SayHello() string {
	return self()
}

type defaultGreeter struct{}

func (d defaultGreeter) SayHello() string {
	return "hello"
}

type overrideGreeter struct{}

func (d overrideGreeter) SayHello() string {
	return "override"
}

func Test_TypedRegistry(t *testing.T) {
	reg := NewTypedRegistry()
	RegisterTypedSingleton[testGreeter](reg, defaultGreeter{})

	r := require.New(t)

	greeter, err := GetTyped[testGreeter](reg)
	r.NoError(err)
	r.Equal("hello", greeter.SayHello())

	RegisterTypedSingleton[testGreeter](reg, overrideGreeter{})
	greeter, err = GetTyped[testGreeter](reg)
	r.NoError(err)
	r.Equal("override", greeter.SayHello())

	RegisterTyped[testGreeter](reg, TypedProviderF(func() (testGreeter, error) {
		return testGreeterF(func() string {
			return "function"
		}), nil
	}))

	greeter, err = GetTyped[testGreeter](reg)
	r.NoError(err)
	r.Equal("function", greeter.SayHello())

	var counter int
	RegisterTyped[testGreeter](reg, TypedProviderF(func() (testGreeter, error) {
		return testGreeterF(func() string {
			counter++
			return fmt.Sprintf("%d", counter)
		}), nil
	}))

	greeter, err = GetTyped[testGreeter](reg)
	r.NoError(err)
	r.Equal("1", greeter.SayHello())

	greeter, err = GetTyped[testGreeter](reg)
	r.NoError(err)
	r.Equal("2", greeter.SayHello())

	counter = 0

	RegisterTyped[testGreeter](reg, CachingProvider(TypedProviderF(func() (testGreeter, error) {
		return testGreeterF(func() string {
			counter++
			return fmt.Sprintf("%d", counter)
		}), nil
	})))

	greeter, err = GetTyped[testGreeter](reg)
	r.NoError(err)
	r.Equal("1", greeter.SayHello())

	greeter, err = GetTyped[testGreeter](reg)
	r.NoError(err)
	r.Equal("2", greeter.SayHello())

	RegisterTypedSingleton[defaultGreeter](reg, defaultGreeter{})
	greeter, err = GetTyped[defaultGreeter](reg)
	r.NoError(err)
	r.Equal("hello", greeter.SayHello())
}

type injectable struct {
	greeter  testGreeter
	override overrideGreeter
}

func newInjectable(greeter testGreeter, override overrideGreeter) (*injectable, error) {
	return &injectable{
		greeter:  greeter,
		override: override,
	}, nil
}

func Test_TypedRegistryInjections(t *testing.T) {
	reg := NewTypedRegistry()
	RegisterTypedSingleton[testGreeter](reg, defaultGreeter{})
	RegisterTypedSingleton[overrideGreeter](reg, overrideGreeter{})
	RegisterTyped[*injectable](reg, InjectableProvider[*injectable](reg, newInjectable))

	r := require.New(t)

	greeter, err := GetTyped[*injectable](reg)
	r.NoError(err)
	r.Equal("hello", greeter.greeter.SayHello())
	r.Equal("override", greeter.override.SayHello())
}
