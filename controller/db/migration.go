package db

import (
	"github.com/openziti/storage/boltz"
)

func (stores *stores) migrateTerminatorIdentityFields(step *boltz.MigrationStep) {
	terminatorIds, _, err := stores.terminator.QueryIds(step.Ctx.Tx(), "true")
	step.SetError(err)

	fieldIdentity := "identity"
	fieldIdentitySecret := "identitySecret"

	for _, terminatorId := range terminatorIds {
		bucket := stores.terminator.GetEntityBucket(step.Ctx.Tx(), []byte(terminatorId))

		if instanceId := bucket.GetString(fieldIdentity); instanceId != nil {
			bucket.SetString(FieldTerminatorInstanceId, *instanceId, nil)
		}

		if instanceSecret := bucket.Get([]byte(fieldIdentitySecret)); instanceSecret != nil {
			bucket.PutValue([]byte(FieldTerminatorInstanceSecret), instanceSecret)
		}

		step.SetError(bucket.Delete([]byte(fieldIdentity)))
		step.SetError(bucket.Delete([]byte(fieldIdentitySecret)))
		step.SetError(bucket.GetError())
	}
}
