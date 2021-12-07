//go:build dataflow
// +build dataflow

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

package tests

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/pkg/errors"
	"sync/atomic"
	"testing"
	"time"
)

func Test_ManualStart(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	xt.GlobalRegistry().RegisterFactory(&testFailoverStrategyFactory{})
	t.Run("creating service and edge router", func(t *testing.T) {
		ctx.testContextChanged(t)
		ctx.RequireAdminManagementApiLogin()

		service := ctx.AdminManagementSession.RequireNewServiceAccessibleToAll("test-failover")

		ctx.CreateEnrollAndStartEdgeRouter()

		log := pfxlog.Logger()

		type host struct {
			id       *identity
			context  ziti.Context
			listener edge.Listener
		}

		t.Run("listen and send data", func(t *testing.T) {
			ctx.testContextChanged(t)

			log.Info("starting listener1")
			host1 := &host{}
			host1.id, host1.context = ctx.AdminManagementSession.RequireCreateSdkContext()
			defer host1.context.Close()

			listener, err := host1.context.ListenWithOptions(service.Name, &ziti.ListenOptions{
				Precedence:  ziti.PrecedenceRequired,
				ManualStart: true,
			})
			host1.listener = listener
			ctx.Req.NoError(err)

			log.Info("started listener1")

			defer func() { _ = host1.listener.Close() }()

			log.Info("starting listener2")
			host2 := &host{}
			host2.id, host2.context = ctx.AdminManagementSession.RequireCreateSdkContext()
			defer host2.context.Close()

			listener, err = host2.context.ListenWithOptions(service.Name, &ziti.ListenOptions{
				Precedence:  ziti.PrecedenceDefault,
				ManualStart: false,
			})
			host2.listener = listener
			ctx.Req.NoError(err)
			log.Info("started listener1")

			defer func() { _ = host2.listener.Close() }()

			go func() {
				count := 0
				for {
					conn, err := host1.listener.AcceptEdge()
					log.Info("accepted connection from listener1")
					if err != nil {
						pfxlog.Logger().WithError(err).Error("failure during accept")
						return
					}
					if count < 10 {
						if err := conn.CompleteAcceptSuccess(); err != nil {
							pfxlog.Logger().WithError(err).Error("failure during complete accept success")
							return
						}
						if _, err := conn.Write([]byte("one")); err != nil {
							pfxlog.Logger().WithError(err).Error("failure during write")
							return
						}
					} else {
						conn.CompleteAcceptFailed(errors.New("test error"))
					}
					_ = conn.Close()
					count++
				}
			}()

			go func() {
				for {
					conn, err := host2.listener.AcceptEdge()
					log.Info("accepted connection from listener2")
					if err != nil {
						pfxlog.Logger().WithError(err).Error("failure during accept")
						return
					}
					if _, err := conn.Write([]byte("two")); err != nil {
						pfxlog.Logger().WithError(err).Error("failure during write")
						return
					}
					_ = conn.Close()
				}
			}()

			_, context := ctx.AdminManagementSession.RequireCreateSdkContext()
			defer context.Close()

			for i := 0; i < 10; i++ {
				log.Infof("dialing: %v", i)
				conn := ctx.WrapConn(context.Dial(service.Name))
				log.Infof("connected: %v", i)
				val := conn.ReadString(3, time.Second)
				ctx.Req.Equal("one", val)
				ctx.Req.NoError(conn.Close())
			}

			// allow up to three failures
			for i := 0; i < 3; i++ {
				log.Infof("dialing: %v", i)
				conn, err := context.Dial(service.Name)
				if err == nil {
					conn := ctx.WrapConn(conn, err)
					val := conn.ReadString(3, time.Second)
					ctx.Req.Equal("two", val)
					ctx.Req.NoError(conn.Close())
				} else {
					pfxlog.Logger().WithError(err).Info("allowed dial failure")
				}
			}

			for i := 0; i < 10; i++ {
				log.Infof("dialing: %v", i)
				conn := ctx.WrapConn(context.Dial(service.Name))
				val := conn.ReadString(3, time.Second)
				ctx.Req.Equal("two", val)
				ctx.Req.NoError(conn.Close())
			}
		})

	})
}

type testFailoverStrategyFactory struct{}

func (self *testFailoverStrategyFactory) GetStrategyName() string {
	return "test-failover"
}

func (self *testFailoverStrategyFactory) NewStrategy() xt.Strategy {
	return &testFailoverStrategy{}
}

type testFailoverStrategy struct {
	xt.DefaultEventVisitor
	failCount int32
}

func (self *testFailoverStrategy) VisitDialFailed(event xt.TerminatorEvent) {
	failCount := atomic.AddInt32(&self.failCount, 1)
	if failCount >= 3 {
		xt.GlobalCosts().SetPrecedence(event.GetTerminator().GetId(), xt.Precedences.Failed)
	}
}

func (self *testFailoverStrategy) VisitDialSucceeded(xt.TerminatorEvent) {
	atomic.StoreInt32(&self.failCount, 0)
}

func (self *testFailoverStrategy) Select(terminators []xt.CostedTerminator) (xt.CostedTerminator, error) {
	return terminators[0], nil
}

func (self *testFailoverStrategy) NotifyEvent(event xt.TerminatorEvent) {
	event.Accept(self)
}

func (self *testFailoverStrategy) HandleTerminatorChange(xt.StrategyChangeEvent) error {
	return nil
}
