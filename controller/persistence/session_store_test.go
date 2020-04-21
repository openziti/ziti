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
	"github.com/google/uuid"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/netfoundry/ziti-foundation/util/stringz"
	"go.etcd.io/bbolt"
	"testing"
	"time"
)

func Test_SessionStore(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	t.Run("test create invalid sessions", ctx.testCreateInvalidSessions)
	t.Run("test create sessions", ctx.testCreateSessions)
	t.Run("test create session certs", ctx.testCreateSessionsCerts)
	t.Run("test load/query sessions", ctx.testLoadQuerySessions)
	t.Run("test update sessions", ctx.testUpdateSessions)
	t.Run("test delete sessions", ctx.testDeleteSessions)
}

func (ctx *TestContext) testCreateInvalidSessions(_ *testing.T) {
	defer ctx.cleanupAll()

	identity := ctx.requireNewIdentity("test-user", false)
	apiSession := NewApiSession(identity.Id)
	ctx.RequireCreate(apiSession)

	service := ctx.requireNewService("test-service")

	session := NewSession("", service.Id)
	err := ctx.Create(session)
	ctx.EqualError(err, "index on sessions.apiSession does not allow null or empty values")

	session.ApiSessionId = "invalid-id"
	err = ctx.Create(session)
	ctx.EqualError(err, fmt.Sprintf("apiSession with id %v not found", session.ApiSessionId))

	session.ApiSessionId = apiSession.Id
	session.ServiceId = ""
	err = ctx.Create(session)
	ctx.EqualError(err, "index on sessions.service does not allow null or empty values")

	session.ServiceId = "invalid-id"
	err = ctx.Create(session)
	ctx.EqualError(err, fmt.Sprintf("service with id %v not found", session.ServiceId))

	session.ServiceId = service.Id
	err = ctx.Create(session)
	ctx.NoError(err)
	err = ctx.Create(session)
	ctx.EqualError(err, fmt.Sprintf("an entity of type session already exists with id %v", session.Id))
}

func (ctx *TestContext) testCreateSessions(_ *testing.T) {
	ctx.cleanupAll()

	identity := ctx.requireNewIdentity("Jojo", false)
	apiSession := NewApiSession(identity.Id)
	ctx.RequireCreate(apiSession)
	service := ctx.requireNewService("test-service")
	session := NewSession(apiSession.Id, service.Id)
	ctx.RequireCreate(session)
	ctx.ValidateBaseline(session)

	sessionIds := ctx.getRelatedIds(apiSession, EntityTypeSessions)
	ctx.EqualValues(1, len(sessionIds))
	ctx.EqualValues(session.Id, sessionIds[0])

	sessionIds = ctx.getRelatedIds(service, EntityTypeSessions)
	ctx.EqualValues(1, len(sessionIds))
	ctx.EqualValues(session.Id, sessionIds[0])

	session2 := NewSession(apiSession.Id, service.Id)
	session2.Tags = ctx.CreateTags()
	ctx.RequireCreate(session2)
	ctx.ValidateBaseline(session2)

	sessionIds = ctx.getRelatedIds(apiSession, EntityTypeSessions)
	ctx.EqualValues(2, len(sessionIds))
	ctx.True(stringz.Contains(sessionIds, session.Id))
	ctx.True(stringz.Contains(sessionIds, session2.Id))

	sessionIds = ctx.getRelatedIds(service, EntityTypeSessions)
	ctx.EqualValues(2, len(sessionIds))
	ctx.True(stringz.Contains(sessionIds, session.Id))
	ctx.True(stringz.Contains(sessionIds, session2.Id))

	ctx.RequireDelete(session)

	sessionIds = ctx.getRelatedIds(apiSession, EntityTypeSessions)
	ctx.EqualValues(1, len(sessionIds))
	ctx.EqualValues(session2.Id, sessionIds[0])

	sessionIds = ctx.getRelatedIds(service, EntityTypeSessions)
	ctx.EqualValues(1, len(sessionIds))
	ctx.EqualValues(session2.Id, sessionIds[0])
}

