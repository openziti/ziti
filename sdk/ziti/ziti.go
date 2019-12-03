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

package ziti

import (
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/common/constants"
	"github.com/netfoundry/ziti-foundation/identity/identity"
	"github.com/netfoundry/ziti-edge/sdk/ziti/config"
	"github.com/netfoundry/ziti-edge/sdk/ziti/edge"
	"github.com/netfoundry/ziti-edge/sdk/ziti/internal/edge_impl"
	"github.com/netfoundry/ziti-foundation/transport"
	"github.com/netfoundry/ziti-foundation/util/info"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Context interface {
	Authenticate() error
	Dial(serviceName string) (net.Conn, error)
	Listen(serviceName string) (net.Listener, error)
	GetServiceId(serviceName string) (string, bool, error)
	GetServices() ([]edge.Service, error)
	GetNetworkSession(id string) (*edge.NetworkSession, error)
	GetNetworkHostSession(id string) (*edge.NetworkSession, error)

	// Close closes any connections open to gateways
	Close()
}

var authUrl, _ = url.Parse("/authenticate?method=cert")
var servicesUrl, _ = url.Parse("/services")
var networkSessionUrl, _ = url.Parse("/network-sessions")

type contextImpl struct {
	config          *config.Config
	initDone        sync.Once
	session         session
	gwConnFactories map[string]edge.ConnFactory
	connMutex       sync.Mutex
}

type session struct {
	id          identity.Identity
	zitiUrl     *url.URL
	tlsCtx      *tls.Config
	clt         http.Client
	session     *edge.Session
	servicesMtx sync.RWMutex
	services    []edge.Service
	netSessions sync.Map
}

func NewContext() Context {
	return &contextImpl{gwConnFactories: make(map[string]edge.ConnFactory)}
}

func NewContextWithConfig(config *config.Config) Context {
	return &contextImpl{gwConnFactories: make(map[string]edge.ConnFactory), config: config}
}

func (context *contextImpl) ensureConfigPresent() error {
	if context.config != nil {
		return nil
	}

	const configEnvVarName = "ZITI_SDK_CONFIG"
	// If configEnvVarName is set, try to use it.
	// The calling application may override this by calling NewContextWithConfig
	confFile := os.Getenv(configEnvVarName)

	if confFile == "" {
		return fmt.Errorf("unable to configure ziti as config environment variable %v not populated", configEnvVarName)
	}

	logrus.Infof("loading Ziti configuration from %s", confFile)
	cfg, err := config.NewFromFile(confFile)
	if err != nil {
		return fmt.Errorf("error loading config file specified by ${%s}: %v", configEnvVarName, err)
	}
	context.config = cfg
	return nil
}

func (context *contextImpl) initialize() error {
	var err error
	context.initDone.Do(func() {
		err = context.initializer()
	})
	return err
}

func (context *contextImpl) initializer() error {
	err := context.ensureConfigPresent()
	if err != nil {
		return err
	}
	session := &context.session

	session.zitiUrl, _ = url.Parse(context.config.ZtAPI)

	id, err := identity.LoadIdentity(context.config.ID)
	if err != nil {
		return err
	}

	session.id = id
	session.tlsCtx = id.ClientTLSConfig()
	session.clt = http.Client{
		Transport: &http.Transport{
			TLSClientConfig: session.tlsCtx,
		},
		Timeout: 30 * time.Second,
	}

	if err = context.Authenticate(); err != nil {
		return err
	}

	// get services
	if session.services, err = context.getServices(); err != nil {
		return err
	}

	return nil
}

func (context *contextImpl) Authenticate() error {
	logrus.Info("attempting to authenticate")
	context.session.netSessions = sync.Map{}
	session := &context.session

	req := new(bytes.Buffer)
	json.NewEncoder(req).Encode(info.GetSdkInfo())
	resp, err := session.clt.Post(session.zitiUrl.ResolveReference(authUrl).String(), "application/json", req)

	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		logrus.Fatal("failed to authenticate with ZT controller")
	}

	apiSessionResp := edge.Session{}

	_, err = edge.ApiResponseDecode(&apiSessionResp, resp.Body)
	if err != nil {
		return err
	}
	logrus.
		WithField("token", apiSessionResp.Token).
		WithField("id", apiSessionResp.Id).
		Debugf("Got session: %v", apiSessionResp)
	session.session = &apiSessionResp

	return nil
}

