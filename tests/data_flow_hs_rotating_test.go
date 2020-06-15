// +build apitests

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
	"fmt"
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"math/rand"
	"sync/atomic"
	"testing"
	"time"
)

func Test_HSRotatingDataflow(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.teardown()
	ctx.startServer()
	ctx.requireAdminLogin()

	ctx.createEnrollAndStartEdgeRouter()

	t.Run("test client first smart routing", func(t *testing.T) {
		ctx.testContextChanged(t)
		testClientFirstWithStrategy(ctx, "smartrouting")
	})

	t.Run("test client first random", func(t *testing.T) {
		ctx.testContextChanged(t)
		testClientFirstWithStrategy(ctx, "random")
	})

	t.Run("test client first weighted", func(t *testing.T) {
		ctx.testContextChanged(t)
		testClientFirstWithStrategy(ctx, "weighted")
	})

	//t.Run("test server first smart routing", func(t *testing.T) {
	//	ctx.testContextChanged(t)
	//	testServerFirstWithStrategy(ctx, "smartrouting")
	//})
	//
	//t.Run("test server first random", func(t *testing.T) {
	//	ctx.testContextChanged(t)
	//	testServerFirstWithStrategy(ctx, "random")
	//})
	//
	//t.Run("test server first weighted", func(t *testing.T) {
	//	ctx.testContextChanged(t)
	//	testServerFirstWithStrategy(ctx, "weighted")
	//})
}

func testClientFirstWithStrategy(ctx *TestContext, strategy string) {
	service := ctx.AdminSession.requireNewServiceAccessibleToAll(strategy)
	fmt.Printf("service id: %v\n", service.id)

	serverContextC := make(chan ziti.Context, 3)
	doneC := make(chan struct{}, 1)
	var serverContexts []ziti.Context
	for i := 0; i < 3; i++ {
		_, context := ctx.AdminSession.requireCreateSdkContext()
		serverContexts = append(serverContexts, context)
		serverContextC <- context
	}

	go func() {
		logger := pfxlog.Logger()
		for {
			select {
			case context := <-serverContextC:
				listener, err := context.Listen(service.name)
				ctx.req.NoError(err)
				service := &clientFirstRotatingService{
					maxRequests: uint32(rand.Intn(5) + 5),
					closeCB:     func() { serverContextC <- context },
				}
				server := newTestServer(listener, service.Handle)
				server.start()
				logger.Infof("started new listener, servicing %v reads", service.maxRequests)
			case <-doneC:
				break
			}
		}
	}()

	clientIdentity := ctx.AdminSession.requireNewIdentityWithOtt(false)
	clientConfig := ctx.enrollIdentity(clientIdentity.id)
	clientContext := ziti.NewContextWithConfig(clientConfig)

	for i := 0; i < 250; i++ {
		conn := ctx.wrapConn(clientContext.Dial(service.name))

		name := uuid.New().String()
		conn.WriteString(name, time.Second)
		conn.ReadExpected("hello, "+name, time.Second)
		conn.RequireClose()
		fmt.Printf("%v: done\n", i+1)
	}

	close(doneC)

	for range serverContexts {
		conn := ctx.wrapConn(clientContext.Dial(service.name))
		conn.WriteString("quit", time.Second)
		conn.ReadExpected("ok", time.Second)
	}
}

type clientFirstRotatingService struct {
	maxRequests uint32
	requests    uint32
	closeCB     func()
	closing     concurrenz.AtomicBoolean
}

func (service *clientFirstRotatingService) Handle(conn *testServerConn) error {
	requests := atomic.AddUint32(&service.requests, 1)
	doClose := requests >= service.maxRequests
	logger := pfxlog.Logger()
	for {
		name, eof := conn.ReadString(1024, time.Minute)
		if eof {
			return nil
		}

		if name == "quit" {
			logger.Infof("%v-%v: received '%v' from client", conn.server.idx, conn.id, name)
			if err := conn.server.listener.UpdatePrecedence(edge.PrecedenceFailed); err != nil {
				logger.WithError(err).Error("failed to update precedence")
				return err
			}
			conn.WriteString("ok", time.Second)
			return conn.server.close()
		}

		conn.WriteString("hello, "+name, time.Second)
		atomic.AddUint32(&conn.server.msgCount, 1)
		if doClose && service.closing.CompareAndSwap(false, true) {
			logger.Infof("%v-%v: preparing to exit after serving %v requests. setting precedence to failed",
				conn.server.idx, conn.id, atomic.LoadUint32(&service.requests))
			if err := conn.server.listener.UpdatePrecedence(edge.PrecedenceFailed); err != nil {
				logger.WithError(err).Error("failed to update precedence")
				return err
			}
			service.closeCB()
			logger.Debugf("%v-%v: sleeping", conn.server.idx, conn.id)
			time.Sleep(100 * time.Millisecond)
			logger.Debugf("%v-%v: exiting", conn.server.idx, conn.id)
			return conn.server.close()
		}
	}
}

