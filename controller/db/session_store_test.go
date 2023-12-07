/*
	Copyright NetFoundry Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package db

import (
	"fmt"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/storage/boltztest"
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/controller/change"
	"go.etcd.io/bbolt"
	"testing"
	"time"
)

const apiSessionsSessionsIdxPath = "/" + RootBucket + "/" + boltz.IndexesBucket + "/" + EntityTypeApiSessions + "/" + EntityTypeSessions

func Test_SessionStore(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	t.Run("test create invalid sessions", ctx.testCreateInvalidSessions)
	t.Run("test update invalid sessions", ctx.testUpdateInvalidSessions)
	t.Run("test create sessions", ctx.testCreateSessions)
	t.Run("test load/query sessions", ctx.testLoadQuerySessions)
	t.Run("test update sessions", ctx.testUpdateSessions)
	t.Run("test delete sessions", ctx.testDeleteSessions)
}

func (ctx *TestContext) testCreateInvalidSessions(_ *testing.T) {
	defer ctx.CleanupAll()

	identity := ctx.RequireNewIdentity("test-user", false)
	apiSession := NewApiSession(identity.Id)
	boltztest.RequireCreate(ctx, apiSession)

	service := ctx.RequireNewService("test-service")

	session := NewSession("", service.Id)
	err := boltztest.Create(ctx, session)
	ctx.EqualError(err, "fk constraint on sessions.apiSession does not allow null or empty values")

	session.ApiSessionId = "invalid-id"
	err = boltztest.Create(ctx, session)
	ctx.EqualError(err, fmt.Sprintf("apiSession with id %v not found", session.ApiSessionId))

	session.ApiSessionId = apiSession.Id
	session.ServiceId = ""
	err = boltztest.Create(ctx, session)
	ctx.EqualError(err, "fk constraint on sessions.service does not allow null or empty values")

	session.ServiceId = "invalid-id"
	err = boltztest.Create(ctx, session)
	ctx.EqualError(err, fmt.Sprintf("service with id %v not found", session.ServiceId))

	session.ServiceId = service.Id
	err = boltztest.Create(ctx, session)
	ctx.NoError(err)
	err = boltztest.Create(ctx, session)
	ctx.EqualError(err, fmt.Sprintf("an entity of type session already exists with id %v", session.Id))
}

func (ctx *TestContext) testUpdateInvalidSessions(_ *testing.T) {
	defer ctx.CleanupAll()

	identity := ctx.RequireNewIdentity("test-user", false)
	apiSession := NewApiSession(identity.Id)
	boltztest.RequireCreate(ctx, apiSession)

	service := ctx.RequireNewService("test-service")

	session := NewSession(apiSession.Id, service.Id)
	boltztest.RequireCreate(ctx, session)

	token := session.Token

	session.ApiSessionId = "invalid-api-session-id"
	session.ServiceId = "invalid-service-id"
	session.Token = "different token"
	session.IdentityId = "different id"
	session.Type = PolicyTypeBindName
	boltztest.RequireUpdate(ctx, session)

	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		loaded, err := ctx.stores.Session.LoadOneById(tx, session.Id)
		ctx.NoError(err)
		ctx.NotNil(loaded)
		ctx.Equal(apiSession.Id, loaded.ApiSessionId)
		ctx.Equal(service.Id, loaded.ServiceId)
		ctx.Equal(token, loaded.Token)
		ctx.Equal(apiSession.IdentityId, loaded.IdentityId)
		ctx.Equal(PolicyTypeDialName, loaded.Type)
		return nil
	})
	ctx.NoError(err)

	boltztest.RequireDelete(ctx, session, apiSessionsSessionsIdxPath)

	err = boltztest.Update(ctx, session)
	ctx.EqualError(err, fmt.Sprintf("session with id %v not found", session.Id))
}

func (ctx *TestContext) testCreateSessions(_ *testing.T) {
	ctx.CleanupAll()

	compareOpts := cmpopts.IgnoreFields(Session{}, "ApiSession")

	identity := ctx.RequireNewIdentity("Jojo", false)
	apiSession := NewApiSession(identity.Id)
	boltztest.RequireCreate(ctx, apiSession)
	service := ctx.RequireNewService("test-service")
	session := NewSession(apiSession.Id, service.Id)
	boltztest.RequireCreate(ctx, session)
	boltztest.ValidateBaseline(ctx, session, compareOpts)

	service2 := ctx.RequireNewService("test-service-2")
	session3 := NewSession(apiSession.Id, service2.Id)
	session3.Tags = ctx.CreateTags()
	boltztest.RequireCreate(ctx, session3)
	boltztest.ValidateBaseline(ctx, session3, compareOpts)

	boltztest.RequireDelete(ctx, service2, apiSessionsSessionsIdxPath)
	boltztest.ValidateDeleted(ctx, session3.Id, apiSessionsSessionsIdxPath)
	boltztest.RequireReload(ctx, session)

	err := boltztest.Delete(ctx, apiSession)
	ctx.NoError(err)

	done, err := ctx.GetStores().EventualEventer.Trigger()
	ctx.NoError(err)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		ctx.Fail("did not receive done notification from eventual eventer")

	}

	boltztest.ValidateDeleted(ctx, session.Id)
	boltztest.ValidateDeleted(ctx, apiSession.GetId())
}

type sessionTestEntities struct {
	identity1   *Identity
	apiSession1 *ApiSession
	apiSession2 *ApiSession
	service1    *EdgeService
	service2    *EdgeService
	session1    *Session
	session2    *Session
	session3    *Session
}

func (ctx *TestContext) createSessionTestEntities() *sessionTestEntities {
	identity1 := ctx.RequireNewIdentity("admin1", true)

	apiSession1 := NewApiSession(identity1.Id)
	boltztest.RequireCreate(ctx, apiSession1)

	apiSession2 := NewApiSession(identity1.Id)
	boltztest.RequireCreate(ctx, apiSession2)

	service1 := ctx.RequireNewService(eid.New())
	service2 := ctx.RequireNewService(eid.New())

	session1 := NewSession(apiSession1.Id, service1.Id)
	boltztest.RequireCreate(ctx, session1)

	session2 := NewSession(apiSession2.Id, service2.Id)
	boltztest.RequireCreate(ctx, session2)

	session3 := NewSession(apiSession2.Id, service2.Id)
	session3.Type = PolicyTypeBindName

	boltztest.RequireCreate(ctx, session3)

	return &sessionTestEntities{
		identity1:   identity1,
		apiSession1: apiSession1,
		apiSession2: apiSession2,
		service1:    service1,
		service2:    service2,
		session1:    session1,
		session2:    session2,
		session3:    session3,
	}
}

func (ctx *TestContext) testLoadQuerySessions(_ *testing.T) {
	ctx.CleanupAll()

	entities := ctx.createSessionTestEntities()

	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		query := fmt.Sprintf(`apiSession = "%v"`, entities.apiSession1.Id)
		ids, _, err := ctx.stores.Session.QueryIds(tx, query)
		ctx.NoError(err)
		ctx.EqualValues(1, len(ids))
		ctx.EqualValues(entities.session1.Id, ids[0])

		query = fmt.Sprintf(`service = "%v"`, entities.service2.Id)
		ids, _, err = ctx.stores.Session.QueryIds(tx, query)
		ctx.NoError(err)
		ctx.EqualValues(2, len(ids))
		ctx.True(stringz.Contains(ids, entities.session2.Id))
		ctx.True(stringz.Contains(ids, entities.session3.Id))
		return nil
	})
	ctx.NoError(err)
}

func (ctx *TestContext) testUpdateSessions(_ *testing.T) {
	ctx.CleanupAll()
	entities := ctx.createSessionTestEntities()
	earlier := time.Now()
	time.Sleep(time.Millisecond * 50)

	mutateCtx := change.New().NewMutateContext()
	err := ctx.GetDb().Update(mutateCtx, func(mutateCtx boltz.MutateContext) error {
		tx := mutateCtx.Tx()
		original, err := ctx.stores.Session.LoadOneById(tx, entities.session1.Id)
		ctx.NoError(err)
		ctx.NotNil(original)

		session, err := ctx.stores.Session.LoadOneById(tx, entities.session1.Id)
		ctx.NoError(err)
		ctx.NotNil(session)

		tags := ctx.CreateTags()
		now := time.Now()
		session.UpdatedAt = earlier
		session.CreatedAt = now
		session.Tags = tags

		err = ctx.stores.Session.Update(mutateCtx, session, nil)
		ctx.NoError(err)
		loaded, err := ctx.stores.Session.LoadOneById(tx, entities.session1.Id)
		ctx.NoError(err)
		ctx.NotNil(loaded)
		ctx.EqualValues(original.CreatedAt, loaded.CreatedAt)
		ctx.True(loaded.UpdatedAt.Equal(now) || loaded.UpdatedAt.After(now))
		session.CreatedAt = loaded.CreatedAt
		session.UpdatedAt = loaded.UpdatedAt
		ctx.True(cmp.Equal(session, loaded, cmpopts.IgnoreFields(Session{}, "ApiSession")), cmp.Diff(session, loaded))
		return nil
	})
	ctx.NoError(err)
}

func (ctx *TestContext) testDeleteSessions(_ *testing.T) {
	ctx.CleanupAll()
	entities := ctx.createSessionTestEntities()
	boltztest.RequireDelete(ctx, entities.session1, apiSessionsSessionsIdxPath)
	boltztest.RequireDelete(ctx, entities.session2, apiSessionsSessionsIdxPath)
	boltztest.RequireDelete(ctx, entities.session3, apiSessionsSessionsIdxPath)
}

func NewSession(apiSessionId, serviceId string) *Session {
	return &Session{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		Token:         eid.New(),
		ApiSessionId:  apiSessionId,
		ServiceId:     serviceId,
		Type:          SessionTypeDial,
	}
}
