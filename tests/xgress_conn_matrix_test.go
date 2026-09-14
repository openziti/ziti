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
	"math/rand/v2"
	"net"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	idlib "github.com/openziti/identity"
	"github.com/openziti/sdk-golang/v2/xgress"
	"github.com/openziti/sdk-golang/v2/ziti"
	"github.com/openziti/sdk-golang/v2/ziti/edge"
	"github.com/openziti/transport/v2"
	routerEnv "github.com/openziti/ziti/v2/router/env"
	"github.com/openziti/ziti/v2/router/xgress_sdk"
	"github.com/openziti/ziti/v2/router/xgress_transport"
)

// Test_XgressConnMatrix pairs every xgress conn implementation that can initiate a circuit with
// every one that can terminate it, and runs the close and half-close conversations across each
// pair. Each end of a circuit has its own conn type bridging the local stream to xgress, and a
// close signal only arrives correctly when both the sender's and receiver's conn types agree on
// how it is carried. Most of those pairs are never exercised by the single-topology data flow
// tests.
//
// Initiators: the SDK legacy conn over ConnectV1, SDK flow-control conns over ConnectV1 and
// ConnectV2 (ConnectV2 is always flow-controlled), the tunneler proxy intercept, the fabric
// proxy and transport listeners, and the router-embedded SDK.
// Terminators: SDK legacy and SDK flow-control listeners, the tunneler host, and the transport
// and edge_transport terminator bindings.
// Datagram bindings (proxy_udp, transport_udp, UDP tunneler services) have no half-close and
// are not covered.
//
// All pairs share one controller and one tunneler-enabled edge router, and each pair has its own
// service. The tunnel intercepts and proxy listeners bind their service when the router starts,
// so every pair's service is created first.
func Test_XgressConnMatrix(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#all"), s("#all"), nil)
	ctx.AdminManagementSession.requireNewServicePolicy("Bind", s("#all"), s("#all"), nil)
	ctx.AdminManagementSession.requireNewEdgeRouterPolicy(s("#all"), s("#all"))
	ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#all"))

	m := &xgConnMatrix{ctx: ctx}
	defer m.close()

	for _, host := range xgConnHostKinds {
		for _, client := range xgConnClientKinds {
			m.pairs = append(m.pairs, m.newPair(t, client, host))
		}
	}

	ctx.CreateEnrollAndStartTunnelerEdgeRouterWithCfgTweaks(func(cfg *routerEnv.Config) {
		m.configureRouter(t, cfg)
	})
	m.startClients(t)

	defer ctx.testContextChanged(t)
	for _, p := range m.pairs {
		t.Run(p.client.name+"->"+p.host.name, p.run)
	}
}

// xgConnClientKind is one way of initiating a circuit. e2e reports whether the initiator
// performs the public-key exchange for end-to-end encryption; a pair only enables encryption on
// the service when both sides do. configureRouter, when set, binds the pair's listenerPort to
// the pair's service in the router config before the router starts.
type xgConnClientKind struct {
	name            string
	e2e             bool
	configureRouter func(p *xgConnPair, cfg *routerEnv.Config)
	dial            func(p *xgConnPair) net.Conn
}

// xgConnHostKind is one way of terminating a circuit. serviceConfigs runs before the service
// and router exist and returns config ids to attach to the service; start runs after the router
// is up and returns the acceptor for the hosted side.
type xgConnHostKind struct {
	name           string
	e2e            bool
	serviceConfigs func(p *xgConnPair) []string
	start          func(p *xgConnPair) xgConnAcceptor
}

type xgConnAcceptor interface {
	Accept() (net.Conn, error)
	Close()
}

