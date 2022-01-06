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
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/openziti/edge/eid"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/openziti/foundation/util/stringz"
	"go.etcd.io/bbolt"
	"testing"
	"time"
)

func Test_SessionStore(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	t.Run("test create invalid sessions", ctx.testCreateInvalidSessions)
	t.Run("test update invalid sessions", ctx.testUpdateInvalidSessions)
	t.Run("test create sessions", ctx.testCreateSessions)
	t.Run("test create session certs", ctx.testCreateSessionsCerts)
	t.Run("test load/query sessions", ctx.testLoadQuerySessions)
	t.Run("test update sessions", ctx.testUpdateSessions)
	t.Run("test delete sessions", ctx.testDeleteSessions)
}

func (ctx *TestContext) testCreateInvalidSessions(_ *testing.T) {
	defer ctx.CleanupAll()

	identity := ctx.RequireNewIdentity("test-user", false)
	apiSession := NewApiSession(identity.Id)
	ctx.RequireCreate(apiSession)

	service := ctx.RequireNewService("test-service")

	session := NewSession("", service.Id)
	err := ctx.Create(session)
	ctx.EqualError(err, "fk constraint on sessions.apiSession does not allow null or empty values")

	session.ApiSessionId = "invalid-id"
	err = ctx.Create(session)
	ctx.EqualError(err, fmt.Sprintf("apiSession with id %v not found", session.ApiSessionId))

	session.ApiSessionId = apiSession.Id
	session.ServiceId = ""
	err = ctx.Create(session)
	ctx.EqualError(err, "fk constraint on sessions.service does not allow null or empty values")

	session.ServiceId = "invalid-id"
	err = ctx.Create(session)
	ctx.EqualError(err, fmt.Sprintf("service with id %v not found", session.ServiceId))

	session.ServiceId = service.Id
	err = ctx.Create(session)
	ctx.NoError(err)
	err = ctx.Create(session)
	ctx.EqualError(err, fmt.Sprintf("an entity of type session already exists with id %v", session.Id))
}

func (ctx *TestContext) testUpdateInvalidSessions(_ *testing.T) {
	defer ctx.CleanupAll()

	identity := ctx.RequireNewIdentity("test-user", false)
	apiSession := NewApiSession(identity.Id)
	ctx.RequireCreate(apiSession)

	service := ctx.RequireNewService("test-service")

	session := NewSession(apiSession.Id, service.Id)
	ctx.RequireCreate(session)

	session.ApiSessionId = "invalid-id"
	err := ctx.Update(session)
	ctx.EqualError(err, fmt.Sprintf("apiSession with id %v not found", session.ApiSessionId))

	session.ApiSessionId = apiSession.Id
	session.ServiceId = ""
	err = ctx.Update(session)
	ctx.EqualError(err, "fk constraint on sessions.service does not allow null or empty values")

	session.ServiceId = "invalid-id"
	err = ctx.Update(session)
	ctx.EqualError(err, fmt.Sprintf("service with id %v not found", session.ServiceId))

	ctx.RequireDelete(session)
	ctx.ValidateDeleted(session.Id)

	err = ctx.Update(session)
	ctx.EqualError(err, fmt.Sprintf("session with id %v not found", session.Id))
}

func (ctx *TestContext) testCreateSessions(_ *testing.T) {
	ctx.CleanupAll()

	compareOpts := cmpopts.IgnoreFields(Session{}, "ApiSession")

	identity := ctx.RequireNewIdentity("Jojo", false)
	apiSession := NewApiSession(identity.Id)
	ctx.RequireCreate(apiSession)
	service := ctx.RequireNewService("test-service")
	session := NewSession(apiSession.Id, service.Id)
	ctx.RequireCreate(session)
	ctx.ValidateBaseline(session, compareOpts)

	session2 := NewSession(apiSession.Id, service.Id)
	ctx.RequireCreate(session2)
	ctx.ValidateBaseline(session2, compareOpts)

	service2 := ctx.RequireNewService("test-service-2")
	session3 := NewSession(apiSession.Id, service2.Id)
	session3.Tags = ctx.CreateTags()
	ctx.RequireCreate(session3)
	ctx.ValidateBaseline(session3, compareOpts)

	ctx.RequireDelete(service2)
	ctx.ValidateDeleted(session3.Id)
	ctx.RequireReload(session)
	ctx.RequireReload(session2)

	err := ctx.Delete(apiSession)
	ctx.NoError(err)

	done, err := ctx.GetStores().EventualEventer.Trigger()
	ctx.NoError(err)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		ctx.Fail("did not receive done notification from eventual eventer")

	}

	ctx.ValidateDeleted(apiSession.GetId())

	ctx.ValidateDeleted(session.Id)
	ctx.ValidateDeleted(session2.Id)

}

func (ctx *TestContext) testCreateSessionsCerts(_ *testing.T) {
	ctx.CleanupAll()

	sessionCert1 := &SessionCert{
		Id:          "a" + eid.New()[1:],
		Cert:        eid.New(),
		Fingerprint: eid.New(),
		ValidFrom:   time.Now(),
		ValidTo:     time.Now().Add(10 * time.Hour),
	}

	sessionCert2 := &SessionCert{
		Id:          "b" + eid.New()[1:],
		Cert:        eid.New(),
		Fingerprint: eid.New(),
		ValidFrom:   time.Now().Add(-1 * time.Hour),
		ValidTo:     time.Now().Add(5 * time.Hour),
	}

	identity := ctx.RequireNewIdentity("Jojo", false)
	apiSession := NewApiSession(identity.Id)
	ctx.RequireCreate(apiSession)
	service := ctx.RequireNewService("test-service")
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
	identity1 := ctx.RequireNewIdentity("admin1", true)

	apiSession1 := NewApiSession(identity1.Id)
	ctx.RequireCreate(apiSession1)

	apiSession2 := NewApiSession(identity1.Id)
	ctx.RequireCreate(apiSession2)

	service1 := ctx.RequireNewService(eid.New())
	service2 := ctx.RequireNewService(eid.New())

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

	err := ctx.GetDb().Update(func(tx *bbolt.Tx) error {
		original, err := ctx.stores.Session.LoadOneById(tx, entities.session1.Id)
		ctx.NoError(err)
		ctx.NotNil(original)

		session, err := ctx.stores.Session.LoadOneById(tx, entities.session1.Id)
		ctx.NoError(err)
		ctx.NotNil(session)

		tags := ctx.CreateTags()
		now := time.Now()
		session.Token = eid.New()
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
		ctx.True(cmp.Equal(session, loaded, cmpopts.IgnoreFields(Session{}, "ApiSession")), cmp.Diff(session, loaded))
		return nil
	})
	ctx.NoError(err)
}

func (ctx *TestContext) testDeleteSessions(_ *testing.T) {
	ctx.CleanupAll()
	entities := ctx.createSessionTestEntities()
	ctx.RequireDelete(entities.session1)
	ctx.RequireDelete(entities.session2)
	ctx.RequireDelete(entities.session3)
}

func NewSession(apiSessionId, serviceId string) *Session {
	return &Session{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		Token:         eid.New(),
		ApiSessionId:  apiSessionId,
		ServiceId:     serviceId,
		Type:          SessionTypeDial,
		Certs:         nil,
	}
}
