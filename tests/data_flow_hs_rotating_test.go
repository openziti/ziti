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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/eid"
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/pkg/errors"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func Test_HSRotatingDataflow(t *testing.T) {
	t.Run("test client first smart routing", func(t *testing.T) {
		testClientFirstWithStrategy(t, "smartrouting")
	})

	t.Run("test client first random", func(t *testing.T) {
		testClientFirstWithStrategy(t, "random")
	})

	t.Run("test client first weighted", func(t *testing.T) {
		testClientFirstWithStrategy(t, "weighted")
	})

	t.Run("test server first smart routing", func(t *testing.T) {
		testServerFirstWithStrategy(t, "smartrouting")
	})

	t.Run("test server first random", func(t *testing.T) {
		testServerFirstWithStrategy(t, "random")
	})

	t.Run("test server first weighted", func(t *testing.T) {
		testServerFirstWithStrategy(t, "weighted")
	})
}

func testClientFirstWithStrategy(t *testing.T, strategy string) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminLogin()

	ctx.CreateEnrollAndStartEdgeRouter()

	service := ctx.AdminSession.RequireNewServiceAccessibleToAll(strategy)

	fmt.Printf("service id: %v\n", service.Id)

	serverContextC := make(chan ziti.Context, 3)
	doneC := make(chan struct{}, 1)
	var serverContexts []ziti.Context
	for i := 0; i < 3; i++ {
		_, context := ctx.AdminSession.RequireCreateSdkContext()
		serverContexts = append(serverContexts, context)
		serverContextC <- context
	}

	dialCount := int32(0)
	dials := make(chan struct{}, 1000)

	servers := make(chan *testServer, 1000)

	go func() {
		logger := pfxlog.Logger()
		for {
			select {
			case context := <-serverContextC:
				logger.Info("starting new listener")
				listener, err := context.Listen(service.Name)
				ctx.Req.NoError(err)
				service := &clientFirstRotatingService{
					maxRequests: uint32(rand.Intn(20) + 10),
					closeCB:     func() { serverContextC <- context },
				}
				server := newTestServer(listener, service.Handle)
				servers <- server
				server.start()
				logger.Infof("started new listener, servicing %v reads (dial capacity)", service.maxRequests)

				notifyFirst := &sync.Once{}
				listener.(edge.SessionListener).SetConnectionChangeHandler(func(conn []edge.Listener) {
					if len(conn) > 0 {
						notifyFirst.Do(func() {
							for i := 0; i < int(service.maxRequests); i++ {
								dials <- struct{}{}
							}
							count := atomic.AddInt32(&dialCount, int32(service.maxRequests))
							logger.Infof("added dial capacity: Available: %v", count)
						})
					}
				})
			case <-doneC:
				close(servers)
				return
			}
		}
	}()

	clientIdentity := ctx.AdminSession.RequireNewIdentityWithOtt(false)
	clientConfig := ctx.EnrollIdentity(clientIdentity.Id)
	clientContext := ziti.NewContextWithConfig(clientConfig)

	logger := pfxlog.Logger()

	ticker := time.NewTicker(time.Millisecond * 500)
	defer ticker.Stop()

	for i := 0; i < 250; i++ {
		select {
		case <-ticker.C:
			count := atomic.LoadInt32(&dialCount)
			logger.Infof("timer expired. Dial capacity: %v", count)
		case <-dials:
			count := atomic.AddInt32(&dialCount, -1)
			logger.Infof("consumed dial capacity. Available: %v", count)
		}

		conn := ctx.WrapConn(clientContext.Dial(service.Name))

		name := eid.New()
		conn.WriteString(name, time.Second)
		conn.ReadExpected("hello, "+name, time.Second)
		conn.RequireClose()
		logger.Infof("%v: done", i+1)
	}

	close(doneC)

	for server := range servers {
		ctx.Req.NoError(server.close())
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
	logger.Infof("handling request on session %v. requests: %v, max: %v",
		conn.server.listener.(edge.SessionListener).GetCurrentSession().Token, requests, service.maxRequests)
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
			logger.Infof("%v-%v: sleeping", conn.server.idx, conn.id)
			time.Sleep(100 * time.Millisecond)
			logger.Infof("%v-%v: exiting", conn.server.idx, conn.id)
			return conn.server.close()
		}
	}
}

func testServerFirstWithStrategy(t *testing.T, strategy string) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminLogin()

	ctx.CreateEnrollAndStartEdgeRouter()

	service := ctx.AdminSession.RequireNewServiceAccessibleToAll(strategy)
	fmt.Printf("service id: %v\n", service.Id)

	serverContextC := make(chan ziti.Context, 3)
	doneC := make(chan struct{}, 1)
	var serverContexts []ziti.Context

	var dialCount int32
	servers := make(chan *testServer, 1000)
	dials := make(chan struct{}, 1000)

	for i := 0; i < 3; i++ {
		_, context := ctx.AdminSession.RequireCreateSdkContext()
		serverContexts = append(serverContexts, context)
		serverContextC <- context
	}

	go func() {
		logger := pfxlog.Logger()
		for {
			select {
			case context := <-serverContextC:
				listener, err := context.Listen(service.Name)
				ctx.Req.NoError(err)
				service := &serverFirstRotatingService{
					maxRequests: uint32(rand.Intn(20) + 10),
					closeCB:     func() { serverContextC <- context },
				}
				server := newTestServer(listener, service.Handle)
				servers <- server
				server.start()
				logger.Infof("started new listener, servicing %v reads (dial capacity)", service.maxRequests)

				notifyFirst := &sync.Once{}
				listener.(edge.SessionListener).SetConnectionChangeHandler(func(conn []edge.Listener) {
					if len(conn) > 0 {
						notifyFirst.Do(func() {
							for i := 0; i < int(service.maxRequests); i++ {
								dials <- struct{}{}
							}
							count := atomic.AddInt32(&dialCount, int32(service.maxRequests))
							logger.Infof("added dial capacity: Available: %v", count)
						})
					}
				})
			case <-doneC:
				close(servers)
				return
			}
		}
	}()

	clientIdentity := ctx.AdminSession.RequireNewIdentityWithOtt(false)
	clientConfig := ctx.EnrollIdentity(clientIdentity.Id)
	clientContext := ziti.NewContextWithConfig(clientConfig)

	ticker := time.NewTicker(time.Millisecond * 500)
	defer ticker.Stop()

	logger := pfxlog.Logger()

	for i := 0; i < 250; i++ {
		select {
		case <-ticker.C:
			count := atomic.LoadInt32(&dialCount)
			logger.Infof("timer expired. Dial capacity: %v", count)
		case <-dials:
			count := atomic.AddInt32(&dialCount, -1)
			logger.Infof("consumed dial capacity. Available: %v", count)
		}

		conn := ctx.WrapConn(clientContext.Dial(service.Name))
		name := conn.ReadString(1024, time.Second)
		conn.WriteString("hello, "+name, time.Second)
		logger.Debugf("%v: done", i+1)
	}

	close(doneC)

	for server := range servers {
		ctx.Req.NoError(server.close())
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

	name := eid.New()
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
