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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openziti/edge-api/rest_model"
	idlib "github.com/openziti/identity"
	"github.com/openziti/sdk-golang/xgress"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/common/eid"
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
// Initiators: the SDK legacy and SDK flow-control conns, the tunneler proxy intercept, the
// fabric proxy and transport listeners, and the router-embedded SDK.
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

	mgmt := ctx.NewEdgeManagementApi(nil)
	_, err := mgmt.Authenticate(ctx.NewAdminCredentials(), nil)
	ctx.Req.NoError(err)

	_, err = mgmt.CreateServicePolicy(rest_model.DialBindDial, s("#all"), s("#all"), nil)
	ctx.Req.NoError(err)
	_, err = mgmt.CreateServicePolicy(rest_model.DialBindBind, s("#all"), s("#all"), nil)
	ctx.Req.NoError(err)
	_, err = mgmt.CreateEdgeRouterPolicy(s("#all"), s("#all"))
	ctx.Req.NoError(err)
	_, err = mgmt.CreateServiceEdgeRouterPolicy(s("#all"), s("#all"))
	ctx.Req.NoError(err)

	m := &xgConnMatrix{ctx: ctx, mgmt: mgmt, namePrefix: "xg-matrix-" + eid.New() + "-"}
	defer m.close()

	for _, host := range xgConnHostKinds {
		for _, client := range xgConnClientKinds {
			p, err := m.newPair(client, host)
			ctx.Req.NoError(err, "creating the %s->%s pair", client.name, host.name)
			m.pairs = append(m.pairs, p)
		}
	}
	ctx.Req.NoError(m.reservePorts())

	router := ctx.CreateEnrollAndStartTunnelerEdgeRouterWithCfgTweaks(m.configureRouter)
	ctx.Req.NoError(m.routerConfigErr)

	_, m.sdkClient = ctx.AdminManagementSession.RequireCreateSdkContext()
	_, m.sdkHost = ctx.AdminManagementSession.RequireCreateSdkContext()
	ctx.Req.NoError(m.startEmbeddedSdk(router))

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
	dial            func(p *xgConnPair) (net.Conn, error)
}

// xgConnHostKind is one way of terminating a circuit. serviceConfigs runs before the service
// and router exist and returns config ids to attach to the service; start runs after the router
// is up and returns the acceptor for the hosted side.
type xgConnHostKind struct {
	name           string
	e2e            bool
	serviceConfigs func(p *xgConnPair) ([]string, error)
	start          func(p *xgConnPair) (xgConnAcceptor, error)
}

type xgConnAcceptor interface {
	Accept() (net.Conn, error)
	Close()
}

var xgConnClientKinds = []xgConnClientKind{
	{name: "sdk-legacy", e2e: true, dial: func(p *xgConnPair) (net.Conn, error) {
		return p.sdkDial(false)
	}},
	{name: "sdk-xgress", e2e: true, dial: func(p *xgConnPair) (net.Conn, error) {
		return p.sdkDial(true)
	}},
	{name: "tunnel-intercept", e2e: true,
		configureRouter: func(p *xgConnPair, _ *routerEnv.Config) {
			p.m.intercepts = append(p.m.intercepts, fmt.Sprintf("%s:%d", p.serviceName, p.listenerPort))
		},
		dial: func(p *xgConnPair) (net.Conn, error) {
			return net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(p.listenerPort), xgConnTimeout)
		},
	},
	{name: "proxy", e2e: false,
		configureRouter: func(p *xgConnPair, cfg *routerEnv.Config) {
			cfg.Listeners = append(cfg.Listeners, routerEnv.ListenerBinding{
				Name: "proxy",
				Options: map[interface{}]interface{}{
					"binding": "proxy",
					"address": fmt.Sprintf("tcp:127.0.0.1:%d", p.listenerPort),
					"service": p.serviceId,
				},
			})
		},
		dial: func(p *xgConnPair) (net.Conn, error) {
			return net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(p.listenerPort), xgConnTimeout)
		},
	},
	{name: "transport", e2e: false, dial: func(p *xgConnPair) (net.Conn, error) {
		return p.transportDial()
	}},
	{name: "embedded-sdk", e2e: true, dial: func(p *xgConnPair) (net.Conn, error) {
		return p.embeddedSdkDial()
	}},
}

