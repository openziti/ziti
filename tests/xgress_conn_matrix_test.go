//go:build dataflow

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
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openziti/sdk-golang/v2/xgress"
	"github.com/openziti/sdk-golang/v2/ziti"
	"github.com/openziti/sdk-golang/v2/ziti/edge"
	routerEnv "github.com/openziti/ziti/v2/router/env"
	"github.com/openziti/ziti/v2/router/xgress_sdk"
)

// Test_XgressConnMatrix pairs every xgress conn implementation that can initiate a circuit with
// every one that can terminate it, and runs the close and half-close conversations across each
// pair. Each end of a circuit has its own conn type bridging the local stream to xgress, and a
// close signal only arrives correctly when both the sender's and receiver's conn types agree on
// how it is carried. Most of those pairs are never exercised by the single-topology data flow
// tests, which is how a half-close relay went missing for years on the tunneler and proxy
// initiators while the SDK-to-SDK path stayed green.
//
// Initiators: the SDK legacy conn over ConnectV1, SDK flow-control conns over ConnectV1 and
// ConnectV2 (ConnectV2 is always flow-controlled), the tunneler proxy intercept, the fabric
// proxy listener, and the router-embedded SDK.
// Terminators: SDK legacy and SDK flow-control listeners, the tunneler host, and the transport
// and edge_transport terminator bindings.
//
// Every pair gets its own controller and a single tunneler-enabled edge router that also runs
// the proxy listener, so the pairs run sequentially and each takes a few seconds.
func Test_XgressConnMatrix(t *testing.T) {
	for _, host := range xgConnHostKinds {
		for _, client := range xgConnClientKinds {
			t.Run(client.name+"->"+host.name, func(t *testing.T) {
				runXgConnPair(t, client, host)
			})
		}
	}
}

// xgConnClientKind is one way of initiating a circuit. e2e reports whether the initiator
// performs the public-key exchange for end-to-end encryption; a pair only enables encryption on
// the service when both sides do.
type xgConnClientKind struct {
	name string
	e2e  bool
	dial func(m *xgConnMatrix) net.Conn
}

// xgConnHostKind is one way of terminating a circuit. serviceConfigs runs before the service
// and router exist and returns config ids to attach to the service; start runs after the router
// is up and returns the acceptor for the hosted side.
type xgConnHostKind struct {
	name           string
	e2e            bool
	serviceConfigs func(m *xgConnMatrix) []string
	start          func(m *xgConnMatrix) xgConnAcceptor
}

type xgConnAcceptor interface {
	Accept() (net.Conn, error)
	Close()
}

var xgConnClientKinds = []xgConnClientKind{
	{name: "sdk-legacy-v1", e2e: true, dial: func(m *xgConnMatrix) net.Conn {
		return m.sdkDial(true, false)
	}},
	{name: "sdk-xgress-v1", e2e: true, dial: func(m *xgConnMatrix) net.Conn {
		return m.sdkDial(true, true)
	}},
	{name: "sdk-xgress-v2", e2e: true, dial: func(m *xgConnMatrix) net.Conn {
		return m.sdkDial(false, true)
	}},
	{name: "tunnel-intercept", e2e: true, dial: func(m *xgConnMatrix) net.Conn {
		return m.tcpDial(m.interceptPort)
	}},
	{name: "proxy", e2e: false, dial: func(m *xgConnMatrix) net.Conn {
		return m.tcpDial(m.proxyPort)
	}},
	{name: "embedded-sdk", e2e: true, dial: func(m *xgConnMatrix) net.Conn {
		return m.embeddedSdkDial()
	}},
}

var xgConnHostKinds = []xgConnHostKind{
	{name: "sdk-legacy", e2e: true, start: func(m *xgConnMatrix) xgConnAcceptor {
		return m.sdkListen(false)
	}},
	{name: "sdk-xgress", e2e: true, start: func(m *xgConnMatrix) xgConnAcceptor {
		return m.sdkListen(true)
	}},
	{name: "tunnel-host", e2e: true,
		serviceConfigs: func(m *xgConnMatrix) []string {
			l := m.tcpListen()
			cfg := m.ctx.newConfig("NH5p4FpGR", map[string]interface{}{
				"address":  "127.0.0.1",
				"port":     l.Addr().(*net.TCPAddr).Port,
				"protocol": "tcp",
			})
			m.ctx.AdminManagementSession.requireCreateEntity(cfg)
			m.tunnelHostListener = l
			return []string{cfg.Id}
		},
		start: func(m *xgConnMatrix) xgConnAcceptor {
			return &netListenerAcceptor{listener: m.tunnelHostListener}
		},
	},
	{name: "transport", e2e: false, start: func(m *xgConnMatrix) xgConnAcceptor {
		return m.terminatorListen("transport")
	}},
	{name: "edge-transport", e2e: true, start: func(m *xgConnMatrix) xgConnAcceptor {
		return m.terminatorListen("edge_transport")
	}},
}

