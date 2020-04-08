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
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	cryptoTls "crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/netfoundry/ziti-edge/gateway/enroll"
	"github.com/netfoundry/ziti-edge/gateway/xgress_edge"
	"github.com/netfoundry/ziti-fabric/router"
	"github.com/netfoundry/ziti-fabric/router/xgress"
	"github.com/netfoundry/ziti-foundation/identity/certtools"
	"github.com/netfoundry/ziti-foundation/util/concurrenz"
	nfpem "github.com/netfoundry/ziti-foundation/util/pem"
	sdkconfig "github.com/netfoundry/ziti-sdk-golang/ziti/config"
	sdkenroll "github.com/netfoundry/ziti-sdk-golang/ziti/enroll"
	"github.com/pkg/errors"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gopkg.in/resty.v1"

	"github.com/Jeffail/gabs"
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/server"
	"github.com/netfoundry/ziti-fabric/controller"
	"github.com/netfoundry/ziti-foundation/transport"
	"github.com/netfoundry/ziti-foundation/transport/quic"
	"github.com/netfoundry/ziti-foundation/transport/tcp"
	"github.com/netfoundry/ziti-foundation/transport/tls"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

const (
	ControllerConfFile    = "ats-ctrl.yml"
	EdgeRouterConfFile    = "ats-edge.router.yml"
	TransitRouterConfFile = "ats-transit.router.yml"
)

func init() {
	pfxlog.Global(logrus.DebugLevel)
	pfxlog.SetPrefix("bitbucket.org/netfoundry/")
	logrus.SetFormatter(pfxlog.NewFormatterStartingToday())

	transport.AddAddressParser(quic.AddressParser{})
	transport.AddAddressParser(tls.AddressParser{})
	transport.AddAddressParser(tcp.AddressParser{})
}

type TestContext struct {
	ApiHost            string
	AdminAuthenticator *updbAuthenticator
	AdminSession       *session
	fabricController   *controller.Controller
	EdgeController     *server.Controller
	req                *require.Assertions
	client             *resty.Client
	enabledJsonLogging bool

	edgeRouterEntity    *edgeRouter
	transitRouterEntity *transitRouter
	router              *router.Router
	testing             *testing.T
}

var defaultTestContext = &TestContext{
	AdminAuthenticator: &updbAuthenticator{
		Username: uuid.New().String(),
		Password: uuid.New().String(),
	},
}

func NewTestContext(t *testing.T) *TestContext {
	ret := &TestContext{
		ApiHost: "127.0.0.1:1281",
		AdminAuthenticator: &updbAuthenticator{
			Username: uuid.New().String(),
			Password: uuid.New().String(),
		},
		req: require.New(t),
	}
	ret.testContextChanged(t)

	return ret
}

func GetTestContext() *TestContext {
	return defaultTestContext
}

func (ctx *TestContext) testContextChanged(t *testing.T) {
	ctx.testing = t
	ctx.req = require.New(t)
}

func (ctx *TestContext) Transport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig: &cryptoTls.Config{
			InsecureSkipVerify: true,
		},
	}
}

func (ctx *TestContext) HttpClient(transport *http.Transport) *http.Client {
	jar, err := cookiejar.New(&cookiejar.Options{})
	ctx.req.NoError(err)

	return &http.Client{
		Transport:     transport,
		CheckRedirect: nil,
		Jar:           jar,
		Timeout:       2000 * time.Second,
	}
}

func (ctx *TestContext) Client(httpClient *http.Client) *resty.Client {
	client := resty.NewWithClient(httpClient)
	return client
}

func (ctx *TestContext) NewClient() *resty.Client {
	return ctx.Client(ctx.HttpClient(ctx.Transport()))
}

func (ctx *TestContext) DefaultClient() *resty.Client {
	if ctx.client == nil {
		ctx.client, _, _ = ctx.NewClientComponents()
	}
	return ctx.client
}

func (ctx *TestContext) NewClientComponents() (*resty.Client, *http.Client, *http.Transport) {
	clientTransport := ctx.Transport()
	httpClient := ctx.HttpClient(clientTransport)
	client := ctx.Client(httpClient)

	client.SetHostURL("https://" + ctx.ApiHost)

	return client, httpClient, clientTransport

}

