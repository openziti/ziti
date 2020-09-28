package xtv

import (
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

type Terminator interface {
	GetId() string
	GetServiceId() string
	GetRouterId() string
	GetBinding() string
	GetAddress() string
	GetIdentity() string
	GetIdentitySecret() []byte
}

type Validator interface {
	Validate(tx *bbolt.Tx, terminator Terminator, create bool) error
}

type Registry interface {
	RegisterBinding(binding, validatorId string)
	RegisterValidator(id string, validator Validator)
	GetValidatorForBinding(binding string) Validator
}

func RegisterValidator(id string, validator Validator) {
	globalRegistry.RegisterValidator(id, validator)
}

func Validate(tx *bbolt.Tx, terminator Terminator, create bool) error {
	validator := globalRegistry.GetValidatorForBinding(terminator.GetBinding())
	return validator.Validate(tx, terminator, create)
}

func InitializeMappings() error {
	globalRegistry.mappings.lock.Lock()
	defer globalRegistry.mappings.lock.Unlock()
	mappings := globalRegistry.mappings.value.Load().(map[string]string)
	for binding, validatorId := range mappings {
		validator := globalRegistry.validators.get(validatorId)
		if validator == nil {
			return errors.Errorf("no validator %v found for terminator binding %v", validatorId, binding)
		}
		globalRegistry.bindings.put(binding, validator)
	}
	return nil
}