// xgConnScenario is one close conversation between the two ends of an established circuit.
type xgConnScenario struct {
	name string
	run  func(t *testing.T, client, host net.Conn)
}

var xgConnScenarios = []xgConnScenario{
	{name: "host-half-close", run: func(t *testing.T, client, host net.Conn) {
		xgConnHalfCloseConversation(t, host, client, "host", "client")
	}},
	{name: "client-half-close", run: func(t *testing.T, client, host net.Conn) {
		xgConnHalfCloseConversation(t, client, host, "client", "host")
	}},
	{name: "host-close", run: func(t *testing.T, client, host net.Conn) {
		xgConnCloseConversation(t, host, client, "host", "client")
	}},
	{name: "client-close", run: func(t *testing.T, client, host net.Conn) {
		xgConnCloseConversation(t, client, host, "client", "host")
	}},
}

const xgConnTimeout = 5 * time.Second

// xgConnHalfCloseConversation has first write and half-close, second read to EOF while its
// own write side stays open, then second write and close, and first read to EOF. The
// second side's write after seeing EOF is the point: a half-close must not tear down the
// other direction.
func xgConnHalfCloseConversation(t *testing.T, first, second net.Conn, firstName, secondName string) {
	xgConnWrite(t, first, firstName, "from "+firstName)
	xgConnCloseWrite(t, first, firstName)

	xgConnExpectEOF(t, second, secondName, "from "+firstName)

	xgConnWrite(t, second, secondName, "from "+secondName)
	xgConnClose(t, second, secondName)

	xgConnExpectEOF(t, first, firstName, "from "+secondName)
}

// xgConnCloseConversation has first write then fully close, and second read the data to EOF.
func xgConnCloseConversation(t *testing.T, first, second net.Conn, firstName, secondName string) {
	xgConnWrite(t, first, firstName, "from "+firstName)
	xgConnClose(t, first, firstName)

	xgConnExpectEOF(t, second, secondName, "from "+firstName)
	xgConnClose(t, second, secondName)
}

func xgConnWrite(t *testing.T, conn net.Conn, name, data string) {
	if err := conn.SetWriteDeadline(time.Now().Add(xgConnTimeout)); err != nil {
		t.Fatalf("%s: set write deadline: %v", name, err)
	}
	defer func() { _ = conn.SetWriteDeadline(time.Time{}) }()
	if _, err := conn.Write([]byte(data)); err != nil {
		t.Fatalf("%s: write failed: %v", name, err)
	}
}

func xgConnCloseWrite(t *testing.T, conn net.Conn, name string) {
	cw, ok := conn.(edge.CloseWriter)
	if !ok {
		t.Fatalf("%s: conn type %T does not support CloseWrite", name, conn)
	}
	if err := cw.CloseWrite(); err != nil {
		t.Fatalf("%s: close write failed: %v", name, err)
	}
}

func xgConnClose(t *testing.T, conn net.Conn, name string) {
	if err := conn.Close(); err != nil {
		t.Fatalf("%s: close failed: %v", name, err)
	}
}

// xgConnExpectEOF reads conn until EOF and requires exactly expected to have arrived. A
// deadline error means the peer's close was never relayed.
func xgConnExpectEOF(t *testing.T, conn net.Conn, name, expected string) {
	if err := conn.SetReadDeadline(time.Now().Add(xgConnTimeout)); err != nil {
		t.Fatalf("%s: set read deadline: %v", name, err)
	}
	defer func() { _ = conn.SetReadDeadline(time.Time{}) }()

	var buf bytes.Buffer
	tmp := make([]byte, 1024)
	for {
		n, err := conn.Read(tmp)
		buf.Write(tmp[:n])
		if err == nil {
			continue
		}
		if !errors.Is(err, io.EOF) {
			t.Fatalf("%s: expected EOF after %q, got %q and error %v", name, expected, buf.String(), err)
		}
		break
	}
	if buf.String() != expected {
		t.Fatalf("%s: expected %q before EOF, got %q", name, expected, buf.String())
	}
}

// xgConnMatrix holds the per-pair environment: one controller, one tunneler-enabled edge
// router running the edge, tunnel proxy-intercept and fabric proxy listeners, and one service.
type xgConnMatrix struct {
	t       *testing.T
	ctx     *TestContext
	service *service

	interceptPort int
	proxyPort     int

	tunnelHostListener net.Listener

	sdkClientOnce sync.Once
	sdkClient     ziti.Context
	lastDial      *ziti.DialEvent

	embeddedOnce sync.Once
	embedded     xgress_sdk.Fabric
}