func (ctx *TestContext) startServer() {
	log := pfxlog.Logger()
	_ = os.Mkdir("testdata", os.FileMode(0755))
	err := filepath.Walk("testdata", func(path string, info os.FileInfo, err error) error {
		if err == nil {
			if !info.IsDir() && strings.HasPrefix(info.Name(), "ctrl.db") {
				pfxlog.Logger().Infof("removing test bolt file or backup: %v", path)
				err = os.Remove(path)
			}
		}
		return err
	})
	ctx.req.NoError(err)

	log.Info("loading config")
	config, err := controller.LoadConfig(ControllerConfFile)
	ctx.req.NoError(err)

	log.Info("creating fabric controller")
	ctx.fabricController, err = controller.NewController(config)
	ctx.req.NoError(err)

	log.Info("creating edge controller")
	ctx.EdgeController, err = server.NewController(config)
	ctx.req.NoError(err)

	ctx.EdgeController.SetHostController(ctx.fabricController)

	ctx.EdgeController.Initialize()

	err = ctx.EdgeController.AppEnv.Handlers.Identity.InitializeDefaultAdmin(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password, uuid.New().String())
	if err != nil {
		log.WithError(err).Warn("error during initialize admin")
	}

	// Note we're not starting the fabric controller. Shouldn't need any of it for testing the edge API
	ctx.EdgeController.Run()
	go func() {
		err = ctx.fabricController.Run()
		ctx.req.NoError(err)
	}()
	err = ctx.waitForPort(time.Minute * 5)
	ctx.req.NoError(err)
}

func (ctx *TestContext) createAndEnrollEdgeRouter(roleAttributes ...string) *edgeRouter {
	// If an edge router has already been created, delete it and create a new one
	if ctx.edgeRouterEntity != nil {
		ctx.AdminSession.requireDeleteEntity(ctx.edgeRouterEntity)
		ctx.edgeRouterEntity = nil
	}

	_ = os.MkdirAll("testdata/edge-router", os.FileMode(0755))

	ctx.edgeRouterEntity = ctx.AdminSession.requireNewEdgeRouter(roleAttributes...)
	jwt := ctx.AdminSession.getEdgeRouterJwt(ctx.edgeRouterEntity.id)

	cfgmap, err := router.LoadConfigMap(EdgeRouterConfFile)
	ctx.req.NoError(err)

	enroller := enroll.NewRestEnroller()
	ctx.req.NoError(enroller.LoadConfig(cfgmap))
	ctx.req.NoError(enroller.Enroll([]byte(jwt), true, ""))

	return ctx.edgeRouterEntity
}

func (ctx *TestContext) createAndEnrollTransitRouter() *transitRouter {
	// If a tx router has already been created, delete it and create a new one
	if ctx.transitRouterEntity != nil {
		ctx.AdminSession.requireDeleteEntity(ctx.transitRouterEntity)
		ctx.transitRouterEntity = nil
	}

	_ = os.MkdirAll("testdata/transit-router", os.FileMode(0755))

	ctx.transitRouterEntity = ctx.AdminSession.requireNewTransitRouter()
	jwt := ctx.AdminSession.getTransitRouterJwt(ctx.transitRouterEntity.id)

	cfgmap, err := router.LoadConfigMap(TransitRouterConfFile)
	ctx.req.NoError(err)

	enroller := enroll.NewRestEnroller()
	ctx.req.NoError(enroller.LoadConfig(cfgmap))
	ctx.req.NoError(enroller.Enroll([]byte(jwt), true, ""))

	return ctx.transitRouterEntity
}

func (ctx *TestContext) createEnrollAndStartTransitRouter() {
	ctx.createAndEnrollTransitRouter()
	ctx.startTransitRouter()
}

func (ctx *TestContext) startTransitRouter() {
	config, err := router.LoadConfig(TransitRouterConfFile)
	ctx.req.NoError(err)
	ctx.router = router.Create(config)

	ctx.req.NoError(ctx.router.Start())
}

func (ctx *TestContext) createEnrollAndStartEdgeRouter(roleAttributes ...string) {
	ctx.createAndEnrollEdgeRouter(roleAttributes...)
	ctx.startEdgeRouter()
}

func (ctx *TestContext) startEdgeRouter() {
	config, err := router.LoadConfig(EdgeRouterConfFile)
	ctx.req.NoError(err)
	ctx.router = router.Create(config)

	xgressEdgeFactory := xgress_edge.NewFactory()
	xgress.GlobalRegistry().Register("edge", xgressEdgeFactory)
	ctx.req.NoError(ctx.router.RegisterXctrl(xgressEdgeFactory))
	ctx.req.NoError(ctx.router.Start())
}