func testServerFirstWithStrategy(ctx *TestContext, strategy string) {
	service := ctx.AdminSession.requireNewServiceAccessibleToAll(strategy)
	fmt.Printf("service id: %v\n", service.id)

	serverContextC := make(chan ziti.Context, 3)
	doneC := make(chan struct{}, 1)
	var serverContexts []ziti.Context
	for i := 0; i < 3; i++ {
		_, context := ctx.AdminSession.requireCreateSdkContext()
		serverContexts = append(serverContexts, context)
		serverContextC <- context
	}

	go func() {
		logger := pfxlog.Logger()
		for {
			select {
			case context := <-serverContextC:
				listener, err := context.Listen(service.name)
				ctx.req.NoError(err)
				service := &serverFirstRotatingService{
					maxRequests: uint32(rand.Intn(5) + 5),
					closeCB:     func() { serverContextC <- context },
				}
				server := newTestServer(listener, service.Handle)
				server.start()
				logger.Infof("started new listener, servicing %v reads", service.maxRequests)
			case <-doneC:
				break
			}
		}
	}()

	clientIdentity := ctx.AdminSession.requireNewIdentityWithOtt(false)
	clientConfig := ctx.enrollIdentity(clientIdentity.id)
	clientContext := ziti.NewContextWithConfig(clientConfig)

	for i := 0; i < 250; i++ {
		conn := ctx.wrapConn(clientContext.Dial(service.name))
		name := conn.ReadString(1024, time.Second)
		conn.WriteString("hello, "+name, time.Second)
		log.Infof("%v: done", i+1)
	}

	close(doneC)

	for range serverContexts {
		conn := ctx.wrapConn(clientContext.Dial(service.name))
		conn.WriteString("quit", time.Second)
		conn.ReadString(len(uuid.New().String()), time.Second) // ignore uuid
		conn.ReadExpected("ok", time.Second)
	}
}

type serverFirstRotatingService struct {
	maxRequests uint32
	requests    uint32
	closeCB     func()
	closing     concurrenz.AtomicBoolean
}

func (service *serverFirstRotatingService) Handle(conn *testServerConn) error {
	requests := atomic.AddUint32(&service.requests, 1)
	doClose := requests >= service.maxRequests
	logger := pfxlog.Logger()

	name := uuid.New().String()
	expected := "hello, " + name
	conn.WriteString(name, time.Minute)
	read, eof := conn.ReadString(len(expected), time.Second)
	if eof {
		return errors.New("unexpected EOF")
	}
	if read == "quit" {
		logger.Infof("%v-%v: received '%v' from client", conn.server.idx, conn.id, name)
		if err := conn.server.listener.UpdatePrecedence(edge.PrecedenceFailed); err != nil {
			logger.WithError(err).Error("failed to update precedence")
			return err
		}
		conn.WriteString("ok", time.Second)
		return conn.server.close()
	}

	if read != expected {
		return errors.Errorf("unexpected result. expected %v, got %v", expected, read)
	}

	conn.RequireClose()

	atomic.AddUint32(&conn.server.msgCount, 1)
	if doClose && service.closing.CompareAndSwap(false, true) {
		logger.Infof("%v-%v: preparing to exit after serving %v requests. setting precedence to failed",
			conn.server.idx, conn.id, atomic.LoadUint32(&service.requests))
		if err := conn.server.listener.UpdatePrecedence(edge.PrecedenceFailed); err != nil {
			logger.WithError(err).Error("failed to update precedence")
			return err
		}
		service.closeCB()
		logger.Debugf("%v-%v: sleeping", conn.server.idx, conn.id)
		time.Sleep(time.Second)
		logger.Debugf("%v-%v: exiting", conn.server.idx, conn.id)
		return conn.server.close()
	}
	return nil
}