var xgConnClientKinds = []xgConnClientKind{
	{name: "sdk-legacy-v1", e2e: true, dial: func(p *xgConnPair) net.Conn {
		return p.sdkDial(true, false)
	}},
	{name: "sdk-xgress-v1", e2e: true, dial: func(p *xgConnPair) net.Conn {
		return p.sdkDial(true, true)
	}},
	{name: "sdk-xgress-v2", e2e: true, dial: func(p *xgConnPair) net.Conn {
		return p.sdkDial(false, true)
	}},
	{name: "tunnel-intercept", e2e: true,
		configureRouter: func(p *xgConnPair, _ *routerEnv.Config) {
			p.m.intercepts = append(p.m.intercepts, fmt.Sprintf("%s:%d", p.service.Name, p.listenerPort))
		},
		dial: func(p *xgConnPair) net.Conn {
			return p.tcpDial(p.listenerPort)
		},
	},
	{name: "proxy", e2e: false,
		configureRouter: func(p *xgConnPair, cfg *routerEnv.Config) {
			cfg.Listeners = append(cfg.Listeners, routerEnv.ListenerBinding{
				Name: "proxy",
				Options: map[interface{}]interface{}{
					"binding": "proxy",
					"address": fmt.Sprintf("tcp:127.0.0.1:%d", p.listenerPort),
					"service": p.service.Id,
				},
			})
		},
		dial: func(p *xgConnPair) net.Conn {
			return p.tcpDial(p.listenerPort)
		},
	},
	{name: "transport", e2e: false, dial: func(p *xgConnPair) net.Conn {
		return p.transportDial()
	}},
	{name: "embedded-sdk", e2e: true, dial: func(p *xgConnPair) net.Conn {
		return p.embeddedSdkDial()
	}},
}