func (ctx *TestContext) enrollIdentity(identityId string) *sdkconfig.Config {
	jwt := ctx.AdminSession.getIdentityJwt(identityId)
	tkn, _, err := sdkenroll.ParseToken(jwt)
	ctx.req.NoError(err)

	flags := sdkenroll.EnrollmentFlags{
		Token: tkn,
	}
	conf, err := sdkenroll.Enroll(flags)
	ctx.req.NoError(err)
	return conf
}

func (ctx *TestContext) waitForPort(duration time.Duration) error {
	now := time.Now()
	endTime := now.Add(duration)
	maxWait := duration
	for {
		conn, err := net.DialTimeout("tcp", ctx.ApiHost, maxWait)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		now = time.Now()
		if !now.Before(endTime) {
			return err
		}
		maxWait = endTime.Sub(now)
		time.Sleep(10 * time.Millisecond)
	}
}

func (ctx *TestContext) unauthenticatedSession() *session {
	return &session{
		testContext:   ctx,
		token:         "",
		authenticator: nil,
		identityId:    "",
	}
}

func (ctx *TestContext) loginWithCert(cert *x509.Certificate, key *crypto.PrivateKey) (*session, error) {
	return (&certAuthenticator{
		cert: cert,
		key:  key,
	}).Authenticate(ctx)
}

func (ctx *TestContext) requireAdminLogin() {
	var err error
	ctx.AdminSession, err = ctx.AdminAuthenticator.Authenticate(ctx)
	ctx.req.NoError(err)
}

func (ctx *TestContext) requireLogin(username, password string) *session {
	session, err := ctx.login(username, password)
	ctx.req.NoError(err)
	return session
}

func (ctx *TestContext) login(username, password string) (*session, error) {
	return (&updbAuthenticator{
		Username: username,
		Password: password,
	}).Authenticate(ctx)
}

func (ctx *TestContext) teardown() {
	pfxlog.Logger().Info("tearing down test context")
	ctx.EdgeController.Shutdown()
	ctx.fabricController.Shutdown()
	if ctx.router != nil {
		ctx.req.NoError(ctx.router.Shutdown())
	}
}

func (ctx *TestContext) newRequest() *resty.Request {
	return ctx.DefaultClient().R().
		SetHeader("Content-Type", "application/json")
}

func (ctx *TestContext) completeUpdbEnrollment(identityId string, password string) {
	result := ctx.AdminSession.requireQuery(fmt.Sprintf("identities/%v", identityId))
	path := result.Search(path("data.enrollment.updb.token")...)
	ctx.req.NotNil(path)
	str, ok := path.Data().(string)
	ctx.req.True(ok)

	enrollBody := gabs.New()
	ctx.setJsonValue(enrollBody, password, "password")

	resp, err := ctx.newRequest().
		SetBody(enrollBody.String()).
		Post("enroll?token=" + str)
	ctx.req.NoError(err)
	ctx.logJson(resp.Body())
	ctx.req.Equal(http.StatusOK, resp.StatusCode())
}

func (ctx *TestContext) completeCaAutoEnrollment(certAuth *certAuthenticator) {
	trans := ctx.Transport()
	trans.TLSClientConfig.Certificates = []cryptoTls.Certificate{
		{
			Certificate: [][]byte{certAuth.cert.Raw},
			PrivateKey:  certAuth.key,
		},
	}
	client := ctx.Client(ctx.HttpClient(trans))
	client.SetHostURL("https://" + ctx.ApiHost)

	resp, err := client.NewRequest().
		SetBody("{}").
		Post("enroll?method=ca")
	ctx.req.NoError(err)
	ctx.logJson(resp.Body())
	ctx.req.Equal(http.StatusOK, resp.StatusCode())
}

func (ctx *TestContext) completeOttEnrollment(identityId string) *certAuthenticator {
	result := ctx.AdminSession.requireQuery(fmt.Sprintf("identities/%v", identityId))

	tokenValue := result.Path("data.enrollment.ott.token")

	ctx.req.NotNil(tokenValue)
	token, ok := tokenValue.Data().(string)
	ctx.req.True(ok)

	privateKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	ctx.req.NoError(err)

	request, err := certtools.NewCertRequest(map[string]string{
		"C": "US", "O": "NetFoundry-APi-Test", "CN": identityId,
	}, nil)

	csr, err := x509.CreateCertificateRequest(rand.Reader, request, privateKey)
	ctx.req.NoError(err)

	csrPem := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr})

	resp, err := ctx.newRequest().
		SetBody(csrPem).
		SetHeader("content-type", "application/x-pem-file").
		Post("enroll?token=" + token)
	ctx.req.NoError(err)
	ctx.logJson(resp.Body())
	ctx.req.Equal(http.StatusOK, resp.StatusCode())

	certPem := string(resp.Body())
	certs := nfpem.PemToX509(certPem)

	ctx.req.NotEmpty(certs)

	return &certAuthenticator{
		cert:    certs[0],
		key:     privateKey,
		certPem: certPem,
	}
}

