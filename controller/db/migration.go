package db

import (
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/pkg/errors"
	"time"
)

const CurrentDbVersion = 1

func (stores *stores) migrate(step *boltz.MigrationStep) int {
	if step.CurrentVersion == 0 {
		stores.migrateToV1(step)
		return 1
	}
	if step.CurrentVersion == 1 {
		return 1
	}
	step.SetError(errors.Errorf("Unsupported fabric datastore version: %v", step.CurrentVersion))
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
