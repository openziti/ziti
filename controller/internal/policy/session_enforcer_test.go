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

package policy

import (
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/storage/boltztest"
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/model"
	"github.com/sirupsen/logrus"
	"testing"
	"time"
)

func Test_SessionEnforcer(t *testing.T) {
	ctx := &enforcerTestContext{
		TestContext: model.NewTestContext(t),
	}
	ctx.Init()
	defer ctx.Cleanup()

	ctx.testSessionsCleanup()
}

type enforcerTestContext struct {
	*model.TestContext
}

func (ctx *enforcerTestContext) testSessionsCleanup() {
	logrus.SetLevel(logrus.DebugLevel)
	ctx.CleanupAll()

	compareOpts := cmpopts.IgnoreFields(db.Session{}, "ApiSession")

	identity := ctx.RequireNewIdentity("Jojo", false)
	apiSession := db.NewApiSession(identity.Id)
	boltztest.RequireCreate(ctx, apiSession)
	service := ctx.RequireNewService("test-service")
	session := NewSession(apiSession.Id, service.Id)
	boltztest.RequireCreate(ctx, session)
	boltztest.ValidateBaseline(ctx, session, compareOpts)

	session2 := NewSession(apiSession.Id, service.Id)
	session2.Type = db.PolicyTypeBindName
	boltztest.RequireCreate(ctx, session2)
	boltztest.ValidateBaseline(ctx, session2, compareOpts)

	service2 := ctx.RequireNewService("test-service-2")
	session3 := NewSession(apiSession.Id, service2.Id)
	session3.Tags = ctx.CreateTags()
	boltztest.RequireCreate(ctx, session3)
	boltztest.ValidateBaseline(ctx, session3, compareOpts)

	boltztest.RequireReload(ctx, session)
	boltztest.RequireReload(ctx, session2)

	enforcer := &ApiSessionEnforcer{
		appEnv:         ctx,
		sessionTimeout: -time.Second,
	}

	ctx.NoError(enforcer.Run())

	done, err := ctx.GetStores().EventualEventer.Trigger()
	ctx.NoError(err)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		ctx.Fail("did not receive done notification from eventual eventer")
	}

	boltztest.ValidateDeleted(ctx, apiSession.Id)
	boltztest.ValidateDeleted(ctx, session.Id)
	boltztest.ValidateDeleted(ctx, session2.Id)
	boltztest.ValidateDeleted(ctx, session3.Id)
}

func NewSession(apiSessionId, serviceId string) *db.Session {
	return &db.Session{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		Token:         eid.New(),
		ApiSessionId:  apiSessionId,
		ServiceId:     serviceId,
		Type:          db.SessionTypeDial,
	}
}