func (ctx *TestContext) validateDateFieldsForCreate(start time.Time, jsonEntity *gabs.Container) time.Time {
	now := time.Now()
	createdAt, updatedAt := ctx.getEntityDates(jsonEntity)
	ctx.req.Equal(createdAt, updatedAt)

	ctx.req.True(start.Before(createdAt) || start.Equal(createdAt))
	ctx.req.True(now.After(createdAt) || now.Equal(createdAt))

	return createdAt
}

func (ctx *TestContext) newService(roleAttributes, configs []string) *service {
	return &service{
		name:           uuid.New().String(),
		roleAttributes: roleAttributes,
		configs:        configs,
		tags:           nil,
	}
}

func (ctx *TestContext) newTerminator(serviceId, routerId, binding, address string) *terminator {
	return &terminator{
		serviceId: serviceId,
		routerId:  routerId,
		binding:   binding,
		address:   address,
		tags:      nil,
	}
}

func (ctx *TestContext) newConfig(configType string, data map[string]interface{}) *config {
	return &config{
		name:       uuid.New().String(),
		configType: configType,
		data:       data,
		tags:       nil,
	}
}

func (ctx *TestContext) newConfigType() *configType {
	return &configType{
		name: uuid.New().String(),
		tags: nil,
	}
}

func (ctx *TestContext) getEntityDates(jsonEntity *gabs.Container) (time.Time, time.Time) {
	createdAtStr := jsonEntity.S("createdAt").Data().(string)
	updatedAtStr := jsonEntity.S("updatedAt").Data().(string)

	ctx.req.NotNil(createdAtStr)
	ctx.req.NotNil(updatedAtStr)

	createdAt, err := time.Parse(time.RFC3339, createdAtStr)
	ctx.req.NoError(err)
	updatedAt, err := time.Parse(time.RFC3339, updatedAtStr)
	ctx.req.NoError(err)
	return createdAt, updatedAt
}

func (ctx *TestContext) validateDateFieldsForUpdate(start time.Time, origCreatedAt time.Time, jsonEntity *gabs.Container) time.Time {
	now := time.Now()
	createdAt, updatedAt := ctx.getEntityDates(jsonEntity)
	ctx.req.Equal(origCreatedAt, createdAt)

	ctx.req.True(createdAt.Before(updatedAt))
	ctx.req.True(start.Before(updatedAt) || start.Equal(updatedAt))
	ctx.req.True(now.After(updatedAt) || now.Equal(updatedAt))

	return createdAt
}

func (ctx *TestContext) validateEntity(entity entity, jsonEntity *gabs.Container) *gabs.Container {
	entity.validate(ctx, jsonEntity)
	return jsonEntity
}

func (ctx *TestContext) idsJson(ids ...string) *gabs.Container {
	entityData := gabs.New()
	ctx.setJsonValue(entityData, ids, "ids")
	return entityData
}