var xgConnHostKinds = []xgConnHostKind{
	{name: "sdk-legacy", e2e: true, start: func(p *xgConnPair) xgConnAcceptor {
		return p.sdkListen(false)
	}},
	{name: "sdk-xgress", e2e: true, start: func(p *xgConnPair) xgConnAcceptor {
		return p.sdkListen(true)
	}},
	{name: "tunnel-host", e2e: true,
		serviceConfigs: func(p *xgConnPair) []string {
			l := p.tcpListen()
			cfg := p.m.ctx.newConfig("NH5p4FpGR", map[string]interface{}{
				"address":  "127.0.0.1",
				"port":     l.Addr().(*net.TCPAddr).Port,
				"protocol": "tcp",
			})
			p.m.ctx.AdminManagementSession.requireCreateEntity(cfg)
			p.tunnelHostListener = l
			return []string{cfg.Id}
		},
		start: func(p *xgConnPair) xgConnAcceptor {
			return &netListenerAcceptor{listener: p.tunnelHostListener}
		},
	},
	{name: "transport", e2e: false, start: func(p *xgConnPair) xgConnAcceptor {
		return p.terminatorListen("transport")
	}},
	{name: "edge-transport", e2e: true, start: func(p *xgConnPair) xgConnAcceptor {
		return p.terminatorListen("edge_transport")
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

// xgConnMatrix is the environment every pair shares: the controller, one tunneler-enabled edge
// router running the edge, tunnel proxy-intercept, fabric proxy and transport listeners, an SDK
// context for dialing, one for hosting, and the router-embedded SDK.
type xgConnMatrix struct {
	ctx   *TestContext
	pairs []*xgConnPair

	transportPort int
	intercepts    []interface{}

	sdkClient ziti.Context
	sdkHost   ziti.Context
	lastDial  *ziti.DialEvent
	embedded  xgress_sdk.Fabric
}

// xgConnPair is one initiator kind paired with one terminator kind, on its own service.
// listenerPort is the router port bound to that service, for initiator kinds that need one.
type xgConnPair struct {
	m       *xgConnMatrix
	t       *testing.T
	client  xgConnClientKind
	host    xgConnHostKind
	service *service

	listenerPort       int
	tunnelHostListener net.Listener
}

func (m *xgConnMatrix) newPair(t *testing.T, client xgConnClientKind, host xgConnHostKind) *xgConnPair {
	p := &xgConnPair{m: m, t: t, client: client, host: host}

	var configs []string
	if host.serviceConfigs != nil {
		configs = host.serviceConfigs(p)
	}

	p.service = m.ctx.newService(nil, configs)
	p.service.Name = "xg-matrix-" + client.name + "-" + host.name
	p.service.encryptionRequired = client.e2e && host.e2e
	m.ctx.AdminManagementSession.requireCreateEntity(p.service)
	return p
}

// configureRouter gives each pair that needs one its own router listener, replaces the tunnel
// listener's intercepts with the pairs' intercepts, and adds the shared transport listener.
func (m *xgConnMatrix) configureRouter(t *testing.T, cfg *routerEnv.Config) {
	var bound []*xgConnPair
	for _, p := range m.pairs {
		if p.client.configureRouter != nil {
			bound = append(bound, p)
		}
	}

	ports := xgConnReservePorts(t, len(bound)+1)
	m.transportPort = ports[0]
	for i, p := range bound {
		p.listenerPort = ports[i+1]
		p.client.configureRouter(p, cfg)
	}

	for _, listener := range cfg.Listeners {
		if listener.Name != "tunnel" {
			continue
		}
		options, ok := listener.Options["options"].(map[interface{}]interface{})
		if !ok {
			t.Fatalf("tunnel listener has no options map")
		}
		options["services"] = m.intercepts
	}

	cfg.Listeners = append(cfg.Listeners, routerEnv.ListenerBinding{
		Name: "transport",
		Options: map[interface{}]interface{}{
			"binding": "transport",
			"address": fmt.Sprintf("tcp:127.0.0.1:%d", m.transportPort),
		},
	})
}

// startClients creates the SDK contexts and the embedded fabric. It runs after every service
// exists and the router is up.
func (m *xgConnMatrix) startClients(t *testing.T) {
	_, m.sdkClient = m.ctx.AdminManagementSession.RequireCreateSdkContext()
	m.sdkClient.Events().AddDialListener(func(_ ziti.Context, evt ziti.DialEvent) {
		evtCopy := evt
		m.lastDial = &evtCopy
	})

	_, m.sdkHost = m.ctx.AdminManagementSession.RequireCreateSdkContext()

	if len(m.ctx.routers) == 0 {
		t.Fatalf("no router started")
	}
	router := &EdgeRouterHelper{Router: m.ctx.routers[len(m.ctx.routers)-1]}
	fabric, err := xgress_sdk.NewFabric(router, xgress.DefaultOptions())
	if err != nil {
		t.Fatalf("embedded sdk fabric creation failed: %v", err)
	}
	m.embedded = fabric
}

// close releases what startClients and the tunnel-host pairs opened, including the listeners of
// pairs a -run filter skipped.
func (m *xgConnMatrix) close() {
	if m.sdkClient != nil {
		m.sdkClient.Close()
	}
	if m.sdkHost != nil {
		m.sdkHost.Close()
	}
	for _, p := range m.pairs {
		if p.tunnelHostListener != nil {
			_ = p.tunnelHostListener.Close()
		}
	}
}

// bind points the pair and the shared test context at t, so helper failures are reported by the
// running subtest.
func (p *xgConnPair) bind(t *testing.T) {
	p.t = t
	p.m.ctx.testContextChanged(t)
}

func (p *xgConnPair) run(t *testing.T) {
	p.bind(t)
	acceptor := p.host.start(p)
	defer acceptor.Close()

	watcher := p.m.ctx.AdminManagementSession.newTerminatorWatcher(p.service.Id, 1)
	watcher.waitForTerminators(10 * time.Second)
	watcher.Close()

	for _, scenario := range xgConnScenarios {
		t.Run(scenario.name, func(t *testing.T) {
			p.bind(t)
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

			clientConn := p.client.dial(p)
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

			p.checkSdkConnMode(p.client.name, clientConn)
			p.checkSdkConnMode(p.host.name, hostConn)

			scenario.run(t, clientConn, hostConn)
		})
	}
}

// checkSdkConnMode requires an SDK conn to be in the mode its kind name claims, so a pair
// that silently negotiated a different mode fails instead of testing the wrong seam.
func (p *xgConnPair) checkSdkConnMode(kindName string, conn net.Conn) {
	if !strings.HasPrefix(kindName, "sdk-") {
		return
	}
	wantXgress := strings.Contains(kindName, "xgress")
	typeName := reflect.TypeOf(conn).String()
	isXgress := strings.Contains(typeName, "Xgress")
	if isXgress != wantXgress {
		p.t.Fatalf("%s: expected sdk conn xgress mode %v, got conn type %s", kindName, wantXgress, typeName)
	}
}

func (p *xgConnPair) sdkDial(forceV1, sdkFlowControl bool) net.Conn {
	m := p.m
	m.lastDial = nil
	options := &ziti.DialOptions{
		ConnectTimeout: xgConnTimeout,
		ForceConnectV1: &forceV1,
		SdkFlowControl: &sdkFlowControl,
	}
	conn, err := m.sdkClient.DialWithOptions(p.service.Name, options)
	if err != nil {
		p.t.Fatalf("sdk dial failed: %v", err)
	}

	wantProtocol := edge.DialProtocolConnectV2
	if forceV1 {
		wantProtocol = edge.DialProtocolConnectV1
	}
	if m.lastDial == nil {
		p.t.Fatalf("no dial event emitted")
	}
	if m.lastDial.Protocol != wantProtocol {
		p.t.Fatalf("expected dial protocol %v, got %v", wantProtocol, m.lastDial.Protocol)
	}
	return conn
}

func (p *xgConnPair) sdkListen(sdkFlowControl bool) xgConnAcceptor {
	options := ziti.DefaultListenOptions()
	options.SdkFlowControl = &sdkFlowControl
	listener, err := p.m.sdkHost.ListenWithOptions(p.service.Name, options)
	if err != nil {
		p.t.Fatalf("sdk listen failed: %v", err)
	}
	return &sdkListenerAcceptor{listener: listener}
}

// terminatorListen starts a local TCP server and points a terminator with the given binding
// at it, so the router dials it directly when a circuit is created.
func (p *xgConnPair) terminatorListen(binding string) xgConnAcceptor {
	l := p.tcpListen()
	routerId := p.m.ctx.edgeRouterEntity.id
	p.m.ctx.AdminManagementSession.requireNewTerminator(p.service.Id, routerId, binding, "tcp:"+l.Addr().String())
	return &netListenerAcceptor{listener: l}
}

func (p *xgConnPair) tcpListen() net.Listener {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		p.t.Fatalf("listen failed: %v", err)
	}
	return l
}

func (p *xgConnPair) tcpDial(port int) net.Conn {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), xgConnTimeout)
	if err != nil {
		p.t.Fatalf("tcp dial to port %d failed: %v", port, err)
	}
	return conn
}

