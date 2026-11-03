package model

import (
	"errors"
	"testing"

	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/db"
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
}
