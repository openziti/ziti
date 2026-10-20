package model

import (
	"errors"
	"testing"

	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/stretchr/testify/require"
)

func asIdentity(id string) *change.Context {
	return change.New().SetChangeAuthorType(change.AuthorTypeIdentity).SetChangeAuthorId(id)
}

func requireUnauthorized(t *testing.T, err error) {
	require.Error(t, err)
	var apiErr *errorz.ApiError
	require.True(t, errors.As(err, &apiErr), "expected an API error, got %T: %v", err, err)
	require.Equal(t, errorz.UnauthorizedStatus, apiErr.Status)
}

func Test_IdentityAdminProtection_DecidedInApply(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	req := require.New(t)

	admin := ctx.requireNewIdentity(true)
	nonAdmin := ctx.requireNewIdentity(false)

	t.Run("a non-admin author may not update an admin target", func(t *testing.T) {
		target := ctx.requireNewIdentity(true)
		target.RoleAttributes = []string{"patched"}
		requireUnauthorized(t, ctx.managers.Identity.Update(target, nil, asIdentity(nonAdmin.Id)))

		stored, err := ctx.managers.Identity.Read(target.Id)
		req.NoError(err)
		req.Empty(stored.RoleAttributes, "the refused update changed nothing")
	})

	t.Run("a non-admin author may not delete an admin target", func(t *testing.T) {
		target := ctx.requireNewIdentity(true)
		requireUnauthorized(t, ctx.managers.Identity.Delete(target.Id, asIdentity(nonAdmin.Id)))

		_, err := ctx.managers.Identity.Read(target.Id)
		req.NoError(err, "the refused delete left the target in place")
	})

	t.Run("a non-admin author may update and delete a non-admin target", func(t *testing.T) {
		target := ctx.requireNewIdentity(false)
		target.RoleAttributes = []string{"patched"}
		req.NoError(ctx.managers.Identity.Update(target, nil, asIdentity(nonAdmin.Id)))
		req.NoError(ctx.managers.Identity.Delete(target.Id, asIdentity(nonAdmin.Id)))
	})

	t.Run("an admin author may update and delete an admin target", func(t *testing.T) {
		target := ctx.requireNewIdentity(true)
		target.RoleAttributes = []string{"patched"}
		req.NoError(ctx.managers.Identity.Update(target, nil, asIdentity(admin.Id)))
		req.NoError(ctx.managers.Identity.Delete(target.Id, asIdentity(admin.Id)))
	})

	t.Run("a controller-originated change is exempt", func(t *testing.T) {
		target := ctx.requireNewIdentity(true)
		target.RoleAttributes = []string{"patched"}
		req.NoError(ctx.managers.Identity.Update(target, nil, change.New().SetChangeAuthorType(change.AuthorTypeController)))
	})

	t.Run("a deleted author is not an admin", func(t *testing.T) {
		formerAdmin := ctx.requireNewIdentity(true)
		req.NoError(ctx.managers.Identity.Delete(formerAdmin.Id, change.New()))
		_, err := ctx.managers.Identity.Read(formerAdmin.Id)
		req.True(boltz.IsErrNotFoundErr(err))

		target := ctx.requireNewIdentity(true)
		requireUnauthorized(t, ctx.managers.Identity.Delete(target.Id, asIdentity(formerAdmin.Id)))
	})
}