func (ctx *TestContext) requireEntityNotEnrolled(name string, entity *gabs.Container) {
	fingerprint, ok := entity.Path("fingerprint").Data().(string)
	ctx.req.True(ok, "expected "+name+" with isVerified=false to not have a fingerprint, could not cast")
	ctx.req.Empty(fingerprint, "expected "+name+" with isVerified=false to have an empty fingerprint")

	token, ok := entity.Path("enrollmentToken").Data().(string)
	ctx.req.True(ok, "expected "+name+" with isVerified=false to have an enrollment token, could not cast")
	ctx.req.NotEmpty(token, "expected "+name+" with isVerified=false to have an enrollment token, was empty")

	jwt, ok := entity.Path("enrollmentJwt").Data().(string)
	ctx.req.True(ok, "expected "+name+" with isVerified=false to have an enrollment jwt, could not cast")
	ctx.req.NotEmpty(jwt, "expected "+name+" with isVerified=false to have an enrollment jwt, was empty")

	createdAtStr, ok := entity.Path("enrollmentCreatedAt").Data().(string)
	ctx.req.True(ok, "expected "+name+" with isVerified=false to have an enrollment created at date, could not cast")
	ctx.req.NotEmpty(createdAtStr, "expected "+name+" with isVerified=false to have an enrollment created at date string, was empty")

	createdAt, err := time.Parse(time.RFC3339, createdAtStr)
	ctx.req.NoError(err, "expected "+name+" with isVerified=false to have a parsable created at date time string")
	ctx.req.NotEmpty(createdAt, "expected "+name+" with isVerified=false to have an enrollment created at date, was empty")

	expiresAtStr, ok := entity.Path("enrollmentExpiresAt").Data().(string)
	ctx.req.True(ok, "expected "+name+" with isVerified=false to have an enrollment expires at date, could not cast")
	ctx.req.NotEmpty(expiresAtStr, "expected "+name+" with isVerified=false to have an enrollment expires at date string, was empty")

	expiresAt, err := time.Parse(time.RFC3339, expiresAtStr)
	ctx.req.NoError(err, "expected "+name+" with isVerified=false to have a parsable expires at date time string")

	ctx.req.True(ok, "expected "+name+" with isVerified=false to have an enrollment expires at date, could not cast")
	ctx.req.NotEmpty(expiresAt, "expected "+name+" with isVerified=false to have an enrollment expires at date, was empty")

	ctx.req.True(expiresAt.After(createdAt), "expected "+name+" with isVerified=false to have an enrollment expires at date after the created at date")
}

func (ctx *TestContext) requireEntityEnrolled(name string, entity *gabs.Container) {
	fingerprint, ok := entity.Path("fingerprint").Data().(string)
	ctx.req.True(ok, "expected "+name+" with isVerified=true to have a fingerprint, could not cast")
	ctx.req.NotEmpty(fingerprint, "expected "+name+" with isVerified=true to have a fingerprint, was empty")
	ctx.req.False(strings.Contains(fingerprint, ":"), "fingerprint should not contain colons")
	ctx.req.False(strings.ToLower(fingerprint) != fingerprint, "fingerprint should not contain uppercase characters")

	token := entity.Path("enrollmentToken").Data()
	ctx.req.Nil(token, "expected "+name+" with isVerified=true to have an nil enrollment token")

	jwt := entity.Path("enrollmentJwt").Data()
	ctx.req.Nil(jwt, "expected "+name+" with isVerified=true to have an nil enrollment jwt")

	createdAt := entity.Path("enrollmentCreatedAt").Data()
	ctx.req.Nil(createdAt, "expected "+name+" with isVerified=true to have an nil enrollment created at date")

	expiresAt := entity.Path("enrollmentExpiresAt").Data()
	ctx.req.Nil(expiresAt, "expected "+name+" with isVerified=true to have an nil enrollment expires at date")
}

func (ctx *TestContext) wrapConn(conn net.Conn, err error) *testConn {
	ctx.req.NoError(err)
	return &testConn{
		Conn: conn,
		ctx:  ctx,
	}
}

type testConn struct {
	net.Conn
	ctx *TestContext
}

func (conn *testConn) WriteString(val string, timeout time.Duration) {
	conn.ctx.req.NoError(conn.SetWriteDeadline(time.Now().Add(timeout)))
	defer func() { _ = conn.SetWriteDeadline(time.Time{}) }()

	buf := []byte(val)
	n, err := conn.Write(buf)
	conn.ctx.req.NoError(err)
	conn.ctx.req.Equal(n, len(buf))
}

func (conn *testConn) ReadString(maxSize int, timeout time.Duration) string {
	conn.ctx.req.NoError(conn.SetReadDeadline(time.Now().Add(timeout)))
	defer func() { _ = conn.SetReadDeadline(time.Time{}) }()

	buf := make([]byte, maxSize)
	n, err := conn.Read(buf)
	conn.ctx.req.NoError(err)
	return string(buf[:n])
}

func (conn *testConn) ReadExpected(expected string, timeout time.Duration) {
	val := conn.ReadString(len(expected)+1, timeout)
	conn.ctx.req.Equal(expected, val)
}

func (conn *testConn) RequireClose() {
	conn.ctx.req.NoError(conn.Close())
}

var testServerCounter uint64
var testConnectionCounter uint64

func newTestServer(listener net.Listener, dispatcher func(conn *testServerConn) error) *testServer {
	idx := atomic.AddUint64(&testServerCounter, 1)
	return &testServer{
		idx:        idx,
		listener:   listener,
		errorC:     make(chan error, 10),
		msgCount:   0,
		dispatcher: dispatcher,
		waiter:     &sync.WaitGroup{},
	}
}