func (ctx *TestContext) testCreateSessionsCerts(_ *testing.T) {
	ctx.cleanupAll()

	sessionCert1 := &SessionCert{
		Id:          "a" + uuid.New().String()[1:],
		Cert:        uuid.New().String(),
		Fingerprint: uuid.New().String(),
		ValidFrom:   time.Now(),
		ValidTo:     time.Now().Add(10 * time.Hour),
	}

	sessionCert2 := &SessionCert{
		Id:          "b" + uuid.New().String()[1:],
		Cert:        uuid.New().String(),
		Fingerprint: uuid.New().String(),
		ValidFrom:   time.Now().Add(-1 * time.Hour),
		ValidTo:     time.Now().Add(5 * time.Hour),
	}

	identity := ctx.requireNewIdentity("Jojo", false)
	apiSession := NewApiSession(identity.Id)
	ctx.RequireCreate(apiSession)
	service := ctx.requireNewService("test-service")
	session := NewSession(apiSession.Id, service.Id)
	session.Certs = []*SessionCert{sessionCert1, sessionCert2}
	ctx.RequireCreate(session)

	var certs []*SessionCert
	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		var err error
		certs, err = ctx.stores.Session.LoadCerts(tx, session.Id)
		return err
	})
	ctx.NoError(err)
	ctx.NotNil(certs)
	ctx.Equal(2, len(certs))
	ctx.True(cmp.Equal(certs, session.Certs), cmp.Diff(certs, session.Certs))
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
	identity1 := ctx.requireNewIdentity("admin1", true)

	apiSession1 := NewApiSession(identity1.Id)
	ctx.RequireCreate(apiSession1)

	apiSession2 := NewApiSession(identity1.Id)
	ctx.RequireCreate(apiSession2)

	service1 := ctx.requireNewService(uuid.New().String())
	service2 := ctx.requireNewService(uuid.New().String())

	session1 := NewSession(apiSession1.Id, service1.Id)
	ctx.RequireCreate(session1)

	session2 := NewSession(apiSession2.Id, service2.Id)
	ctx.RequireCreate(session2)

	session3 := NewSession(apiSession2.Id, service2.Id)
	ctx.RequireCreate(session3)

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
	ctx.cleanupAll()

	entities := ctx.createSessionTestEntities()

	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		session, err := ctx.stores.Session.LoadOneByToken(tx, entities.session1.Token)
		ctx.NoError(err)
		ctx.NotNil(session)
		ctx.EqualValues(entities.session1.Id, session.Id)

		query := fmt.Sprintf(`apiSession = "%v"`, entities.apiSession1.Id)
		session, err = ctx.stores.Session.LoadOneByQuery(tx, query)
		ctx.NoError(err)
		ctx.NotNil(session)
		ctx.EqualValues(entities.session1.Id, session.Id)

		query = fmt.Sprintf(`service = "%v"`, entities.service2.Id)
		ids, _, err := ctx.stores.Session.QueryIds(tx, query)
		ctx.NoError(err)
		ctx.EqualValues(2, len(ids))
		ctx.True(stringz.Contains(ids, entities.session2.Id))
		ctx.True(stringz.Contains(ids, entities.session3.Id))
		return nil
	})
	ctx.NoError(err)
}

func (ctx *TestContext) testUpdateSessions(_ *testing.T) {
	ctx.cleanupAll()
	entities := ctx.createSessionTestEntities()
	earlier := time.Now()
	time.Sleep(time.Millisecond * 50)

	err := ctx.GetDb().Update(func(tx *bbolt.Tx) error {
		original, err := ctx.stores.Session.LoadOneById(tx, entities.session1.Id)
		ctx.NoError(err)
		ctx.NotNil(original)

		session, err := ctx.stores.Session.LoadOneById(tx, entities.session1.Id)
		ctx.NoError(err)
		ctx.NotNil(session)

		tags := ctx.CreateTags()
		now := time.Now()
		session.Token = uuid.New().String()
		session.UpdatedAt = earlier
		session.CreatedAt = now
		session.ApiSessionId = entities.apiSession2.Id
		session.ServiceId = entities.service2.Id
		session.Tags = tags

		err = ctx.stores.Session.Update(boltz.NewMutateContext(tx), session, nil)
		ctx.NoError(err)
		loaded, err := ctx.stores.Session.LoadOneById(tx, entities.session1.Id)
		ctx.NoError(err)
		ctx.NotNil(loaded)
		ctx.EqualValues(original.CreatedAt, loaded.CreatedAt)
		ctx.True(loaded.UpdatedAt.Equal(now) || loaded.UpdatedAt.After(now))
		session.CreatedAt = loaded.CreatedAt
		session.UpdatedAt = loaded.UpdatedAt
		ctx.True(cmp.Equal(session, loaded), cmp.Diff(session, loaded))
		return nil
	})
	ctx.NoError(err)
}

func (ctx *TestContext) testDeleteSessions(_ *testing.T) {
	ctx.cleanupAll()
	entities := ctx.createSessionTestEntities()
	ctx.RequireDelete(entities.session1)
	ctx.RequireDelete(entities.session2)
	ctx.RequireDelete(entities.session3)
}