// transportDial creates a circuit through the fabric transport listener, which takes the
// service in a request line before the stream starts.
func (p *xgConnPair) transportDial() net.Conn {
	addr, err := transport.ParseAddress(fmt.Sprintf("tcp:127.0.0.1:%d", p.m.transportPort))
	if err != nil {
		p.t.Fatalf("invalid transport listener address: %v", err)
	}
	clientId := &idlib.TokenId{Token: "xg-matrix-client"}
	serviceId := &idlib.TokenId{Token: p.service.Id}
	conn, err := xgress_transport.ClientDial(addr, clientId, serviceId, nil)
	if err != nil {
		p.t.Fatalf("transport dial failed: %v", err)
	}
	return conn
}

// embeddedSdkDial drives the router-embedded SDK. The fabric wants a net.Conn to bridge, so
// a local TCP pair stands in for the application: the accepted side is handed to the fabric
// and the dialing side is what the test reads and writes.
func (p *xgConnPair) embeddedSdkDial() net.Conn {
	l := p.tcpListen()
	defer func() { _ = l.Close() }()

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := l.Accept()
		if err == nil {
			accepted <- conn
		}
	}()

	appConn := p.tcpDial(l.Addr().(*net.TCPAddr).Port)
	var fabricConn net.Conn
	select {
	case fabricConn = <-accepted:
	case <-time.After(xgConnTimeout):
		p.t.Fatalf("local accept for embedded sdk failed")
	}

	options := &ziti.DialOptions{ConnectTimeout: xgConnTimeout}

	// the fabric learns its services from the router data model asynchronously
	deadline := time.Now().Add(10 * time.Second)
	for {
		err := p.m.embedded.TunnelWithOptions(p.service.Name, options, fabricConn, true)
		if err == nil {
			return appConn
		}
		if !strings.Contains(err.Error(), "not found") || time.Now().After(deadline) {
			p.t.Fatalf("embedded sdk tunnel failed: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// xgConnReservePorts returns n distinct free ports below the default ephemeral ranges (32768 and
// up on Linux, 49152 and up on macOS and Windows). A port from a :0 listen is an ephemeral one,
// which an outbound connection opened while the router starts can take before the router's
// listener binds it.
func xgConnReservePorts(t *testing.T, n int) []int {
	var probes []net.Listener
	defer func() {
		for _, l := range probes {
			_ = l.Close()
		}
	}()

	var ports []int
	for port := 20000 + rand.IntN(10000); len(ports) < n && port < 32768; port++ {
		l, err := net.Listen("tcp4", "0.0.0.0:"+strconv.Itoa(port))
		if err != nil {
			continue
		}
		probes = append(probes, l)
		ports = append(ports, port)
	}
	if len(ports) < n {
		t.Fatalf("reserved only %d of %d ports", len(ports), n)
	}
	return ports
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
}

func (self *sdkListenerAcceptor) Accept() (net.Conn, error) {
	return self.listener.AcceptEdge()
}

func (self *sdkListenerAcceptor) Close() {
	_ = self.listener.Close()
}
