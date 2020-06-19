package db

import (
	"github.com/openziti/fabric/controller/xt_smartrouting"
	"github.com/openziti/foundation/storage/ast"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/pkg/errors"
	"strings"
	"time"
)

const CurrentDbVersion = 3

func (stores *stores) migrate(step *boltz.MigrationStep) int {
	if step.CurrentVersion > CurrentDbVersion {
		step.SetError(errors.Errorf("unsupported fabric datastore version: %v", step.CurrentVersion))
		return 0
	}

	if step.CurrentVersion < 1 {
		stores.migrateToV1(step)
	}

	if step.CurrentVersion < 2 {
		stores.extractTerminators(step)
	}

	if step.CurrentVersion < 3 {
		stores.setNames(step, stores.service)
		stores.setNames(step, stores.router)
	}

	if step.CurrentVersion <= CurrentDbVersion {
		return CurrentDbVersion
	}

	step.SetError(errors.Errorf("unsupported fabric datastore version: %v", step.CurrentVersion))
	return 0
}

func (stores *stores) migrateToV1(step *boltz.MigrationStep) {
	now := time.Now()
	stores.initCreatedAtUpdatedAt(step, now, stores.service)
	stores.initCreatedAtUpdatedAt(step, now, stores.router)
}

func (stores *stores) initCreatedAtUpdatedAt(step *boltz.MigrationStep, now time.Time, store boltz.CrudStore) {
	ids, _, err := store.QueryIds(step.Ctx.Tx(), "true")
	step.SetError(err)
	for _, id := range ids {
		entityBucket := store.GetEntityBucket(step.Ctx.Tx(), []byte(id))
		if entityBucket == nil {
			step.SetError(errors.Errorf("could not get entity bucket for %v with id %v", store.GetSingularEntityType(), id))
			return
		}
		entityBucket.SetTime(boltz.FieldCreatedAt, now, nil)
		entityBucket.SetTime(boltz.FieldUpdatedAt, now, nil)
		if step.SetError(entityBucket.GetError()) {
			return
		}
	}
}

func (stores *stores) setNames(step *boltz.MigrationStep, store boltz.CrudStore) {
	ids, _, err := store.QueryIds(step.Ctx.Tx(), "true")
	step.SetError(err)
	for _, id := range ids {
		entityBucket := store.GetEntityBucket(step.Ctx.Tx(), []byte(id))
		if entityBucket == nil {
			step.SetError(errors.Errorf("could not get entity bucket for %v with id %v", store.GetSingularEntityType(), id))
			return
		}
		if name := entityBucket.GetString(FieldName); name == nil || len(*name) == 0 {
			entityBucket.SetString(FieldName, string(id), nil)
			step.SetError(entityBucket.GetError())
		}
	}
}

const (
	FieldServiceEgress   = "egress"
	FieldServiceBinding  = "binding"
	FieldServiceEndpoint = "endpoint"
)

func (stores *stores) extractTerminators(step *boltz.MigrationStep) {
	serviceIds, _, err := stores.service.QueryIds(step.Ctx.Tx(), "true")
	step.SetError(err)

	symbolEgress := stores.service.AddSymbol(FieldServiceEgress, ast.NodeTypeString)
	symbolBinding := stores.service.AddSymbol(FieldServiceBinding, ast.NodeTypeString)
	symbolEndpoint := stores.service.AddSymbol(FieldServiceEndpoint, ast.NodeTypeString)

	for _, serviceId := range serviceIds {
		service, err := stores.service.LoadOneById(step.Ctx.Tx(), serviceId)
		if step.SetError(err) {
			return
		}

		if service.TerminatorStrategy == "" {
			service.TerminatorStrategy = xt_smartrouting.Name
			if step.SetError(stores.service.Update(step.Ctx, service, nil)) {
				return
			}
		}

		hasTerminators := stores.service.GetRelatedEntitiesCursor(step.Ctx.Tx(), serviceId, EntityTypeTerminators, true).IsValid()
		if hasTerminators {
			continue
		}
		routerId := boltz.FieldToString(symbolEgress.Eval(step.Ctx.Tx(), []byte(serviceId)))
		binding := boltz.FieldToString(symbolBinding.Eval(step.Ctx.Tx(), []byte(serviceId)))
		address := boltz.FieldToString(symbolEndpoint.Eval(step.Ctx.Tx(), []byte(serviceId)))

		if routerId == nil || *routerId == "" || !stores.router.IsEntityPresent(step.Ctx.Tx(), *routerId) {
			continue
		}

		if address == nil || *address == "" {
			continue
		}

		if binding == nil || *binding == "" {
			if strings.HasPrefix(*address, "udp:") {
				val := "udp"
				binding = &val
			} else {
				val := "transport"
				binding = &val
			}
		}

		terminator := &Terminator{
			Service: serviceId,
			Router:  *routerId,
			Binding: *binding,
			Address: *address,
		}

		step.SetError(stores.terminator.Create(step.Ctx, terminator))
	}
}
