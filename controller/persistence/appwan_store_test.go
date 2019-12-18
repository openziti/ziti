/*
	Copyright 2019 Netfoundry, Inc.

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
	"github.com/google/uuid"
	"testing"
)

func Test_AppWanSessionStore(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	t.Run("test create invalid appwans", ctx.testCreateInvalidAppwans)
	//t.Run("test create appwans", ctx.testCreateAppwans)
	//t.Run("test load/query appwans", ctx.testLoadQueryAppwans)
	//t.Run("test update appwans", ctx.testUpdateAppwans)
	//t.Run("test delete appwans", ctx.testDeleteAppwans)}
}

func (ctx *TestContext) testCreateInvalidAppwans(_ *testing.T) {
	defer ctx.cleanupAll()

	appwan := NewAppwan("")
	err := ctx.create(appwan)
	ctx.EqualError(err, "index on appwans.name does not allow null or empty values")

	appwan = NewAppwan(uuid.New().String())
	appwan.Identities = append(appwan.Identities, uuid.New().String())
	err = ctx.create(appwan)
	ctx.EqualError(err, fmt.Sprintf("can't link to unknown identities with id %v", appwan.Identities[0]))

	appwan.Identities = nil
	appwan.Services = append(appwan.Services, uuid.New().String())
	err = ctx.create(appwan)
	ctx.EqualError(err, fmt.Sprintf("can't link to unknown services with id %v", appwan.Services[0]))
}