func runXgConnPair(t *testing.T, client xgConnClientKind, host xgConnHostKind) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#all"), s("#all"), nil)
	ctx.AdminManagementSession.requireNewServicePolicy("Bind", s("#all"), s("#all"), nil)
	ctx.AdminManagementSession.requireNewEdgeRouterPolicy(s("#all"), s("#all"))
	ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#all"))

	m := &xgConnMatrix{
		t:             t,
		ctx:           ctx,
		interceptPort: xgConnFreePort(t),
		proxyPort:     xgConnFreePort(t),
	}

	var configs []string
	if host.serviceConfigs != nil {
		configs = host.serviceConfigs(m)
	}

	m.service = ctx.newService(nil, configs)
	m.service.Name = "xg-matrix-" + strings.ReplaceAll(strings.ToLower(client.name+"-"+host.name), " ", "")
	m.service.encryptionRequired = client.e2e && host.e2e
	ctx.AdminManagementSession.requireCreateEntity(m.service)

	ctx.CreateEnrollAndStartTunnelerEdgeRouterWithCfgTweaks(m.tweakRouterConfig)

	acceptor := host.start(m)
	defer acceptor.Close()

	watcher := ctx.AdminManagementSession.newTerminatorWatcher(m.service.Id, 1)
	watcher.waitForTerminators(10 * time.Second)
	watcher.Close()

	for _, scenario := range xgConnScenarios {
		t.Run(scenario.name, func(t *testing.T) {
			m.t = t
			accepted := make(chan net.Conn, 1)
			acceptErr := make(chan error, 1)
			go func() {
				conn, err := acceptor.Accept()
				if err != nil {
					acceptErr <- err
					return
				}
				accepted <- conn
			}()

			clientConn := client.dial(m)
			defer func() { _ = clientConn.Close() }()

			var hostConn net.Conn
			select {
			case hostConn = <-accepted:
			case err := <-acceptErr:
				t.Fatalf("host accept failed: %v", err)
			case <-time.After(xgConnTimeout):
				t.Fatalf("host did not accept a connection")
			}
			defer func() { _ = hostConn.Close() }()

			m.checkSdkConnMode(client.name, clientConn)
			m.checkSdkConnMode(host.name, hostConn)

			scenario.run(t, clientConn, hostConn)
		})
	}
}

// tweakRouterConfig points the tunnel proxy-mode intercept at the pair's service and adds a
// fabric proxy listener for it. Both bindings resolve the service at startup, so the service
// must already exist.
func (m *xgConnMatrix) tweakRouterConfig(cfg *routerEnv.Config) {
	for _, listener := range cfg.Listeners {
		if listener.Name != "tunnel" {
			continue
		}
		options, ok := listener.Options["options"].(map[interface{}]interface{})
		if !ok {
			m.t.Fatalf("tunnel listener has no options map")
		}
		options["services"] = []interface{}{fmt.Sprintf("%s:%d", m.service.Name, m.interceptPort)}
	}

	cfg.Listeners = append(cfg.Listeners, routerEnv.ListenerBinding{
		Name: "proxy",
		Options: map[interface{}]interface{}{
			"binding": "proxy",
			"address": fmt.Sprintf("tcp:127.0.0.1:%d", m.proxyPort),
			"service": m.service.Id,
		},
	})
}

func (m *xgConnMatrix) router() *EdgeRouterHelper {
	if len(m.ctx.routers) == 0 {
		m.t.Fatalf("no router started")
	}
	return &EdgeRouterHelper{Router: m.ctx.routers[len(m.ctx.routers)-1]}
}

// checkSdkConnMode requires an SDK conn to be in the mode its kind name claims, so a pair
// that silently negotiated a different mode fails instead of testing the wrong seam.
func (m *xgConnMatrix) checkSdkConnMode(kindName string, conn net.Conn) {
	if !strings.HasPrefix(kindName, "sdk-") {
		return
	}
	wantXgress := strings.Contains(kindName, "xgress")
	typeName := reflect.TypeOf(conn).String()
	isXgress := strings.Contains(typeName, "Xgress")
	if isXgress != wantXgress {
		m.t.Fatalf("%s: expected sdk conn xgress mode %v, got conn type %s", kindName, wantXgress, typeName)
	}
}

