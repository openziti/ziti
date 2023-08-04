// +build slowtests

package health

import (
	"context"
	"fmt"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/stretchr/testify/require"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

type svcChangeEvent struct {
	cost       uint16
	precedence edge.Precedence
}

func newEventTracker(t *testing.T) *eventTracker {
	return &eventTracker{
		Assertions:  require.New(t),
		events:      make(chan *svcChangeEvent, 16),
		healthSends: make(chan bool, 16),
	}
}

type eventTracker struct {
	*require.Assertions
	events      chan *svcChangeEvent
	healthSends chan bool
}

func (self *eventTracker) SendHealthEvent(pass bool) error {
	self.healthSends <- pass
	return nil
}

func (self *eventTracker) UpdateCostAndPrecedence(cost uint16, precedence edge.Precedence) error {
	fmt.Printf("event cost=%v, precedence=%v\n", cost, precedence)
	self.events <- &svcChangeEvent{
		cost:       cost,
		precedence: precedence,
	}

	return nil
}

func (self *eventTracker) assertEvent(t time.Duration, cost uint16, p edge.Precedence) {
	time.Sleep(t)

	fmt.Printf("after %v expecting cost=%v, prec=%v\n", t, cost, p)

	var evt *svcChangeEvent
	select {
	case evt = <-self.events:
	case <-time.After(100 * time.Millisecond):
		self.Fail("no event found")
		return
	}

	self.Equal(int(cost), int(evt.cost))
	self.Equal(int(p), int(evt.precedence))
}

func (self *eventTracker) assertHealthSend(t time.Duration, checkPassed bool) {
	time.Sleep(t)

	var send bool
	select {
	case send = <-self.healthSends:
	case <-time.After(100 * time.Millisecond):
		self.Fail("no event found")
		return
	}

	self.Equal(checkPassed, send)
}

func (self *eventTracker) assertNoEvent(t time.Duration) {
	select {
	case <-self.events:
		self.Fail("no events should be found")
	case <-time.After(t):
	}
}

func (self *eventTracker) assertNoSend(t time.Duration) {
	select {
	case <-self.healthSends:
		self.Fail("no health send should be found")
	case <-time.After(t):
	}
}

func Test_ManagerWithEventCounts(t *testing.T) {
	mgr := NewManager()
	defer mgr.Shutdown()

	req := require.New(t)

	et := newEventTracker(t)

	state := NewServiceState("my-service", ziti.PrecedenceRequired, 100, et)

	failEvents := uint16(3)
	passEvents := uint16(2)

	pingDef := &PortCheckDefinition{
		BaseCheckDefinition: BaseCheckDefinition{
			Interval: time.Second,
			Actions: []*ActionDefinition{
				{
					Trigger:           "fail",
					ConsecutiveEvents: &failEvents,
					Action:            "mark unhealthy",
				},
				{
					Trigger:           "pass",
					ConsecutiveEvents: &passEvents,
					Action:            "mark healthy",
				},
				{
					Trigger: "pass",
					Action:  "decrease cost 20",
				},
				{
					Trigger: "fail",
					Action:  "increase cost 10",
				},
			},
			Timeout: 100 * time.Millisecond,
		},
		Address: "localhost:9876",
	}

	err := mgr.RegisterServiceChecks(state, []CheckDefinition{pingDef})
	req.NoError(err)

	et.assertEvent(100*time.Millisecond, 110, edge.PrecedenceRequired)
	et.assertEvent(time.Second, 120, edge.PrecedenceRequired)
	et.assertEvent(time.Second, 130, edge.PrecedenceFailed)

	listener, err := net.Listen("tcp", "localhost:9876")
	req.NoError(err)

	go func() {
		for {
			c, err := listener.Accept()
			if err != nil {
				fmt.Printf("closing listener: %v\n", err)
				return
			}
			_ = c.Close()
		}
	}()

	et.assertEvent(time.Second, 110, edge.PrecedenceFailed)
	et.assertEvent(time.Second, 100, edge.PrecedenceRequired)

	// no new events should be generated until status changes as we're at min cost and same precedence
	select {
	case <-et.events:
		req.Fail("no events should be found")
	case <-time.After(2 * time.Second):
	}

	err = listener.Close()
	req.NoError(err)

	et.assertEvent(time.Second, 110, edge.PrecedenceRequired)
	et.assertEvent(time.Second, 120, edge.PrecedenceRequired)
	et.assertEvent(time.Second, 130, edge.PrecedenceFailed)
}

func Test_ManagerWithDurations(t *testing.T) {
	mgr := NewManager()
	defer mgr.Shutdown()

	req := require.New(t)

	et := newEventTracker(t)

	state := NewServiceState("my-service", ziti.PrecedenceDefault, 0, et)

	failEvents := 3 * time.Second
	passEvents := 1800 * time.Millisecond

	pingDef := &PortCheckDefinition{
		BaseCheckDefinition: BaseCheckDefinition{
			Interval: time.Second,
			Actions: []*ActionDefinition{
				{
					Trigger:  "fail",
					Duration: &failEvents,
					Action:   "mark unhealthy",
				},
				{
					Trigger:  "pass",
					Duration: &passEvents,
					Action:   "mark healthy",
				},
				{
					Trigger: "pass",
					Action:  "decrease cost 20",
				},
				{
					Trigger: "fail",
					Action:  "increase cost 10",
				},
			},
			Timeout: 100 * time.Millisecond,
		},
		Address: "localhost:9876",
	}

	err := mgr.RegisterServiceChecks(state, []CheckDefinition{pingDef})
	req.NoError(err)

	et.assertEvent(500*time.Millisecond, 10, edge.PrecedenceDefault)
	et.assertEvent(time.Second, 20, edge.PrecedenceDefault)
	et.assertEvent(time.Second, 30, edge.PrecedenceDefault)
	et.assertEvent(time.Second, 40, edge.PrecedenceFailed)

	now := time.Now()
	listener, err := net.Listen("tcp", "localhost:9876")
	req.NoError(err)

	go func() {
		for {
			c, err := listener.Accept()
			if err != nil {
				fmt.Printf("closing listener: %v\n", err)
				return
			}
			_ = c.Close()
		}
	}()

	waitFor := time.Second - time.Now().Sub(now)
	et.assertEvent(waitFor, 20, edge.PrecedenceFailed)
	et.assertEvent(time.Second, 0, edge.PrecedenceFailed)
	et.assertEvent(time.Second, 0, edge.PrecedenceDefault)

	// no new events should be generated until status changes as we're at min cost and same precedence
	et.assertNoEvent(2 * time.Second)

	now = time.Now()
	err = listener.Close()
	req.NoError(err)

	waitFor = time.Second - time.Now().Sub(now)
	et.assertEvent(waitFor, 10, edge.PrecedenceDefault)
	et.assertEvent(time.Second, 20, edge.PrecedenceDefault)
	et.assertEvent(time.Second, 30, edge.PrecedenceDefault)
	et.assertEvent(time.Second, 40, edge.PrecedenceFailed)
}

func newHttpCheck() *HttpCheckDefinition {
	return &HttpCheckDefinition{
		BaseCheckDefinition: BaseCheckDefinition{
			Interval: time.Second,
			Actions: []*ActionDefinition{
				{Trigger: "fail", Action: "mark unhealthy"},
				{Trigger: "pass", Action: "mark healthy"},
				{Trigger: "pass", Action: "increase cost 10"},
			},
			Timeout: 100 * time.Millisecond,
		},
		Url: "http://localhost:9876",
	}
}

func runHttpCheck(req *require.Assertions, f func(http.ResponseWriter, *http.Request)) func() {
	listener, err := net.Listen("tcp", "localhost:9876")
	req.NoError(err)
	var handler http.HandlerFunc = f
	s := http.Server{Handler: handler}
	go func() { _ = s.Serve(listener) }()

	now := time.Now()
	endTime := now.Add(2 * time.Second)
	maxWait := 2 * time.Second
	for {
		conn, err := net.DialTimeout("tcp", "localhost:9876", maxWait)
		if err == nil {
			_ = conn.Close()
			break
		}
		now = time.Now()
		if !now.Before(endTime) {
			_ = listener.Close()
			req.NoError(err)
		}
		maxWait = endTime.Sub(now)
		time.Sleep(10 * time.Millisecond)
	}

	return func() {
		_ = s.Shutdown(context.Background())
		_ = listener.Close()
	}
}

func Test_ManagerWithSimpleHttp(t *testing.T) {
	mgr := NewManager()
	defer mgr.Shutdown()

	req := require.New(t)
	et := newEventTracker(t)
	state := NewServiceState("my-service", ziti.PrecedenceDefault, 0, et)

	healthCheckDef := newHttpCheck()

	err := mgr.RegisterServiceChecks(state, []CheckDefinition{healthCheckDef})
	req.NoError(err)
	et.assertEvent(100*time.Millisecond, 0, edge.PrecedenceFailed)

	mgr.UnregisterServiceChecks("my-service")

	state = NewServiceState("my-service", ziti.PrecedenceDefault, 0, et)
	closeF := runHttpCheck(req, func(resp http.ResponseWriter, req *http.Request) {
		resp.WriteHeader(200)
	})
	defer closeF()

	err = mgr.RegisterServiceChecks(state, []CheckDefinition{healthCheckDef})
	req.NoError(err)
	et.assertEvent(100*time.Millisecond, 10, edge.PrecedenceDefault)
}

func Test_ManagerWithHttpStatusCode(t *testing.T) {
	mgr := NewManager()
	defer mgr.Shutdown()

	req := require.New(t)
	et := newEventTracker(t)
	state := NewServiceState("my-service", ziti.PrecedenceDefault, 0, et)

	healthCheckDef := newHttpCheck()
	healthCheckDef.ExpectStatus = 201

	closeF := runHttpCheck(req, func(resp http.ResponseWriter, req *http.Request) {
		fmt.Println("first")
		resp.WriteHeader(200)
	})
	defer closeF()

	err := mgr.RegisterServiceChecks(state, []CheckDefinition{healthCheckDef})
	req.NoError(err)
	et.assertEvent(100*time.Millisecond, 0, edge.PrecedenceFailed)

	closeF()
	mgr.UnregisterServiceChecks("my-service")

	state = NewServiceState("my-service", ziti.PrecedenceDefault, 0, et)
	closeF = runHttpCheck(req, func(resp http.ResponseWriter, req *http.Request) {
		fmt.Println("second")
		resp.WriteHeader(201)
	})
	defer closeF()

	err = mgr.RegisterServiceChecks(state, []CheckDefinition{healthCheckDef})
	req.NoError(err)
	et.assertEvent(100*time.Millisecond, 10, edge.PrecedenceDefault)
}

func Test_ManagerWithHttpExpectBody(t *testing.T) {
	mgr := NewManager()
	defer mgr.Shutdown()

	req := require.New(t)
	et := newEventTracker(t)
	state := NewServiceState("my-service", ziti.PrecedenceDefault, 0, et)

	healthCheckDef := newHttpCheck()
	healthCheckDef.ExpectStatus = 201
	healthCheckDef.ExpectBody = "this better be here"

	closeF := runHttpCheck(req, func(resp http.ResponseWriter, req *http.Request) {
		fmt.Println("first")
		resp.WriteHeader(201)
	})
	defer closeF()

	err := mgr.RegisterServiceChecks(state, []CheckDefinition{healthCheckDef})
	req.NoError(err)
	et.assertEvent(100*time.Millisecond, 0, edge.PrecedenceFailed)

	closeF()
	mgr.UnregisterServiceChecks("my-service")

	state = NewServiceState("my-service", ziti.PrecedenceDefault, 0, et)
	closeF = runHttpCheck(req, func(resp http.ResponseWriter, req *http.Request) {
		fmt.Println("second")
		resp.WriteHeader(201)
		_, _ = resp.Write([]byte("ok, this better be here, or else!"))
	})
	defer closeF()

	err = mgr.RegisterServiceChecks(state, []CheckDefinition{healthCheckDef})
	req.NoError(err)
	et.assertEvent(100*time.Millisecond, 10, edge.PrecedenceDefault)
}

func Test_ManagerWithHttpMethod(t *testing.T) {
	mgr := NewManager()
	defer mgr.Shutdown()

	req := require.New(t)
	et := newEventTracker(t)
	state := NewServiceState("my-service", ziti.PrecedenceDefault, 0, et)

	healthCheckDef := newHttpCheck()
	healthCheckDef.ExpectStatus = 201
	healthCheckDef.ExpectBody = "this better be here"
	healthCheckDef.Method = "POST"

	closeF := runHttpCheck(req, func(resp http.ResponseWriter, req *http.Request) {
		fmt.Println("first")
		if req.Method == "POST" {
			resp.WriteHeader(400)
		} else {
			resp.WriteHeader(201)
			_, _ = resp.Write([]byte("ok, this better be here, or else!"))
		}
	})
	defer closeF()

	err := mgr.RegisterServiceChecks(state, []CheckDefinition{healthCheckDef})
	req.NoError(err)
	et.assertEvent(100*time.Millisecond, 0, edge.PrecedenceFailed)

	closeF()
	mgr.UnregisterServiceChecks("my-service")

	state = NewServiceState("my-service", ziti.PrecedenceDefault, 0, et)
	closeF = runHttpCheck(req, func(resp http.ResponseWriter, req *http.Request) {
		fmt.Println("first")
		if req.Method != "POST" {
			resp.WriteHeader(400)
		} else {
			resp.WriteHeader(201)
			_, _ = resp.Write([]byte("ok, this better be here, or else!"))
		}
	})
	defer closeF()

	err = mgr.RegisterServiceChecks(state, []CheckDefinition{healthCheckDef})
	req.NoError(err)
	et.assertEvent(100*time.Millisecond, 10, edge.PrecedenceDefault)
}

func Test_ManagerWithHttpBody(t *testing.T) {
	mgr := NewManager()
	defer mgr.Shutdown()

	req := require.New(t)
	et := newEventTracker(t)
	state := NewServiceState("my-service", ziti.PrecedenceDefault, 0, et)

	healthCheckDef := newHttpCheck()
	healthCheckDef.ExpectStatus = 201
	healthCheckDef.ExpectBody = "this better be here"
	healthCheckDef.Method = "POST"
	healthCheckDef.Body = "hi!"

	closeF := runHttpCheck(req, func(resp http.ResponseWriter, req *http.Request) {
		fmt.Println("first")
		buf := &strings.Builder{}
		_, _ = io.Copy(buf, req.Body)
		body := buf.String()

		if strings.Contains(body, "hi!") {
			resp.WriteHeader(400)
			return
		}

		resp.WriteHeader(201)
		_, _ = resp.Write([]byte("ok, this better be here, or else!"))
	})
	defer closeF()

	err := mgr.RegisterServiceChecks(state, []CheckDefinition{healthCheckDef})
	req.NoError(err)
	et.assertEvent(100*time.Millisecond, 0, edge.PrecedenceFailed)

	closeF()
	mgr.UnregisterServiceChecks("my-service")

	state = NewServiceState("my-service", ziti.PrecedenceDefault, 0, et)
	closeF = runHttpCheck(req, func(resp http.ResponseWriter, req *http.Request) {
		fmt.Println("first")
		buf := &strings.Builder{}
		_, _ = io.Copy(buf, req.Body)
		body := buf.String()

		if !strings.Contains(body, "hi!") {
			resp.WriteHeader(400)
			return
		}

		if req.Method != "POST" {
			resp.WriteHeader(400)
			return
		}

		resp.WriteHeader(201)
		_, _ = resp.Write([]byte("ok, this better be here, or else!"))
	})
	defer closeF()

	err = mgr.RegisterServiceChecks(state, []CheckDefinition{healthCheckDef})
	req.NoError(err)
	et.assertEvent(100*time.Millisecond, 10, edge.PrecedenceDefault)
}

func Test_ChangeAndHealthSend(t *testing.T) {
	mgr := NewManager()
	defer mgr.Shutdown()

	req := require.New(t)
	et := newEventTracker(t)
	state := NewServiceState("my-service", ziti.PrecedenceDefault, 0, et)

	healthCheckDef := &HttpCheckDefinition{
		BaseCheckDefinition: BaseCheckDefinition{
			Interval: 100 * time.Millisecond,
			Actions: []*ActionDefinition{
				{Trigger: "change", Action: "send event"},
			},
			Timeout: 100 * time.Millisecond,
		},
		Url: "http://localhost:9876",
	}

	err := mgr.RegisterServiceChecks(state, []CheckDefinition{healthCheckDef})
	req.NoError(err)
	et.assertHealthSend(100*time.Millisecond, false)
	et.assertNoSend(200 * time.Millisecond)

	state = NewServiceState("my-service", ziti.PrecedenceDefault, 0, et)
	closeF := runHttpCheck(req, func(resp http.ResponseWriter, req *http.Request) {
		resp.WriteHeader(200)
	})
	defer closeF()

	et.assertHealthSend(500*time.Millisecond, true)
	et.assertNoSend(200 * time.Millisecond)

}
