package env

import (
	"testing"

	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/permissions"
	"github.com/stretchr/testify/require"
)

func newResolvedSecurityCtx(identity *model.Identity, mfaPending bool) *SecurityCtx {
	ctx := &SecurityCtx{
		resolvedPermissions: map[string]struct{}{},
		resolvedApiSession:  &model.ApiSession{},
		resolvedIdentity:    identity,
	}
	if mfaPending {
		ctx.resolvedMfaAuthQueries = []*rest_model.AuthQueryDetail{{}}
	}
	ctx.resolvePermissions()
	return ctx
}

func TestSecurityCtx_ResolvePermissions(t *testing.T) {
	identity := &model.Identity{IsAdmin: true, Permissions: []string{"identity.delete"}}

	t.Run("a fully authenticated session holds everything it was granted", func(t *testing.T) {
		ctx := newResolvedSecurityCtx(identity, false)
		require.Contains(t, ctx.resolvedPermissions, permissions.AuthenticatedPermission)
		require.Contains(t, ctx.resolvedPermissions, permissions.AdminPermission)
		require.Contains(t, ctx.resolvedPermissions, "identity.delete")
		require.NotContains(t, ctx.resolvedPermissions, permissions.PartiallyAuthenticatePermission)
	})

	t.Run("a session with MFA pending holds only the partial authentication permission", func(t *testing.T) {
		ctx := newResolvedSecurityCtx(identity, true)
		require.Contains(t, ctx.resolvedPermissions, permissions.PartiallyAuthenticatePermission)
		require.NotContains(t, ctx.resolvedPermissions, permissions.AuthenticatedPermission)
		require.NotContains(t, ctx.resolvedPermissions, permissions.AdminPermission)
		require.NotContains(t, ctx.resolvedPermissions, "identity.delete", "granular permissions must wait for MFA")
	})
}