var xgConnHostKinds = []xgConnHostKind{
	{name: "sdk-legacy", e2e: true, start: func(p *xgConnPair) (xgConnAcceptor, error) {
		return p.sdkListen(false)
	}},
	{name: "sdk-xgress", e2e: true, start: func(p *xgConnPair) (xgConnAcceptor, error) {
		return p.sdkListen(true)
	}},
	{name: "tunnel-host", e2e: true,
		serviceConfigs: func(p *xgConnPair) ([]string, error) {
			l, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				return nil, err
			}
			p.tunnelHostListener = l
			cfg, err := p.m.mgmt.CreateConfig("NH5p4FpGR" /* host.v1 */, map[string]interface{}{
				"address":  "127.0.0.1",
				"port":     l.Addr().(*net.TCPAddr).Port,
				"protocol": "tcp",
			})
			if err != nil {
				return nil, fmt.Errorf("create host.v1 config: %w", err)
			}
			return []string{cfg.ID}, nil
		},
		start: func(p *xgConnPair) (xgConnAcceptor, error) {
			return &netListenerAcceptor{listener: p.tunnelHostListener}, nil
		},
	},
	{name: "transport", e2e: false, start: func(p *xgConnPair) (xgConnAcceptor, error) {
		return p.terminatorListen("transport")
	}},
	{name: "edge-transport", e2e: true, start: func(p *xgConnPair) (xgConnAcceptor, error) {
		return p.terminatorListen("edge_transport")
	}},
}

// xgConnScenario is one close conversation between the two ends of an established circuit. The
// first side, the host when hostFirst is set and the client otherwise, writes and then
// half-closes if halfClose is set or closes if not, and the second side reads that to EOF. After
// a half-close the second side then writes and closes, and the first side reads that to EOF.
//
// With roundTripFirst, the first side writes, the second side reads that and replies, and the
// first side reads the reply before anything else, so the half-close follows data in both
// directions.
type xgConnScenario struct {
	name           string
	hostFirst      bool
	halfClose      bool
	roundTripFirst bool
}

var xgConnScenarios = []xgConnScenario{
	{name: "host-half-close", hostFirst: true, halfClose: true},
	{name: "client-half-close", halfClose: true},
	{name: "client-half-close-after-round-trip", halfClose: true, roundTripFirst: true},
	{name: "host-close", hostFirst: true},
	{name: "client-close"},
}

const xgConnTimeout = 5 * time.Second

// xgConnMatrix is the environment every pair shares: the controller, one tunneler-enabled edge
// router running the edge, tunnel proxy-intercept, fabric proxy and transport listeners, an SDK
// context for dialing, one for hosting, and the router-embedded SDK.
type xgConnMatrix struct {
	ctx        *TestContext
	mgmt       *ManagementHelperClient
	namePrefix string
	pairs      []*xgConnPair

	transportPort   int
	intercepts      []interface{}
	routerConfigErr error

	sdkClient ziti.Context
	sdkHost   ziti.Context
	embedded  xgress_sdk.Fabric
}

// xgConnPair is one initiator kind paired with one terminator kind, on its own service.
// listenerPort is the router port bound to that service, for initiator kinds that need one.
type xgConnPair struct {
	m           *xgConnMatrix
	client      xgConnClientKind
	host        xgConnHostKind
	serviceId   string
	serviceName string

	listenerPort       int
	tunnelHostListener net.Listener
}

// newPair creates the service for one initiator and terminator kind, along with any configs
// the terminator kind attaches to it.
func (m *xgConnMatrix) newPair(client xgConnClientKind, host xgConnHostKind) (*xgConnPair, error) {
	p := &xgConnPair{m: m, client: client, host: host}

	var configs []string
	if host.serviceConfigs != nil {
		var err error
		if configs, err = host.serviceConfigs(p); err != nil {
			return nil, err
		}
	}

	p.serviceName = m.namePrefix + client.name + "-" + host.name
	service, err := m.mgmt.CreateService(p.serviceName, client.e2e && host.e2e, configs)
	if err != nil {
		return nil, fmt.Errorf("create service %s: %w", p.serviceName, err)
	}
	p.serviceId = service.ID
	return p, nil
}

// reservePorts assigns a port to the shared transport listener and to each pair whose initiator
// kind binds a router listener.
func (m *xgConnMatrix) reservePorts() error {
	var bound []*xgConnPair
	for _, p := range m.pairs {
		if p.client.configureRouter != nil {
			bound = append(bound, p)
		}
	}

	ports, err := xgConnReservePorts(len(bound) + 1)
	if err != nil {
		return err
	}
	m.transportPort = ports[0]
	for i, p := range bound {
		p.listenerPort = ports[i+1]
	}
	return nil
}

