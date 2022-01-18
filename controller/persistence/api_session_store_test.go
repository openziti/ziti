/*
	Copyright NetFoundry, Inc.

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

package persistence

import (
	"fmt"
	"github.com/google/go-cmp/cmp"
	"github.com/openziti/edge/eid"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/openziti/foundation/util/stringz"
	"go.etcd.io/bbolt"
	"testing"
	"time"
)

func Test_ApiSessionStore(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	t.Run("test create invalid api sessions", ctx.testCreateInvalidApiSessions)
	t.Run("test create api sessions", ctx.testCreateApiSessions)
	t.Run("test load/query api sessions", ctx.testLoadQueryApiSessions)
	t.Run("test update api sessions", ctx.testUpdateApiSessions)
	t.Run("test delete api sessions", ctx.testDeleteApiSessions)
}

func (ctx *TestContext) testCreateInvalidApiSessions(t *testing.T) {
	ctx.BaseTestContext.NextTest(t)
	defer ctx.CleanupAll()

	apiSession := NewApiSession(eid.New())
	err := ctx.Create(apiSession)
	ctx.EqualError(err, fmt.Sprintf("identity with id %v not found", apiSession.IdentityId))

	apiSession.IdentityId = ""
	err = ctx.Create(apiSession)
	ctx.EqualError(err, "fk constraint on apiSessions.identity does not allow null or empty values")

	identity := ctx.RequireNewIdentity("user1", false)
	apiSession.Token = ""
	apiSession.IdentityId = identity.Id

	err = ctx.Create(apiSession)
	ctx.EqualError(err, "index on apiSessions.token does not allow null or empty values")

	apiSession.Token = eid.New()
	err = ctx.Create(apiSession)
	ctx.NoError(err)
	err = ctx.Create(apiSession)
	ctx.EqualError(err, fmt.Sprintf("an entity of type apiSession already exists with id %v", apiSession.GetId()))
}

func (ctx *TestContext) testCreateApiSessions(t *testing.T) {
	ctx.BaseTestContext.NextTest(t)
	ctx.CleanupAll()

	identity := ctx.RequireNewIdentity("Jojo", false)

	apiSession := NewApiSession(identity.Id)
	ctx.RequireCreate(apiSession)

	ctx.ValidateBaseline(apiSession)

	apiSession2 := NewApiSession(identity.Id)
	apiSession2.Tags = ctx.CreateTags()
	ctx.RequireCreate(apiSession2)

	ctx.ValidateBaseline(apiSession2)

	err := ctx.Delete(apiSession)
	ctx.NoError(err)
	ctx.RequireDelete(identity)

	done, err := ctx.GetStores().EventualEventer.Trigger()
	ctx.NoError(err)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		ctx.Fail("did not receive done notification from eventual eventer")

	}

	ctx.ValidateDeleted(apiSession.Id)
	ctx.ValidateDeleted(apiSession2.Id)
}

type apiSessionTestEntities struct {
	identity1   *Identity
	identity2   *Identity
	apiSession1 *ApiSession
	apiSession2 *ApiSession
	apiSession3 *ApiSession
	serviceId   string
	session     *Session
}

func (ctx *TestContext) createApiSessionTestEntities() *apiSessionTestEntities {
	identity1 := ctx.RequireNewIdentity("admin1", true)
	identity2 := ctx.RequireNewIdentity("user1", false)

	apiSession1 := NewApiSession(identity1.Id)
	ctx.RequireCreate(apiSession1)

	apiSession2 := NewApiSession(identity2.Id)
	ctx.RequireCreate(apiSession2)

	apiSession3 := NewApiSession(identity2.Id)
	ctx.RequireCreate(apiSession3)

	service := ctx.RequireNewService("test-service")
	session := &Session{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		Token:         eid.New(),
		ApiSessionId:  apiSession2.Id,
		ServiceId:     service.Id,
	}
	ctx.RequireCreate(session)

	return &apiSessionTestEntities{
		identity1:   identity1,
		identity2:   identity2,
		apiSession1: apiSession1,
		apiSession2: apiSession2,
		apiSession3: apiSession3,
		serviceId:   service.Id,
		session:     session,
	}
}

func (ctx *TestContext) testLoadQueryApiSessions(t *testing.T) {
	ctx.BaseTestContext.NextTest(t)
	ctx.CleanupAll()

	entities := ctx.createApiSessionTestEntities()

	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		apiSession, err := ctx.stores.ApiSession.LoadOneByToken(tx, entities.apiSession1.Token)
		ctx.NoError(err)
		ctx.NotNil(apiSession)
		ctx.EqualValues(entities.apiSession1.Id, apiSession.Id)

		query := fmt.Sprintf(`identity = "%v"`, entities.identity1.Id)
		apiSession, err = ctx.stores.ApiSession.LoadOneByQuery(tx, query)
		ctx.NoError(err)
		ctx.NotNil(apiSession)
		ctx.EqualValues(entities.apiSession1.Id, apiSession.Id)

		query = fmt.Sprintf(`identity = "%v"`, entities.identity2.Id)
		ids, _, err := ctx.stores.ApiSession.QueryIds(tx, query)
		ctx.NoError(err)
		ctx.EqualValues(2, len(ids))
		ctx.True(stringz.Contains(ids, entities.apiSession2.Id))
		ctx.True(stringz.Contains(ids, entities.apiSession3.Id))

		return nil
	})
	ctx.NoError(err)
}

func (ctx *TestContext) testUpdateApiSessions(t *testing.T) {
	ctx.BaseTestContext.NextTest(t)
	ctx.CleanupAll()
	entities := ctx.createApiSessionTestEntities()
	earlier := time.Now()

	err := ctx.GetDb().Update(func(tx *bbolt.Tx) error {
		original, err := ctx.stores.ApiSession.LoadOneById(tx, entities.apiSession1.Id)
		ctx.NoError(err)
		ctx.NotNil(original)

		apiSession, err := ctx.stores.ApiSession.LoadOneById(tx, entities.apiSession1.Id)
		ctx.NoError(err)
		ctx.NotNil(apiSession)

		tags := ctx.CreateTags()
		now := time.Now()
		apiSession.Token = eid.New()
		apiSession.UpdatedAt = earlier
		apiSession.CreatedAt = now
		apiSession.IdentityId = entities.identity2.Id
		apiSession.Tags = tags

		err = ctx.stores.ApiSession.Update(boltz.NewMutateContext(tx), apiSession, nil)
		ctx.NoError(err)
		loaded, err := ctx.stores.ApiSession.LoadOneById(tx, entities.apiSession1.Id)
		ctx.NoError(err)
		ctx.NotNil(loaded)
		ctx.EqualValues(original.CreatedAt, loaded.CreatedAt)
		ctx.True(loaded.UpdatedAt.Equal(now) || loaded.UpdatedAt.After(now))
		apiSession.CreatedAt = loaded.CreatedAt
		apiSession.UpdatedAt = loaded.UpdatedAt
		ctx.True(cmp.Equal(apiSession, loaded), cmp.Diff(apiSession, loaded))
		return nil
	})
	ctx.NoError(err)
}

func (ctx *TestContext) testDeleteApiSessions(t *testing.T) {
	ctx.BaseTestContext.NextTest(t)
	ctx.CleanupAll()
	entities := ctx.createApiSessionTestEntities()

	err := ctx.Delete(entities.apiSession1)
	ctx.NoError(err)

	err = ctx.Delete(entities.apiSession2)
	ctx.NoError(err)

	err = ctx.Delete(entities.apiSession3)
	ctx.NoError(err)

	done, err := ctx.GetStores().EventualEventer.Trigger()
	ctx.NoError(err)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		ctx.Fail("did not receive done notification from eventual eventer")

	}

	ctx.ValidateDeleted(entities.apiSession1.GetId())
	ctx.ValidateDeleted(entities.apiSession2.GetId())
	ctx.ValidateDeleted(entities.apiSession3.GetId())
}
