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

package policy

import (
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/eid"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/sirupsen/logrus"
	"testing"
	"time"
)

func Test_SessionEnforcer(t *testing.T) {
	ctx := &enforcerTestContext{
		TestContext: model.NewTestContext(t),
	}

	defer ctx.Cleanup()
	ctx.Init()

	ctx.testSessionsCleanup()
}

type enforcerTestContext struct {
	*model.TestContext
}

func (ctx *enforcerTestContext) testSessionsCleanup() {
	logrus.SetLevel(logrus.DebugLevel)
	ctx.CleanupAll()

	compareOpts := cmpopts.IgnoreFields(persistence.Session{}, "ApiSession")

	identity := ctx.RequireNewIdentity("Jojo", false)
	apiSession := persistence.NewApiSession(identity.Id)
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

	ctx.RequireReload(session)
	ctx.RequireReload(session2)

	enforcer := &ApiSessionEnforcer{
		appEnv:         ctx,
		sessionTimeout: -time.Second,
	}

	ctx.NoError(enforcer.Run())
	ctx.ValidateDeleted(apiSession.Id)
	ctx.ValidateDeleted(session.Id)
	ctx.ValidateDeleted(session2.Id)
	ctx.ValidateDeleted(session3.Id)
}

func NewSession(apiSessionId, serviceId string) *persistence.Session {
	return &persistence.Session{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		Token:         eid.New(),
		ApiSessionId:  apiSessionId,
		ServiceId:     serviceId,
		Type:          persistence.SessionTypeDial,
		Certs:         nil,
	}
}