// configureRouter gives each pair that needs one its own router listener, replaces the tunnel
// listener's intercepts with the pairs' intercepts, and adds the shared transport listener. It
// runs as a router config tweak, which can't return an error, so it reports a config it can't
// adjust in routerConfigErr.
func (m *xgConnMatrix) configureRouter(cfg *routerEnv.Config) {
	for _, p := range m.pairs {
		if p.client.configureRouter != nil {
			p.client.configureRouter(p, cfg)
		}
	}

	for _, listener := range cfg.Listeners {
		if listener.Name != "tunnel" {
			continue
		}
		options, ok := listener.Options["options"].(map[interface{}]interface{})
		if !ok {
			m.routerConfigErr = errors.New("tunnel listener has no options map")
			continue
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

// startEmbeddedSdk creates the router-embedded SDK fabric once the router's identity has access
// to every pair's service. The fabric loads the services it can dial when it is created, so it
// can tunnel to any of them from then on.
func (m *xgConnMatrix) startEmbeddedSdk(router *EdgeRouterHelper) error {
	watcher := &xgConnServiceWatcher{prefix: m.namePrefix, want: len(m.pairs), done: make(chan struct{})}
	rdm := router.GetRouterDataModel()
	routerId := router.GetRouterId().Token
	rdm.SubscribeToIdentityChanges(routerId, watcher, true)
	defer rdm.UnsubscribeFromIdentityChanges(routerId, watcher)

	select {
	case <-watcher.done:
	case <-time.After(10 * time.Second):
		return errors.New("router identity never gained access to every pair's service")
	}

	fabric, err := xgress_sdk.NewFabric(router, xgress.DefaultOptions())
	if err != nil {
		return fmt.Errorf("create embedded sdk fabric: %w", err)
	}
	m.embedded = fabric
	return nil
}

// close releases the SDK contexts and the tunnel-host listeners, including those of pairs a -run
// filter skipped.
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

// run is the subtest for one pair. It starts the pair's host, waits for its terminator, and runs
// each scenario over a fresh circuit.
func (p *xgConnPair) run(t *testing.T) {
	ctx := p.m.ctx
	ctx.testContextChanged(t)

	acceptor, err := p.host.start(p)
	ctx.Req.NoError(err, "starting the %s host", p.host.name)
	defer acceptor.Close()

	watcher := ctx.AdminManagementSession.newTerminatorWatcher(p.serviceId, 1)
	watcher.waitForTerminators(10 * time.Second)
	watcher.Close()

	for _, scenario := range xgConnScenarios {
		t.Run(scenario.name, func(t *testing.T) {
			ctx.testContextChanged(t)

			circuit, err := p.connect(acceptor)
			ctx.Req.NoError(err)
			defer circuit.Close()

			first, second := circuit.client, circuit.host
			firstName, secondName := "client", "host"
			if scenario.hostFirst {
				first, second = circuit.host, circuit.client
				firstName, secondName = "host", "client"
			}

			if scenario.roundTripFirst {
				request, reply := "request from "+firstName, "reply from "+secondName
				ctx.Req.NoError(xgConnWrite(first, request), "%s request write", firstName)
				data, err := xgConnReadN(second, len(request))
				ctx.Req.NoError(err, "%s read of the request", secondName)
				ctx.Req.Equal(request, data)

				ctx.Req.NoError(xgConnWrite(second, reply), "%s reply write", secondName)
				data, err = xgConnReadN(first, len(reply))
				ctx.Req.NoError(err, "%s read of the reply", firstName)
				ctx.Req.Equal(reply, data)
			}

			ctx.Req.NoError(xgConnWrite(first, "from "+firstName), "%s write", firstName)
			if scenario.halfClose {
				closeWriter, ok := first.(edge.CloseWriter)
				ctx.Req.True(ok, "%s conn type %T should support CloseWrite", firstName, first)
				ctx.Req.NoError(closeWriter.CloseWrite(), "%s close write", firstName)
			} else {
				ctx.Req.NoError(first.Close(), "%s close", firstName)
			}

			// a deadline error here means the close was never relayed
			data, err := xgConnReadToEOF(second)
			ctx.Req.NoError(err, "%s should read to EOF once the %s stops writing; got %q", secondName, firstName, data)
			ctx.Req.Equal("from "+firstName, data)

			if scenario.halfClose {
				// a half-close must not tear down the other direction
				ctx.Req.NoError(xgConnWrite(second, "from "+secondName), "%s write after reading EOF", secondName)
				ctx.Req.NoError(second.Close(), "%s close", secondName)

				data, err = xgConnReadToEOF(first)
				ctx.Req.NoError(err, "%s should read to EOF once the %s closes; got %q", firstName, secondName, data)
				ctx.Req.Equal("from "+secondName, data)
			} else {
				ctx.Req.NoError(second.Close(), "%s close after reading EOF", secondName)
			}
		})
	}
}

// xgConnCircuit is the two ends of one circuit: the conn the initiator kind dialed and the one
// the terminator kind accepted.
type xgConnCircuit struct {
	client net.Conn
	host   net.Conn
}

// Close closes both ends, ignoring errors from ends a scenario already closed.
func (self *xgConnCircuit) Close() {
	_ = self.client.Close()
	_ = self.host.Close()
}

// connect dials the pair's service with the initiator kind and returns both ends of the circuit.
func (p *xgConnPair) connect(acceptor xgConnAcceptor) (*xgConnCircuit, error) {
	type acceptResult struct {
		conn net.Conn
		err  error
	}
	accepted := make(chan acceptResult, 1)
	go func() {
		conn, err := acceptor.Accept()
		accepted <- acceptResult{conn: conn, err: err}
	}()

	client, err := p.client.dial(p)
	if err != nil {
		return nil, fmt.Errorf("%s dial failed: %w", p.client.name, err)
	}

	select {
	case result := <-accepted:
		if result.err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("%s accept failed: %w", p.host.name, result.err)
		}
		return &xgConnCircuit{client: client, host: result.conn}, nil
	case <-time.After(xgConnTimeout):
		_ = client.Close()
		return nil, fmt.Errorf("%s did not accept a connection", p.host.name)
	}
}

// sdkDial dials the pair's service from the SDK client context. It fails if the conn's
// flow-control mode isn't sdkFlowControl, so a pair can't silently land on a different seam than
// its kind names.
func (p *xgConnPair) sdkDial(sdkFlowControl bool) (net.Conn, error) {
	options := &ziti.DialOptions{
		ConnectTimeout: xgConnTimeout,
		SdkFlowControl: &sdkFlowControl,
	}
	conn, err := p.m.sdkClient.DialWithOptions(p.serviceName, options)
	if err != nil {
		return nil, err
	}
	if err = xgConnRequireXgressMode(conn, sdkFlowControl); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

func (p *xgConnPair) sdkListen(sdkFlowControl bool) (xgConnAcceptor, error) {
	options := ziti.DefaultListenOptions()
	options.SdkFlowControl = &sdkFlowControl
	listener, err := p.m.sdkHost.ListenWithOptions(p.serviceName, options)
	if err != nil {
		return nil, err
	}
	return &sdkListenerAcceptor{listener: listener, sdkFlowControl: sdkFlowControl}, nil
}

// terminatorListen starts a local TCP server and points a terminator with the given binding
// at it, so the router dials it directly when a circuit is created.
func (p *xgConnPair) terminatorListen(binding string) (xgConnAcceptor, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	routerId := p.m.ctx.edgeRouterEntity.id
	if _, err = p.m.mgmt.CreateTerminator(p.serviceId, routerId, binding, "tcp:"+l.Addr().String()); err != nil {
		_ = l.Close()
		return nil, fmt.Errorf("create %s terminator: %w", binding, err)
	}
	return &netListenerAcceptor{listener: l}, nil
}

// transportDial creates a circuit through the fabric transport listener, which takes the
// service in a request line before the stream starts.
func (p *xgConnPair) transportDial() (net.Conn, error) {
	addr, err := transport.ParseAddress(fmt.Sprintf("tcp:127.0.0.1:%d", p.m.transportPort))
	if err != nil {
		return nil, err
	}
	clientId := &idlib.TokenId{Token: "xg-matrix-client"}
	serviceId := &idlib.TokenId{Token: p.serviceId}
	conn, err := xgress_transport.ClientDial(addr, clientId, serviceId, nil)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// embeddedSdkDial drives the router-embedded SDK. The fabric wants a net.Conn to bridge, so
// a local TCP pair stands in for the application: the accepted side is handed to the fabric
// and the dialing side is what the test reads and writes.
func (p *xgConnPair) embeddedSdkDial() (net.Conn, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	defer func() { _ = l.Close() }()

	appConn, err := net.DialTimeout("tcp", l.Addr().String(), xgConnTimeout)
	if err != nil {
		return nil, err
	}

	// the handshake completed during the dial, so the connection is already queued
	fabricConn, err := l.Accept()
	if err != nil {
		_ = appConn.Close()
		return nil, err
	}

	options := &ziti.DialOptions{ConnectTimeout: xgConnTimeout}
	if err = p.m.embedded.TunnelWithOptions(p.serviceName, options, fabricConn, true); err != nil {
		_ = appConn.Close()
		_ = fabricConn.Close()
		return nil, fmt.Errorf("embedded sdk tunnel failed: %w", err)
	}
	return appConn, nil
}

// xgConnWrite writes data to conn, failing if the write doesn't complete within xgConnTimeout.
func xgConnWrite(conn net.Conn, data string) error {
	if err := conn.SetWriteDeadline(time.Now().Add(xgConnTimeout)); err != nil {
		return err
	}
	defer func() { _ = conn.SetWriteDeadline(time.Time{}) }()
	_, err := conn.Write([]byte(data))
	return err
}

// xgConnRequireXgressMode returns an error unless conn is an SDK conn whose flow-control mode is
// sdkFlowControl. An SDK conn on its own xgress circuit reports that circuit under the xg key of
// its state.
func xgConnRequireXgressMode(conn net.Conn, sdkFlowControl bool) error {
	sdkConn, ok := conn.(edge.Conn)
	if !ok {
		return fmt.Errorf("conn type %T is not an SDK conn", conn)
	}
	state := map[string]interface{}{}
	if err := json.Unmarshal([]byte(sdkConn.GetState()), &state); err != nil {
		return fmt.Errorf("parse sdk conn state: %w", err)
	}
	if _, xgress := state["xg"]; xgress != sdkFlowControl {
		return fmt.Errorf("expected sdk conn xgress mode %v, got %v", sdkFlowControl, xgress)
	}
	return nil
}

// xgConnReadN reads exactly n bytes from conn and returns what arrived. If they don't all arrive
// within xgConnTimeout it returns what arrived so far with the deadline error.
func xgConnReadN(conn net.Conn, n int) (string, error) {
	if err := conn.SetReadDeadline(time.Now().Add(xgConnTimeout)); err != nil {
		return "", err
	}
	defer func() { _ = conn.SetReadDeadline(time.Time{}) }()
	data := make([]byte, n)
	read, err := io.ReadFull(conn, data)
	return string(data[:read]), err
}

// xgConnReadToEOF reads conn until EOF and returns what arrived. If EOF doesn't arrive within
// xgConnTimeout it returns what arrived so far with the deadline error.
func xgConnReadToEOF(conn net.Conn) (string, error) {
	if err := conn.SetReadDeadline(time.Now().Add(xgConnTimeout)); err != nil {
		return "", err
	}
	defer func() { _ = conn.SetReadDeadline(time.Time{}) }()
	data, err := io.ReadAll(conn)
	return string(data), err
}

// xgConnReservePorts returns n distinct free ports below the default ephemeral ranges (32768 and
// up on Linux, 49152 and up on macOS and Windows). A port from a :0 listen is an ephemeral one,
// which an outbound connection opened while the router starts can take before the router's
// listener binds it.
func xgConnReservePorts(n int) ([]int, error) {
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
		return nil, fmt.Errorf("reserved only %d of %d ports", len(ports), n)
	}
	return ports, nil
}

// xgConnServiceWatcher is an identity subscriber that closes done once the identity has access
// to want services whose names start with prefix.
type xgConnServiceWatcher struct {
	prefix string
	want   int
	done   chan struct{}
	once   sync.Once
}

func (self *xgConnServiceWatcher) NotifyIdentityEvent(state *common.IdentityState, _ common.IdentityEventType) {
	self.check(state)
}

func (self *xgConnServiceWatcher) NotifyServiceChange(state *common.IdentityState, _, _ *common.IdentityService, _ common.ServiceEventType) {
	self.check(state)
}

func (self *xgConnServiceWatcher) NotifyBatchComplete(*common.RouterDataModel, uint64) {}

func (self *xgConnServiceWatcher) check(state *common.IdentityState) {
	if state == nil {
		return
	}
	count := 0
	for _, service := range state.Services {
		if strings.HasPrefix(service.GetName(), self.prefix) {
			count++
		}
	}
	if count >= self.want {
		self.once.Do(func() { close(self.done) })
	}
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

// sdkListenerAcceptor accepts from an SDK listener and fails a conn whose flow-control mode
// isn't sdkFlowControl.
type sdkListenerAcceptor struct {
	listener       edge.Listener
	sdkFlowControl bool
}

func (self *sdkListenerAcceptor) Accept() (net.Conn, error) {
	conn, err := self.listener.AcceptEdge()
	if err != nil {
		return nil, err
	}
	if err = xgConnRequireXgressMode(conn, self.sdkFlowControl); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

func (self *sdkListenerAcceptor) Close() {
	_ = self.listener.Close()
}