func (context *contextImpl) Dial(serviceName string) (net.Conn, error) {
	if err := context.initialize(); err != nil {
		return nil, fmt.Errorf("failed to initilize context: (%v)", err)
	}
	id, ok := context.getServiceId(serviceName)
	if !ok {
		return nil, fmt.Errorf("service '%s' not found in ZT", serviceName)
	}

	var conn net.Conn
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		ns, err := context.GetNetworkSession(id)
		if err != nil {
			return nil, err
		}
		conn, err = context.dialSession(serviceName, ns)
		if err != nil && attempt == 0 {
			if strings.Contains(err.Error(), "closed") {
				context.deleteNetworkSession(id)
				continue
			}
		}
		return conn, err
	}
	return nil, fmt.Errorf("unable to dial service '%s' (%v)", serviceName, err)
}

func (context *contextImpl) dialSession(service string, netSession *edge.NetworkSession) (net.Conn, error) {
	edgeConnFactory, err := context.getGatewayConnFactory(netSession)
	if err != nil {
		return nil, err
	}
	edgeConn := edgeConnFactory.NewConn(service)
	return edgeConn.Connect(netSession)
}

func (context *contextImpl) Listen(serviceName string) (net.Listener, error) {
	if err := context.initialize(); err != nil {
		return nil, fmt.Errorf("failed to initilize context: (%v)", err)
	}

	if id, ok, _ := context.GetServiceId(serviceName); ok {
		for attempt := 0; attempt < 2; attempt++ {
			ns, err := context.GetNetworkHostSession(id)
			if err != nil {
				return nil, err
			}
			listener, err := context.listenSession(ns, serviceName)
			if err != nil && attempt == 0 {
				if strings.Contains(err.Error(), "closed") {
					context.deleteNetworkSession(id)
					continue
				}
			}
			return listener, err
		}
	}
	return nil, fmt.Errorf("service '%s' not found in ZT", serviceName)
}

func (context *contextImpl) listenSession(netSession *edge.NetworkSession, serviceName string) (net.Listener, error) {
	edgeConnFactory, err := context.getGatewayConnFactory(netSession)
	if err != nil {
		return nil, err
	}
	edgeConn := edgeConnFactory.NewConn(serviceName)
	return edgeConn.Listen(netSession, serviceName)
}

func (context *contextImpl) getGatewayConnFactory(netSession *edge.NetworkSession) (edge.ConnFactory, error) {
	logger := pfxlog.Logger().WithField("ns", netSession.Token)

	if len(netSession.Gateways) == 0 {
		return nil, errors.New("no edge routers available")
	}
	gateway := netSession.Gateways[0]
	ingressUrl := gateway.Urls["tls"]

	context.connMutex.Lock()
	defer context.connMutex.Unlock()

	// remove any closed connections
	for key, val := range context.gwConnFactories {
		if val.IsClosed() {
			delete(context.gwConnFactories, key)
		}
	}

	if edgeConn, found := context.gwConnFactories[ingressUrl]; found {
		return edgeConn, nil
	}

	ingAddr, err := transport.ParseAddress(ingressUrl)
	if err != nil {
		return nil, err
	}

	id := context.session.id
	dialer := channel2.NewClassicDialer(identity.NewIdentity(id), ingAddr, map[int32][]byte{
		edge.SessionTokenHeader: []byte(context.session.session.Token),
	})

	ch, err := channel2.NewChannel("ziti-sdk", dialer, nil)
	if err != nil {
		logger.Error(err)
		return nil, err
	}

	edgeConn := edge_impl.NewEdgeConnFactory(ch)
	context.gwConnFactories[ingressUrl] = edgeConn
	return edgeConn, nil
}

func (context *contextImpl) GetServiceId(name string) (string, bool, error) {
	if err := context.initialize(); err != nil {
		return "", false, fmt.Errorf("failed to initilize context: (%v)", err)
	}

	id, found := context.getServiceId(name)
	return id, found, nil
}

