package model

import (
	"errors"
	"testing"

	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/db"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/stretchr/testify/require"
)

func Test_DefaultAdmin_RemainsAdmin(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	req := require.New(t)

	req.NoError(ctx.managers.Identity.InitializeDefaultAdmin("defaultadmin", "defaultadminpassword", "Default Admin"))
	defaultAdmin, err := ctx.managers.Identity.ReadDefaultAdmin()
	req.NoError(err)
	req.True(defaultAdmin.IsAdmin)
	req.True(defaultAdmin.IsDefaultAdmin)

	t.Run("clearing isAdmin on the default admin is rejected", func(t *testing.T) {
		defaultAdmin.IsAdmin = false
		err := ctx.managers.Identity.Update(defaultAdmin, nil, change.New())
		require.Error(t, err)
		var fe *errorz.FieldError
		require.True(t, errors.As(err, &fe), "expected a field error, got %T: %v", err, err)
		require.Equal(t, db.FieldIdentityIsAdmin, fe.FieldName)

		stored, err := ctx.managers.Identity.Read(defaultAdmin.Id)
		require.NoError(t, err)
		require.True(t, stored.IsAdmin, "the rejected update changed nothing")
	})

	t.Run("a default admin cannot be created without admin rights", func(t *testing.T) {
		err := ctx.managers.Identity.Create(&Identity{Name: "second-default", IdentityTypeId: db.DefaultIdentityType, IsDefaultAdmin: true}, change.New())
		require.Error(t, err)
		var fe *errorz.FieldError
		require.True(t, errors.As(err, &fe), "expected a field error, got %T: %v", err, err)
		require.Equal(t, db.FieldIdentityIsAdmin, fe.FieldName)
	})

	t.Run("a default admin whose flag was cleared earlier still counts as an admin", func(t *testing.T) {
		// data written before the guard existed: set the flag directly, bypassing the store
		req.NoError(ctx.GetDb().Update(nil, func(mctx boltz.MutateContext) error {
			bucket := ctx.GetStores().Identity.GetEntityBucket(mctx.Tx(), []byte(defaultAdmin.Id))
			bucket.SetBool(db.FieldIdentityIsAdmin, false, nil)
			return bucket.GetError()
		}))
		stored, err := ctx.managers.Identity.Read(defaultAdmin.Id)
		req.NoError(err)
		req.False(stored.IsAdmin)
		req.True(stored.IsDefaultAdmin)

		target := ctx.requireNewIdentity(true)
		target.RoleAttributes = []string{"patched"}
		req.NoError(ctx.managers.Identity.Update(target, nil, asIdentity(defaultAdmin.Id)), "the default admin may still change admin targets")

		nonAdmin := ctx.requireNewIdentity(false)
		requireUnauthorized(t, ctx.managers.Identity.Delete(defaultAdmin.Id, asIdentity(nonAdmin.Id)))
	})
}
