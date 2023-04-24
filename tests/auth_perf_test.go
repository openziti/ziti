//go:build apitests && perftests
// +build apitests,perftests

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

package tests

import (
	"github.com/openziti/sdk-golang/ziti"
	"github.com/rcrowley/go-metrics"
	"os"
	"testing"
	"time"
)

func Test_AuthPerformance(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	reg := metrics.NewRegistry()
	histogram := newHistogram()
	meter := metrics.NewMeter()
	ctx.Req.NoError(reg.Register("auth.time", histogram))
	ctx.Req.NoError(reg.Register("auth.rate", meter))

	go metrics.Write(reg, 5*time.Second, os.Stdout)

	identity := ctx.AdminManagementSession.RequireNewIdentityWithOtt(false)
	config := ctx.AdminManagementSession.testContext.EnrollIdentity(identity.Id)

	context, err := ziti.NewContext(config)
	ctx.Req.NoError(err)

	for i := 0; i < 25; i++ {
		go func() {
			for {
				start := time.Now()
				err := context.Authenticate()
				ctx.Req.NoError(err)
				meter.Mark(1)
				done := time.Now()
				diff := done.Sub(start)
				histogram.Update(diff.Milliseconds())
			}
		}()
	}

	time.Sleep(time.Hour)
}

func Test_CombinedSessionCreatePerformance(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	reg := metrics.NewRegistry()
	apiSessionCreateHistogram := newHistogram()
	apiSessionCreateMeter := metrics.NewMeter()
	ctx.Req.NoError(reg.Register("api-session.create.time", apiSessionCreateHistogram))
	ctx.Req.NoError(reg.Register("api-session.create.rate", apiSessionCreateMeter))

	sessionCreateHistogram := newHistogram()
	sessionCreateMeter := metrics.NewMeter()
	ctx.Req.NoError(reg.Register("session.create.time", sessionCreateHistogram))
	ctx.Req.NoError(reg.Register("session.create.rate", sessionCreateMeter))

	go metrics.Write(reg, 5*time.Second, os.Stdout)

	ctx.CreateEnrollAndStartEdgeRouter()

	identity := ctx.AdminManagementSession.RequireNewIdentityWithOtt(false)
	config := ctx.AdminManagementSession.testContext.EnrollIdentity(identity.Id)

	context, err := ziti.NewContext(config)
	ctx.Req.NoError(err)

	err = context.Authenticate()
	ctx.Req.NoError(err)

	client := context.(*ziti.ContextImpl).CtrlClt

	service := ctx.AdminManagementSession.RequireNewServiceAccessibleToAll("smartrouting")

	for i := 0; i < 25; i++ {
		go func() {
			for {
				start := time.Now()
				err := context.Authenticate()
				ctx.Req.NoError(err)
				apiSessionCreateMeter.Mark(1)
				done := time.Now()
				diff := done.Sub(start)
				apiSessionCreateHistogram.Update(diff.Milliseconds())

				start = time.Now()

				_, err = client.CreateSession(service.Id, ziti.SessionType(ziti.SessionDial))
				ctx.Req.NoError(err)
				sessionCreateMeter.Mark(1)
				done = time.Now()
				diff = done.Sub(start)
				sessionCreateHistogram.Update(diff.Milliseconds())

			}
		}()
	}

	time.Sleep(time.Hour)
}

func Test_SessionCreatePerformance(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	reg := metrics.NewRegistry()
	histogram := newHistogram()
	meter := metrics.NewMeter()
	ctx.Req.NoError(reg.Register("auth.time", histogram))
	ctx.Req.NoError(reg.Register("auth.rate", meter))

	go metrics.Write(reg, 5*time.Second, os.Stdout)

	ctx.CreateEnrollAndStartEdgeRouter()

	service := ctx.AdminManagementSession.RequireNewServiceAccessibleToAll("smartrouting")
	identity := ctx.AdminManagementSession.RequireNewIdentityWithOtt(false)
	config := ctx.AdminManagementSession.testContext.EnrollIdentity(identity.Id)

	context, err := ziti.NewContext(config)
	ctx.Req.NoError(err)

	err = context.Authenticate()
	ctx.Req.NoError(err)

	client := context.(*ziti.ContextImpl).CtrlClt

	for i := 0; i < 50; i++ {
		go func() {
			for {
				start := time.Now()
				_, err := client.CreateSession(service.Id, ziti.SessionType(ziti.SessionDial))
				ctx.Req.NoError(err)
				meter.Mark(1)
				done := time.Now()
				diff := done.Sub(start)
				histogram.Update(diff.Milliseconds())
			}
		}()
	}

	time.Sleep(time.Hour)
}