type testServer struct {
	idx        uint64
	listener   net.Listener
	errorC     chan error
	closed     concurrenz.AtomicBoolean
	msgCount   uint32
	dispatcher func(conn *testServerConn) error
	waiter     *sync.WaitGroup
}

func (server *testServer) waitForDone(ctx *TestContext, timeout time.Duration) {
	select {
	case err, ok := <-server.errorC:
		if ok {
			ctx.req.NoError(err)
		}
	case <-time.After(timeout):
		ctx.req.Fail("wait for done on test server timed out")
	}
}

func (server *testServer) start() {
	go server.acceptLoop()
}

func (server *testServer) close() error {
	server.closed.Set(true)
	return server.listener.Close()
}

func (server *testServer) acceptLoop() {
	var err error
	for !server.closed.Get() {
		var conn net.Conn
		conn, err = server.listener.Accept()
		if conn != nil {
			server.waiter.Add(1)
			connId := atomic.AddUint64(&testConnectionCounter, 1)
			go server.dispatch(&testServerConn{id: connId, Conn: conn, server: server})
		} else {
			break
		}
	}

	// If server is closed, assume this error is just letting us know the listener was closed
	if !server.closed.Get() {
		if err != nil {
			server.errorC <- err
		}
	}

	waitDone := make(chan struct{})
	go func() {
		server.waiter.Wait()
		close(waitDone)
	}()

	select {
	case _, ok := <-waitDone:
		if !ok {
			fmt.Println("all connections closed")
		}
	case <-time.After(10 * time.Second):
		fmt.Println("timed out waiting for all connections to close")
	}

	close(server.errorC)
	fmt.Printf("%v: service exiting\n", server.idx)
}

func (server *testServer) dispatch(conn *testServerConn) {
	defer func() {
		fmt.Printf("marking waiter done to conn %v-%v\n", conn.server.idx, conn.id)
		server.waiter.Done()
	}()

	defer func() {
		val := recover()
		if val != nil {
			if err, ok := val.(error); ok {
				fmt.Printf("panic from server.dispatch: %+v\n", err)
				server.errorC <- err
			}
		}
	}()

	defer func() {
		fmt.Printf("closing conn %v-%v\n", conn.server.idx, conn.id)
		conn.RequireClose()
	}()

	fmt.Printf("beginnging dispatch to conn %v-%v\n", conn.server.idx, conn.id)
	err := server.dispatcher(conn)
	fmt.Printf("finished dispatch to conn %v-%v\n", conn.server.idx, conn.id)
	if err != nil {
		fmt.Printf("failure from server.dispatch: %+v\n", err)
		server.errorC <- err
	}
}

type testServerConn struct {
	id uint64
	net.Conn
	server *testServer
}

func (conn *testServerConn) WriteString(val string, timeout time.Duration) {
	err := conn.SetWriteDeadline(time.Now().Add(timeout))
	if err != nil {
		panic(err)
	}
	defer func() { _ = conn.SetWriteDeadline(time.Time{}) }()

	buf := []byte(val)
	n, err := conn.Write(buf)
	if err != nil {
		panic(fmt.Errorf("conn %v-%v timed out trying to write string %v (%w)", conn.server.idx, conn.id, val, err))
	}
	if n != len(buf) {
		panic(errors.Errorf("conn %v-%v expected to write %v bytes, but only wrote %v", conn.server.idx, conn.id, len(buf), n))
	}
}

func (conn *testServerConn) ReadString(maxSize int, timeout time.Duration) (string, bool) {
	err := conn.SetReadDeadline(time.Now().Add(timeout))
	if err != nil {
		panic(err)
	}
	defer func() { _ = conn.SetReadDeadline(time.Time{}) }()

	buf := make([]byte, maxSize)
	n, err := conn.Read(buf)
	if err != nil {
		if err == io.EOF {
			return "", true
		}
		panic(fmt.Errorf("conn %v-%v timed out trying to read (%w)", conn.server.idx, conn.id, err))
	}
	return string(buf[:n]), false
}

func (conn *testServerConn) ReadExpected(expected string, timeout time.Duration) {
	val, eof := conn.ReadString(len(expected)+1, timeout)
	if eof {
		panic(errors.Errorf("expected to read string '%v', but got EOF", expected))
	}
	if val != expected {
		panic(errors.Errorf("expected to read string '%v', but got '%v'", expected, val))
	}
}

func (conn *testServerConn) RequireClose() {
	err := conn.Close()
	if err != nil {
		panic(err)
	}
}
