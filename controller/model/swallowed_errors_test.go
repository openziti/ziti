package model

import (
	"errors"
	"testing"

	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/db"
	"github.com/openziti/ziti/v2/controller/idgen"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/stretchr/testify/require"
)

func Test_CreateWithAuthenticators_ReturnsDispatchError(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	req := require.New(t)

	existing := ctx.requireNewIdentity(false)

	duplicate := &Identity{Name: existing.Name, IdentityTypeId: db.DefaultIdentityType}
	authenticator := &Authenticator{Method: db.MethodAuthenticatorCert}
	authenticator.SubType = &AuthenticatorCert{Authenticator: authenticator, Fingerprint: idgen.MustNewUUIDString()}

	id, authenticatorIds, err := ctx.managers.Identity.CreateWithAuthenticators(duplicate, []*Authenticator{authenticator}, change.New())
	req.Error(err, "a create that fails in Apply reports the failure")
	req.Empty(id)
	req.Nil(authenticatorIds)

	_, err = ctx.managers.Identity.Read(duplicate.Id)
	req.True(boltz.IsErrNotFoundErr(err), "the identity was not created")
}

func Test_Update_RejectsDuplicateAndEmptyName(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	req := require.New(t)

	first := ctx.requireNewService()
	second := ctx.requireNewService()
	originalName := second.Name

	second.Name = first.Name
	err := ctx.managers.EdgeService.Update(second, nil, change.New())
	req.Error(err, "renaming to a name already in use is an error")
	var fe *errorz.FieldError
	req.True(errors.As(err, &fe))
	req.Equal("name", fe.FieldName)

	second.Name = ""
	err = ctx.managers.EdgeService.Update(second, nil, change.New())
	req.Error(err, "renaming to an empty name is an error")

	stored, err := ctx.managers.EdgeService.Read(second.Id)
	req.NoError(err)
	req.Equal(originalName, stored.Name, "a rejected rename leaves the stored name unchanged")
}