func (m *xgConnMatrix) sdkDial(forceV1, sdkFlowControl bool) net.Conn {
	m.sdkClientOnce.Do(func() {
		_, m.sdkClient = m.ctx.AdminManagementSession.RequireCreateSdkContext()
		m.sdkClient.Events().AddDialListener(func(_ ziti.Context, evt ziti.DialEvent) {
			evtCopy := evt
			m.lastDial = &evtCopy
		})
	})

	m.lastDial = nil
	options := &ziti.DialOptions{
		ConnectTimeout: xgConnTimeout,
		ForceConnectV1: &forceV1,
		SdkFlowControl: &sdkFlowControl,
	}
	conn, err := m.sdkClient.DialWithOptions(m.service.Name, options)
	if err != nil {
		m.t.Fatalf("sdk dial failed: %v", err)
	}

	wantProtocol := edge.DialProtocolConnectV2
	if forceV1 {
		wantProtocol = edge.DialProtocolConnectV1
	}
	if m.lastDial == nil {
		m.t.Fatalf("no dial event emitted")
	}
	if m.lastDial.Protocol != wantProtocol {
		m.t.Fatalf("expected dial protocol %v, got %v", wantProtocol, m.lastDial.Protocol)
	}
	return conn
}

func (m *xgConnMatrix) sdkListen(sdkFlowControl bool) xgConnAcceptor {
	_, hostCtx := m.ctx.AdminManagementSession.RequireCreateSdkContext()
	options := ziti.DefaultListenOptions()
	options.SdkFlowControl = &sdkFlowControl
	listener, err := hostCtx.ListenWithOptions(m.service.Name, options)
	if err != nil {
		m.t.Fatalf("sdk listen failed: %v", err)
	}
	return &sdkListenerAcceptor{listener: listener, hostCtx: hostCtx}
}

// terminatorListen starts a local TCP server and points a terminator with the given binding
// at it, so the router dials it directly when a circuit is created.
func (m *xgConnMatrix) terminatorListen(binding string) xgConnAcceptor {
	l := m.tcpListen()
	routerId := m.ctx.edgeRouterEntity.id
	m.ctx.AdminManagementSession.requireNewTerminator(m.service.Id, routerId, binding, "tcp:"+l.Addr().String())
	return &netListenerAcceptor{listener: l}
}

func (m *xgConnMatrix) tcpListen() net.Listener {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		m.t.Fatalf("listen failed: %v", err)
	}
	return l
}

func (m *xgConnMatrix) tcpDial(port int) net.Conn {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), xgConnTimeout)
	if err != nil {
		m.t.Fatalf("tcp dial to port %d failed: %v", port, err)
	}
	return conn
}

// embeddedSdkDial drives the router-embedded SDK. The fabric wants a net.Conn to bridge, so
// a local TCP pair stands in for the application: the accepted side is handed to the fabric
// and the dialing side is what the test reads and writes.
func (m *xgConnMatrix) embeddedSdkDial() net.Conn {
	m.embeddedOnce.Do(func() {
		fabric, err := xgress_sdk.NewFabric(m.router(), xgress.DefaultOptions())
		if err != nil {
			m.t.Fatalf("embedded sdk fabric creation failed: %v", err)
		}
		m.embedded = fabric
	})

	l := m.tcpListen()
	defer func() { _ = l.Close() }()

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := l.Accept()
		if err == nil {
			accepted <- conn
		}
	}()

	appConn := m.tcpDial(l.Addr().(*net.TCPAddr).Port)
	var fabricConn net.Conn
	select {
	case fabricConn = <-accepted:
	case <-time.After(xgConnTimeout):
		m.t.Fatalf("local accept for embedded sdk failed")
	}

	options := &ziti.DialOptions{ConnectTimeout: xgConnTimeout}

	// the fabric learns its services from the router data model asynchronously
	deadline := time.Now().Add(10 * time.Second)
	for {
		err := m.embedded.TunnelWithOptions(m.service.Name, options, fabricConn, true)
		if err == nil {
			return appConn
		}
		if !strings.Contains(err.Error(), "not found") || time.Now().After(deadline) {
			m.t.Fatalf("embedded sdk tunnel failed: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func xgConnFreePort(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not allocate port: %v", err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

type netListenerAcceptor struct {
	listener net.Listener
}

func (self *netListenerAcceptor) Accept() (net.Conn, error) {
	return self.listener.Accept()
}

func (self *netListenerAcceptor) Close() {
	_ = self.listener.Close()
}

type sdkListenerAcceptor struct {
	listener edge.Listener
	hostCtx  ziti.Context
}

func (self *sdkListenerAcceptor) Accept() (net.Conn, error) {
	return self.listener.AcceptEdge()
}

func (self *sdkListenerAcceptor) Close() {
	_ = self.listener.Close()
	self.hostCtx.Close()
}
