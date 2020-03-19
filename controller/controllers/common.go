package controllers

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"go.etcd.io/bbolt"
)

func DeleteEntityById(store boltz.CrudStore, db boltz.Db, id string) error {
	return db.Update(func(tx *bbolt.Tx) error {
		ctx := boltz.NewMutateContext(tx)
		if !store.IsEntityPresent(tx, id) {
			return boltz.NewNotFoundError(store.GetSingularEntityType(), "id", id)
		}

		if err := store.DeleteById(ctx, id); err != nil {
			pfxlog.Logger().
				WithField("id", id).
				WithField("type", store.GetSingularEntityType()).
				WithError(err).Error("could not delete by id")
			return err
		}
		return nil
	})
}