func (context *contextImpl) getServiceId(name string) (string, bool) {
	context.session.servicesMtx.RLock()
	defer context.session.servicesMtx.RUnlock()

	for _, s := range context.session.services {
		if s.Name == name {
			return s.Id, true
		}
	}
	return "", false
}

func (context *contextImpl) GetServices() ([]edge.Service, error) {
	if err := context.initialize(); err != nil {
		return nil, fmt.Errorf("failed to initilize context: (%v)", err)
	}
	return context.getServices()
}

func (context *contextImpl) getServices() ([]edge.Service, error) {

	servReq, _ := http.NewRequest("GET", context.session.zitiUrl.ResolveReference(servicesUrl).String(), nil)

	if context.session.session.Token == "" {
		return nil, errors.New("session token is empty")
	}
	servReq.Header.Set(constants.ZitiSession, context.session.session.Token)
	pgOffset := 0
	pgLimit := 100
	servicesMap := make(map[string]edge.Service)

	for {
		q := servReq.URL.Query()
		q.Set("limit", strconv.Itoa(pgLimit))
		q.Set("offset", strconv.Itoa(pgOffset))
		servReq.URL.RawQuery = q.Encode()
		resp, err := context.session.clt.Do(servReq)

		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			return nil, errors.New("unauthorized")
		}

		if err != nil {
			return nil, err
		}

		s := &[]edge.Service{}
		meta, err := edge.ApiResponseDecode(s, resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if meta == nil {
			// shouldn't happen
			return nil, errors.New("nil metadata in response to GET /services")
		}
		if meta.Pagination == nil {
			return nil, errors.New("nil pagination in response to GET /services")
		}

		for _, svc := range *s {
			servicesMap[svc.Name] = svc
		}

		pgOffset += pgLimit
		if pgOffset >= meta.Pagination.TotalCount {
			break
		}
	}

	services := make([]edge.Service, len(servicesMap))
	i := 0
	for _, s := range servicesMap {
		services[i] = s
		i++
	}
	context.session.servicesMtx.Lock()
	context.session.services = services
	context.session.servicesMtx.Unlock()
	return services, nil
}

func (context *contextImpl) GetNetworkSession(id string) (*edge.NetworkSession, error) {
	return context.getNetworkSession(id, false)
}

func (context *contextImpl) GetNetworkHostSession(id string) (*edge.NetworkSession, error) {
	return context.getNetworkSession(id, true)
}

func (context *contextImpl) getNetworkSession(id string, host bool) (*edge.NetworkSession, error) {
	if err := context.initialize(); err != nil {
		return nil, fmt.Errorf("failed to initilize context: (%v)", err)
	}
	val, ok := context.session.netSessions.Load(id)
	if ok {
		return val.(*edge.NetworkSession), nil
	}
	body := fmt.Sprintf(`{"serviceId":"%s", "hosting": %s}`, id, strconv.FormatBool(host))
	reqBody := bytes.NewBufferString(body)

	req, _ := http.NewRequest("POST", context.session.zitiUrl.ResolveReference(networkSessionUrl).String(), reqBody)
	req.Header.Set(constants.ZitiSession, context.session.session.Token)
	req.Header.Set("Content-Type", "application/json")

	logrus.WithField("service_id", id).Debug("requesting network session")
	resp, err := context.session.clt.Do(req)

	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		respBody, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("Failed to create network session: %s\n%s", resp.Status, string(respBody))
	}

	netSession := new(edge.NetworkSession)
	_, err = edge.ApiResponseDecode(netSession, resp.Body)
	if err != nil {
		pfxlog.Logger().Error("failed to decode net session response", err)
		return nil, err
	}
	context.session.netSessions.Store(id, netSession)

	return netSession, nil
}

func (context *contextImpl) deleteNetworkSession(id string) {
	context.session.netSessions.Delete(id)
}

func (context *contextImpl) Close() {
	logger := pfxlog.Logger()

	context.connMutex.Lock()
	defer context.connMutex.Unlock()

	// remove any closed connections
	for key, val := range context.gwConnFactories {
		if !val.IsClosed() {
			if err := val.Close(); err != nil {
				logger.WithError(err).Error("error while closing connection")
			}
		}
		delete(context.gwConnFactories, key)
	}
}
